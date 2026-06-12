// Package reg 把注册子系统装配成一个可被 Wails App 直接调用的门面（Manager）。
package reg

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"time"

	"web2img/internal/reg/crypto"
	"web2img/internal/reg/dto"
	gptdispatch "web2img/internal/reg/regkit/dispatcher/gpt"
	"web2img/internal/reg/regkit/dispatcher"
	"web2img/internal/reg/regkit/mailbox"
	"web2img/internal/reg/regkit/proxypicker"
	"web2img/internal/reg/regkit/smspool"
	"web2img/internal/reg/regkit/workerpool"
	"web2img/internal/reg/repo"
	"web2img/internal/reg/service"

	"gorm.io/gorm"
)

// Manager 注册子系统门面。
type Manager struct {
	db      *gorm.DB
	aes     *crypto.AESGCM
	sysCfg  *service.SystemConfigService
	cfgRepo *repo.SystemConfigRepo

	MailSvc *service.MailPoolService
	TaskSvc *service.RegisterTaskService
	GptSvc  *service.PoolGptService

	mailMgr     *mailbox.Manager
	proxyPicker *proxypicker.Picker
	pool        *workerpool.Pool
	stopCh      chan struct{}
}

// NewManager 在 dataDir 下打开 reg.db、装配全部服务与 dispatcher，并启动 worker pool。
func NewManager(dataDir string) (*Manager, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	db, err := repo.Open(filepath.Join(dataDir, "reg.db"))
	if err != nil {
		return nil, err
	}
	aes, err := loadOrCreateAES(filepath.Join(dataDir, "reg.key"))
	if err != nil {
		return nil, err
	}

	cfgRepo := repo.NewSystemConfigRepo(db)
	mailRepo := repo.NewMailPoolRepo(db)
	taskRepo := repo.NewRegisterTaskRepo(db)
	logRepo := repo.NewRegisterTaskLogRepo(db)
	gptRepo := repo.NewPoolGptRepo(db)
	phoneRepo := repo.NewPhonePoolRepo(db)
	proxyRepo := repo.NewProxyRepo(db)

	sysCfg := service.NewSystemConfigService(cfgRepo)
	proxySvc := service.NewProxyService(proxyRepo, aes)
	mailSvc := service.NewMailPoolService(mailRepo, aes)
	gptSvc := service.NewPoolGptService(gptRepo, aes)
	taskSvc := service.NewRegisterTaskService(taskRepo, logRepo, mailRepo)

	// regkit 依赖装配
	mailMgr := mailbox.NewManager(mailRepo, aes)
	smsMgr := smspool.NewManager(phoneRepo, sysCfg)
	proxyPicker := proxypicker.NewPicker(proxySvc, proxyRepo, sysCfg)
	regDeps := dispatcher.Deps{MailMgr: mailMgr, ProxyPicker: proxyPicker, SysCfg: sysCfg, SMSMgr: smsMgr}

	taskSvc.RegisterDispatcher("gpt", &gptdispatch.Dispatcher{Deps: regDeps, Pool: gptSvc})

	// worker pool：并发由配置 register.worker_concurrency 决定（默认 3）
	conc := int(sysCfg.GetInt(context.Background(), "register.worker_concurrency", 3))
	pool := workerpool.New(conc, taskSvc.RunTask)
	taskSvc.SetSubmitter(pool)
	pool.Start()

	m := &Manager{
		db:          db,
		aes:         aes,
		sysCfg:      sysCfg,
		cfgRepo:     cfgRepo,
		MailSvc:     mailSvc,
		TaskSvc:     taskSvc,
		GptSvc:      gptSvc,
		mailMgr:     mailMgr,
		proxyPicker: proxyPicker,
		pool:        pool,
		stopCh:      make(chan struct{}),
	}
	// 恢复上次崩溃残留任务
	go func() { _, _, _ = taskSvc.RecoverPending(context.Background()) }()
	// 自动刷新有效期：周期性把临近过期的号 access_token 续上
	go m.autoRefreshLoop()
	return m, nil
}

// pickProxyURL 取一个代理 URL（尊重动态代理网关配置）；失败返回直连。
func (m *Manager) pickProxyURL(ctx context.Context) string {
	if m.proxyPicker == nil {
		return ""
	}
	if r, err := m.proxyPicker.Pick(ctx, 0); err == nil && r != nil {
		return r.URL
	}
	return ""
}

// autoRefreshLoop 每 30 分钟把 2 小时内到期的号刷新一遍。
func (m *Manager) autoRefreshLoop() {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	run := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()
		rows, err := m.GptSvc.ListRefreshableIDs(ctx, 2*3600)
		if err != nil || len(rows) == 0 {
			return
		}
		_, _ = m.GptSvc.BatchRefresh(ctx, rows, m.pickProxyURL(ctx))
	}
	// 启动后稍等再跑首轮
	select {
	case <-time.After(2 * time.Minute):
		run()
	case <-m.stopCh:
		return
	}
	for {
		select {
		case <-ticker.C:
			run()
		case <-m.stopCh:
			return
		}
	}
}

// GptImport 批量导入号池。
func (m *Manager) GptImport(ctx context.Context, text string) (*dto.GptPoolImportResult, error) {
	return m.GptSvc.Import(ctx, text)
}

// GptUpdate 编辑号池行。
func (m *Manager) GptUpdate(ctx context.Context, id uint64, req *dto.GptPoolUpdateReq) error {
	return m.GptSvc.Update(ctx, id, req)
}

// GptDeleteInvalid 删除失效号。
func (m *Manager) GptDeleteInvalid(ctx context.Context) (int64, error) {
	return m.GptSvc.DeleteInvalid(ctx)
}

// GptRefresh 刷新单个号的有效期。
func (m *Manager) GptRefresh(ctx context.Context, id uint64) (*dto.GptRefreshResp, error) {
	return m.GptSvc.RefreshOne(ctx, id, m.pickProxyURL(ctx))
}

// GptBatchRefresh 批量刷新有效期（ids 空=全部可刷新）。
func (m *Manager) GptBatchRefresh(ctx context.Context, ids []uint64) (*dto.GptBatchRefreshResp, error) {
	return m.GptSvc.BatchRefresh(ctx, ids, m.pickProxyURL(ctx))
}

// GptProbeQuota 查询单个号的额度。
func (m *Manager) GptProbeQuota(ctx context.Context, id uint64) (*dto.GptQuotaResp, error) {
	return m.GptSvc.ProbeQuota(ctx, id, m.pickProxyURL(ctx))
}

// GptDetail 返回单个号的明文凭证（编辑用）。
func (m *Manager) GptDetail(ctx context.Context, id uint64) (*dto.GptPoolDetailResp, error) {
	return m.GptSvc.Detail(ctx, id)
}

// PoolPickToken 选一个可用号池账号（号池生图用）。accountID=0 自动挑选。
// exclude 中的账号 ID 会被跳过（出图失败后切号用）。
func (m *Manager) PoolPickToken(ctx context.Context, accountID uint64, exclude ...uint64) (*service.UsableToken, error) {
	return m.GptSvc.PickUsable(ctx, accountID, m.pickProxyURL(ctx), exclude...)
}

// PoolAcquireToken 为号池生图申请一个账号（一号一并发，随机挑号，全忙则排队等待）。
// 用完必须 PoolReleaseAccount 释放。accountID=0 自动随机挑选；exclude 跳过指定账号。
func (m *Manager) PoolAcquireToken(ctx context.Context, accountID uint64, exclude ...uint64) (*service.UsableToken, error) {
	return m.GptSvc.AcquireUsable(ctx, accountID, m.pickProxyURL(ctx), exclude)
}

// PoolReleaseAccount 释放号池生图占用的账号。
func (m *Manager) PoolReleaseAccount(id uint64) { m.GptSvc.Release(id) }

// PoolProxyURL 出图用代理 URL。
func (m *Manager) PoolProxyURL(ctx context.Context) string { return m.pickProxyURL(ctx) }

// PoolMarkUsed 出图成功后累加号池账号使用计数。
func (m *Manager) PoolMarkUsed(ctx context.Context, id uint64) { m.GptSvc.MarkUsed(ctx, id) }

// Config 读取注册子系统配置（打码 / 短信 / 邮箱后端 / 代理等 key）。
func (m *Manager) Config(ctx context.Context) (map[string]string, error) {
	return m.cfgRepo.All(ctx)
}

// SetConfig 批量写入配置。
func (m *Manager) SetConfig(ctx context.Context, kv map[string]string) error {
	for k, v := range kv {
		if err := m.cfgRepo.Set(ctx, k, v); err != nil {
			return err
		}
	}
	return nil
}

// GptExport 导出号池为文本（每行 email----password----access----refresh）。
func (m *Manager) GptExport(ctx context.Context, scope string, ids []uint64) (string, error) {
	return m.GptSvc.ExportText(ctx, scope, ids, m.aes)
}

// MailFetch 收取指定邮箱最近邮件（只读预览）。
func (m *Manager) MailFetch(ctx context.Context, id uint64, limit int) ([]mailbox.MailMessage, error) {
	cfg := dispatcher.BuildMailBackendConfig(ctx, m.sysCfg)
	return m.mailMgr.FetchRecent(ctx, id, cfg, limit)
}

// dtoAlias 让 App 包能不直接 import dto。
type (
	MailImportResult = dto.MailPoolImportResult
)

func loadOrCreateAES(keyPath string) (*crypto.AESGCM, error) {
	if data, err := os.ReadFile(keyPath); err == nil {
		if key, err := hex.DecodeString(string(data)); err == nil && len(key) == 32 {
			return crypto.NewAESGCM(key)
		}
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, []byte(hex.EncodeToString(key)), 0o600); err != nil {
		return nil, err
	}
	return crypto.NewAESGCM(key)
}
