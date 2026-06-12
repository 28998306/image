package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"web2img/internal/reg/crypto"
	"web2img/internal/reg/dto"
	"web2img/internal/reg/model"
	"web2img/internal/reg/repo"
)

// PoolGptService GPT 号池服务。
type PoolGptService struct {
	repo *repo.PoolGptRepo
	aes  *crypto.AESGCM

	busyMu sync.Mutex
	busy   map[uint64]bool // 号池生图占用中的账号（一号一并发）
}

// NewPoolGptService 构造。
func NewPoolGptService(r *repo.PoolGptRepo, aes *crypto.AESGCM) *PoolGptService {
	return &PoolGptService{repo: r, aes: aes, busy: make(map[uint64]bool)}
}

func (s *PoolGptService) enc(plain string) []byte {
	if plain == "" || s.aes == nil {
		return nil
	}
	out, err := s.aes.Encrypt([]byte(plain))
	if err != nil {
		return nil
	}
	return out
}

func (s *PoolGptService) dec(b []byte) string {
	if len(b) == 0 || s.aes == nil {
		return ""
	}
	if out, err := s.aes.Decrypt(b); err == nil {
		return string(out)
	}
	return ""
}

// Create 落库一条新注册 GPT 账号。
func (s *PoolGptService) Create(ctx context.Context, req *dto.GptPoolCreateReq) (*model.PoolGpt, error) {
	status := req.Status
	if status == "" {
		status = model.GPTStatusValid
	}
	p := &model.PoolGpt{
		Email:           strings.ToLower(strings.TrimSpace(req.Email)),
		Status:          status,
		PasswordEnc:     s.enc(req.Password),
		AccessTokenEnc:  s.enc(req.AccessToken),
		RefreshTokenEnc: s.enc(req.RefreshToken),
		IDTokenEnc:      s.enc(req.IDToken),
		APIKeyEnc:       s.enc(req.APIKey),
	}
	if req.OAuthIssuer != "" {
		p.OAuthIssuer = &req.OAuthIssuer
	}
	if req.OAuthClientID != "" {
		p.OAuthClientID = &req.OAuthClientID
	}
	if req.PlanType != "" {
		p.PlanType = &req.PlanType
	}
	if req.ChatGPTAccountID != "" {
		p.ChatGPTAccountID = &req.ChatGPTAccountID
	}
	if req.Notes != "" {
		p.Notes = &req.Notes
	}
	if req.ExpiresAt > 0 {
		t := time.UnixMilli(req.ExpiresAt).UTC()
		p.ExpiresAt = &t
	}
	if err := s.repo.Create(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// Update 编辑一行（空字符串=不改；token 留空=保留旧值；status 留空=不改）。
func (s *PoolGptService) Update(ctx context.Context, id uint64, req *dto.GptPoolUpdateReq) error {
	fields := map[string]any{}
	if req.Status != "" {
		fields["status"] = req.Status
	}
	fields["notes"] = req.Notes
	if req.Password != "" {
		fields["password_enc"] = s.enc(req.Password)
	}
	if req.AccessToken != "" {
		fields["access_token_enc"] = s.enc(req.AccessToken)
		if exp, ok := jwtExpUnix(req.AccessToken); ok {
			t := time.Unix(exp, 0).UTC()
			fields["expires_at"] = t
		}
	}
	if req.RefreshToken != "" {
		fields["refresh_token_enc"] = s.enc(req.RefreshToken)
	}
	if req.IDToken != "" {
		fields["id_token_enc"] = s.enc(req.IDToken)
	}
	if req.APIKey != "" {
		fields["api_key_enc"] = s.enc(req.APIKey)
	}
	if req.ExpiresAt > 0 {
		fields["expires_at"] = time.UnixMilli(req.ExpiresAt).UTC()
	}
	if len(fields) == 0 {
		return nil
	}
	return s.repo.Update(ctx, id, fields)
}

// DeleteInvalid 删除所有「失效」状态的号。
func (s *PoolGptService) DeleteInvalid(ctx context.Context) (int64, error) {
	return s.repo.SoftDeleteByStatus(ctx, model.GPTStatusInvalid)
}

// Import 批量导入号池，兼容 sub2api-data / codex 单文件 / 扁平 JSON / 数组。
// 按邮箱 upsert：已存在则更新 token/有效期，否则新建。
func (s *PoolGptService) Import(ctx context.Context, text string) (*dto.GptPoolImportResult, error) {
	reqs, err := parseGptImport(text)
	if err != nil {
		return nil, err
	}
	res := &dto.GptPoolImportResult{}
	for _, r := range reqs {
		email := strings.ToLower(strings.TrimSpace(r.Email))
		if email == "" || (r.AccessToken == "" && r.RefreshToken == "") {
			res.Skipped++
			continue
		}
		r.Email = email
		// 用 access_token 补全 plan_type / chatgpt_account_id / 有效期
		if r.AccessToken != "" {
			claims := jwtClaims(r.AccessToken)
			if r.PlanType == "" {
				r.PlanType = planTypeFromClaims(claims)
			}
			if r.ChatGPTAccountID == "" {
				r.ChatGPTAccountID = chatgptAccountIDFromClaims(claims)
			}
			// 记录签发 token 的 client_id —— 刷新 RT 时必须用同一个 client_id。
			if r.OAuthClientID == "" {
				r.OAuthClientID = clientIDFromClaims(claims)
			}
			if r.ExpiresAt == 0 {
				if exp, ok := jwtExpUnix(r.AccessToken); ok {
					r.ExpiresAt = exp * 1000
				}
			}
		}
		existing, gerr := s.repo.GetByEmail(ctx, email)
		if gerr == nil && existing != nil {
			fields := map[string]any{"status": model.GPTStatusValid}
			if r.AccessToken != "" {
				fields["access_token_enc"] = s.enc(r.AccessToken)
			}
			if r.RefreshToken != "" {
				fields["refresh_token_enc"] = s.enc(r.RefreshToken)
			}
			if r.IDToken != "" {
				fields["id_token_enc"] = s.enc(r.IDToken)
			}
			if r.PlanType != "" {
				fields["plan_type"] = r.PlanType
			}
			if r.ChatGPTAccountID != "" {
				fields["chatgpt_account_id"] = r.ChatGPTAccountID
			}
			if r.OAuthClientID != "" {
				fields["oauth_client_id"] = r.OAuthClientID
			}
			if r.ExpiresAt > 0 {
				fields["expires_at"] = time.UnixMilli(r.ExpiresAt).UTC()
			}
			if err := s.repo.Update(ctx, existing.ID, fields); err != nil {
				res.Errors = append(res.Errors, email+": "+err.Error())
				continue
			}
			res.Updated++
			continue
		}
		rr := r
		if _, err := s.Create(ctx, &rr); err != nil {
			res.Errors = append(res.Errors, email+": "+err.Error())
			continue
		}
		res.Imported++
	}
	return res, nil
}

// List 列表。
func (s *PoolGptService) List(ctx context.Context, f repo.PoolGptFilter) ([]*dto.GptPoolResp, int64, error) {
	rows, total, err := s.repo.List(ctx, f)
	if err != nil {
		return nil, 0, err
	}
	out := make([]*dto.GptPoolResp, 0, len(rows))
	for _, r := range rows {
		out = append(out, gptPoolToResp(r))
	}
	return out, total, nil
}

// Stats 统计。
func (s *PoolGptService) Stats(ctx context.Context) (*dto.GptPoolStatsResp, error) {
	m, err := s.repo.Stats(ctx)
	if err != nil {
		return nil, err
	}
	return &dto.GptPoolStatsResp{
		Total:    m["total"],
		Valid:    m[model.GPTStatusValid],
		Invalid:  m[model.GPTStatusInvalid],
		Disabled: m[model.GPTStatusDisabled],
		Cooldown: m[model.GPTStatusCooldown],
	}, nil
}

// Delete 删除一行。
func (s *PoolGptService) Delete(ctx context.Context, id uint64) error {
	return s.repo.SoftDelete(ctx, id)
}

// DeleteByIDs 批量删除。
func (s *PoolGptService) DeleteByIDs(ctx context.Context, ids []uint64) (int64, error) {
	return s.repo.SoftDeleteByIDs(ctx, ids)
}

// ExportText 导出号池为文本，每行 email----password----access_token----refresh_token。
func (s *PoolGptService) ExportText(ctx context.Context, scope string, ids []uint64, aes *crypto.AESGCM) (string, error) {
	rows, err := s.repo.ListForExport(ctx, scope, ids, 0)
	if err != nil {
		return "", err
	}
	dec := func(b []byte) string {
		if len(b) == 0 || aes == nil {
			return ""
		}
		if out, e := aes.Decrypt(b); e == nil {
			return string(out)
		}
		return ""
	}
	var sb strings.Builder
	for _, p := range rows {
		sb.WriteString(p.Email)
		sb.WriteString("----")
		sb.WriteString(dec(p.PasswordEnc))
		sb.WriteString("----")
		sb.WriteString(dec(p.AccessTokenEnc))
		sb.WriteString("----")
		sb.WriteString(dec(p.RefreshTokenEnc))
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

func gptPoolToResp(p *model.PoolGpt) *dto.GptPoolResp {
	r := &dto.GptPoolResp{
		ID:                  p.ID,
		Email:               p.Email,
		Status:              p.Status,
		SuccessCount:        p.SuccessCount,
		FailureCount:        p.FailureCount,
		HasAccessToken:      len(p.AccessTokenEnc) > 0,
		HasRefreshToken:     len(p.RefreshTokenEnc) > 0,
		RegisteredAt:        p.RegisteredAt.UnixMilli(),
		ImageQuotaRemaining: p.ImageQuotaRemaining,
		ImageQuotaTotal:     p.ImageQuotaTotal,
	}
	if p.PlanType != nil {
		r.PlanType = *p.PlanType
	}
	if p.ChatGPTAccountID != nil {
		r.ChatGPTAccountID = *p.ChatGPTAccountID
	}
	if p.Notes != nil {
		r.Notes = *p.Notes
	}
	if p.ExpiresAt != nil {
		r.ExpiresAt = p.ExpiresAt.UnixMilli()
	}
	if p.LastRefreshAt != nil {
		r.LastRefreshAt = p.LastRefreshAt.UnixMilli()
	}
	if p.ImageQuotaResetAt != nil {
		r.ImageQuotaResetAt = p.ImageQuotaResetAt.UnixMilli()
	}
	if p.LastQuotaCheckAt != nil {
		r.LastQuotaCheckAt = p.LastQuotaCheckAt.UnixMilli()
	}
	return r
}

// UsableToken 一个可用号池账号的运行期凭证（号池生图用）。
type UsableToken struct {
	ID          uint64
	Email       string
	AccessToken string
	AccountID   string // chatgpt_account_id
	ExpiresAt   int64  // unix 毫秒
}

// PickUsable 选一个可用账号并解出 access_token；过期且可刷新时先尝试刷新。
// exclude 中的账号 ID 会被跳过（出图失败后切号用）。
func (s *PoolGptService) PickUsable(ctx context.Context, id uint64, proxyURL string, exclude ...uint64) (*UsableToken, error) {
	p, err := s.repo.PickUsable(ctx, id, exclude...)
	if err != nil {
		if err == repo.ErrNotFound {
			return nil, fmt.Errorf("没有可用号池账号（需状态有效且 access_token 未过期）")
		}
		return nil, err
	}
	at := s.dec(p.AccessTokenEnc)
	// 临近过期（<60s）且有 RT → 试着刷新一次
	if exp, ok := jwtExpUnix(at); ok && time.Now().Unix() >= exp-60 {
		if s.dec(p.RefreshTokenEnc) != "" {
			if _, rerr := s.RefreshOne(ctx, p.ID, proxyURL); rerr == nil {
				if np, e := s.repo.GetByID(ctx, p.ID); e == nil {
					p = np
					at = s.dec(p.AccessTokenEnc)
				}
			}
		}
	}
	if at == "" {
		return nil, fmt.Errorf("账号 %s 未取得 access_token", p.Email)
	}
	out := &UsableToken{ID: p.ID, Email: p.Email, AccessToken: at}
	if p.ChatGPTAccountID != nil {
		out.AccountID = *p.ChatGPTAccountID
	}
	if out.AccountID == "" {
		out.AccountID = chatgptAccountIDFromClaims(jwtClaims(at))
	}
	if p.ExpiresAt != nil {
		out.ExpiresAt = p.ExpiresAt.UnixMilli()
	}
	return out, nil
}

// MarkUsed 出图成功后累加使用计数。
func (s *PoolGptService) MarkUsed(ctx context.Context, id uint64) {
	_ = s.repo.IncrSuccess(ctx, id)
}

// busyKeys 返回当前占用中的账号 ID 列表（需持锁调用）。
func (s *PoolGptService) busyKeys() []uint64 {
	ids := make([]uint64, 0, len(s.busy))
	for id := range s.busy {
		ids = append(ids, id)
	}
	return ids
}

// reserve 在锁内挑一个账号并标记占用，保证并发挑号互不撞号（一号一并发）。
// id>0 指定账号（忽略忙碌，按用户意愿）；否则随机挑一个未占用且不在 extra 里的。
// 返回 ErrNotFound 表示「全忙」，errNoAccount 表示「根本没有可用账号」。
var errNoAccount = fmt.Errorf("没有可用号池账号（需状态有效且 access_token 未过期）")

func (s *PoolGptService) reserve(ctx context.Context, id uint64, extra []uint64) (*model.PoolGpt, error) {
	s.busyMu.Lock()
	defer s.busyMu.Unlock()
	if id > 0 {
		p, err := s.repo.PickUsable(ctx, id)
		if err != nil {
			if err == repo.ErrNotFound {
				return nil, errNoAccount
			}
			return nil, err
		}
		s.busy[p.ID] = true
		return p, nil
	}
	exclude := append(s.busyKeys(), extra...)
	p, err := s.repo.PickUsableRandom(ctx, exclude)
	if err != nil {
		if err == repo.ErrNotFound {
			// 区分「全忙」与「根本没号」：再数一遍总可用数。
			if n, cerr := s.repo.CountUsable(ctx); cerr == nil && n == 0 {
				return nil, errNoAccount
			}
			if len(extra) > 0 {
				// 已经排除了一批失败账号，可能只是这批被排除完了。
				return nil, errNoAccount
			}
			return nil, repo.ErrNotFound // 全忙，可等待
		}
		return nil, err
	}
	s.busy[p.ID] = true
	return p, nil
}

// Release 释放占用的账号。
func (s *PoolGptService) Release(id uint64) {
	if id == 0 {
		return
	}
	s.busyMu.Lock()
	delete(s.busy, id)
	s.busyMu.Unlock()
}

// AcquireUsable 为「号池生图」申请一个可用账号并解出 access_token（一号一并发，随机挑号）。
//   - id>0 指定账号；否则随机挑一个未占用的。
//   - extra 中的账号会被跳过（出错切号用）。
//   - 当所有账号都在忙时会排队等待（最多 ~5 分钟），而不是直接失败。
//
// 成功返回的账号处于「占用中」，调用方用完后必须 Release。
func (s *PoolGptService) AcquireUsable(ctx context.Context, id uint64, proxyURL string, extra []uint64) (*UsableToken, error) {
	deadline := time.Now().Add(5 * time.Minute)
	for {
		p, err := s.reserve(ctx, id, extra)
		if err == repo.ErrNotFound {
			// 全忙，等空闲号。
			if time.Now().After(deadline) {
				return nil, fmt.Errorf("号池账号都在忙，等待超时（建议导入更多账号）")
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(2 * time.Second):
			}
			continue
		}
		if err != nil {
			return nil, err
		}
		at := s.dec(p.AccessTokenEnc)
		// 临近过期（<60s）且有 RT → 试着刷新一次（锁外执行）。
		if exp, ok := jwtExpUnix(at); ok && time.Now().Unix() >= exp-60 {
			if s.dec(p.RefreshTokenEnc) != "" {
				if _, rerr := s.RefreshOne(ctx, p.ID, proxyURL); rerr == nil {
					if np, e := s.repo.GetByID(ctx, p.ID); e == nil {
						p = np
						at = s.dec(p.AccessTokenEnc)
					}
				}
			}
		}
		if at == "" {
			s.Release(p.ID)
			return nil, fmt.Errorf("账号 %s 未取得 access_token", p.Email)
		}
		out := &UsableToken{ID: p.ID, Email: p.Email, AccessToken: at}
		if p.ChatGPTAccountID != nil {
			out.AccountID = *p.ChatGPTAccountID
		}
		if out.AccountID == "" {
			out.AccountID = chatgptAccountIDFromClaims(jwtClaims(at))
		}
		if p.ExpiresAt != nil {
			out.ExpiresAt = p.ExpiresAt.UnixMilli()
		}
		return out, nil
	}
}

// Detail 返回指定账号的明文凭证（编辑弹窗用）。
func (s *PoolGptService) Detail(ctx context.Context, id uint64) (*dto.GptPoolDetailResp, error) {
	p, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return &dto.GptPoolDetailResp{
		ID:           p.ID,
		Email:        p.Email,
		AccessToken:  s.dec(p.AccessTokenEnc),
		RefreshToken: s.dec(p.RefreshTokenEnc),
		IDToken:      s.dec(p.IDTokenEnc),
	}, nil
}

// atValidUntil 返回 access_token 的到期 unix 秒（0=无法判断/已过期）。
func atValidSeconds(at string) (int64, bool) {
	if at == "" {
		return 0, false
	}
	if exp, ok := jwtExpUnix(at); ok {
		return exp, time.Now().Unix() < exp
	}
	return 0, false
}

// RefreshOne 用 refresh_token 刷新指定账号的 access_token，并更新有效期。
func (s *PoolGptService) RefreshOne(ctx context.Context, id uint64, proxyURL string) (*dto.GptRefreshResp, error) {
	p, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	rt := s.dec(p.RefreshTokenEnc)
	if rt == "" {
		// 没有 RT 但 AT 仍有效 → 账号可用，无需刷新。
		if exp, ok := atValidSeconds(s.dec(p.AccessTokenEnc)); ok {
			return &dto.GptRefreshResp{OK: true, ExpiresAt: exp * 1000, Message: "无 refresh_token，但 access_token 仍有效"}, nil
		}
		return nil, fmt.Errorf("账号未配置 refresh_token，无法刷新")
	}
	// 解析签发 client_id：优先用入库记录，其次从当前 access_token 的 claims 取，
	// 最后才退回默认 codex client。不同来源（codex / sub2api）client_id 不同，
	// 用错会被 OpenAI 拒（invalid_grant / 400）。
	clientID := ""
	if p.OAuthClientID != nil {
		clientID = strings.TrimSpace(*p.OAuthClientID)
	}
	if clientID == "" {
		if at := s.dec(p.AccessTokenEnc); at != "" {
			clientID = clientIDFromClaims(jwtClaims(at))
		}
	}
	tr, err := refreshOpenAIToken(ctx, rt, clientID, proxyURL)
	if err != nil {
		msg := err.Error()
		if len(msg) > 250 {
			msg = msg[:250]
		}
		// RT 已被使用过（轮换型 RT 单次有效）/ invalid_grant：若 access_token 仍在有效期内，
		// 账号本身仍可用，不算失败、不标失效，只是无法续期而已。
		if exp, ok := atValidSeconds(s.dec(p.AccessTokenEnc)); ok {
			_ = s.repo.Update(ctx, id, map[string]any{
				"error_message": msg,
				"status":        model.GPTStatusValid,
			})
			return &dto.GptRefreshResp{
				OK:        true,
				ExpiresAt: exp * 1000,
				Message:   "RT 无法续期（可能已被使用过），但 access_token 仍在有效期内，账号可用",
			}, nil
		}
		// AT 也过期了 → 确实失效。
		_ = s.repo.Update(ctx, id, map[string]any{
			"error_message": msg,
			"status":        model.GPTStatusInvalid,
		})
		return nil, err
	}
	now := time.Now().UTC()
	fields := map[string]any{
		"access_token_enc": s.enc(tr.AccessToken),
		"last_refresh_at":  now,
		"error_message":    "",
		"status":           model.GPTStatusValid,
	}
	var expMilli int64
	if exp, ok := jwtExpUnix(tr.AccessToken); ok {
		t := time.Unix(exp, 0).UTC()
		fields["expires_at"] = t
		expMilli = t.UnixMilli()
	} else if tr.ExpiresIn > 0 {
		t := now.Add(time.Duration(tr.ExpiresIn) * time.Second)
		fields["expires_at"] = t
		expMilli = t.UnixMilli()
	}
	if strings.TrimSpace(tr.RefreshToken) != "" {
		fields["refresh_token_enc"] = s.enc(tr.RefreshToken)
	}
	if tr.IDToken != "" {
		fields["id_token_enc"] = s.enc(tr.IDToken)
	}
	if err := s.repo.Update(ctx, id, fields); err != nil {
		return nil, err
	}
	return &dto.GptRefreshResp{OK: true, ExpiresAt: expMilli, RefreshedAt: now.UnixMilli()}, nil
}

// ListRefreshableIDs 列出可刷新账号 ID（withinExpirySeconds 内到期/已过期）。
func (s *PoolGptService) ListRefreshableIDs(ctx context.Context, withinExpirySeconds int64) ([]uint64, error) {
	rows, err := s.repo.ListRefreshable(ctx, withinExpirySeconds, 20000)
	if err != nil {
		return nil, err
	}
	ids := make([]uint64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	return ids, nil
}

// BatchRefresh 批量刷新（ids 为空时刷新全部可刷新账号）。proxyURLFn 按账号返回代理。
func (s *PoolGptService) BatchRefresh(ctx context.Context, ids []uint64, proxyURL string) (*dto.GptBatchRefreshResp, error) {
	if len(ids) == 0 {
		rows, err := s.repo.ListRefreshable(ctx, 0, 20000)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			ids = append(ids, r.ID)
		}
	}
	res := &dto.GptBatchRefreshResp{}
	for _, id := range ids {
		if _, err := s.RefreshOne(ctx, id, proxyURL); err != nil {
			res.Failed++
			if len(res.Errors) < 30 {
				res.Errors = append(res.Errors, fmt.Sprintf("#%d: %v", id, err))
			}
			continue
		}
		res.Refreshed++
	}
	return res, nil
}

// ProbeQuota 查询指定账号额度（必要时先刷新过期 token），并落库。
func (s *PoolGptService) ProbeQuota(ctx context.Context, id uint64, proxyURL string) (*dto.GptQuotaResp, error) {
	p, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	at := s.dec(p.AccessTokenEnc)
	if at == "" {
		return nil, fmt.Errorf("账号未取得 access_token")
	}
	// access_token 过期则先刷新
	if exp, ok := jwtExpUnix(at); ok && time.Now().Unix() >= exp {
		if _, rerr := s.RefreshOne(ctx, id, proxyURL); rerr == nil {
			if np, e := s.repo.GetByID(ctx, id); e == nil {
				at = s.dec(np.AccessTokenEnc)
				p = np
			}
		}
	}
	accountID := ""
	if p.ChatGPTAccountID != nil {
		accountID = *p.ChatGPTAccountID
	}
	probe, err := probeChatGPTQuota(ctx, at, accountID, proxyURL)
	if err != nil {
		return &dto.GptQuotaResp{OK: false, Message: err.Error()}, nil
	}
	now := time.Now().UTC()
	fields := map[string]any{"last_quota_check_at": now}
	if probe.ImageQuotaRemaining != nil {
		fields["image_quota_remaining"] = *probe.ImageQuotaRemaining
	}
	if probe.ImageQuotaTotal != nil {
		fields["image_quota_total"] = *probe.ImageQuotaTotal
	}
	if probe.ImageQuotaResetAt > 0 {
		fields["image_quota_reset_at"] = time.Unix(probe.ImageQuotaResetAt, 0).UTC()
	}
	if probe.PlanType == "" {
		probe.PlanType = planTypeFromClaims(jwtClaims(at))
	}
	if probe.PlanType != "" {
		fields["plan_type"] = probe.PlanType
	}
	_ = s.repo.Update(ctx, id, fields)
	resp := &dto.GptQuotaResp{
		OK:                  true,
		PlanType:            probe.PlanType,
		ImageQuotaRemaining: probe.ImageQuotaRemaining,
		ImageQuotaTotal:     probe.ImageQuotaTotal,
		WeeklyRemaining:     probe.WeeklyRemaining,
		CreditsBalance:      probe.CreditsBalance,
		DefaultModel:        probe.DefaultModel,
		CheckedAt:           now.UnixMilli(),
	}
	if probe.ImageQuotaResetAt > 0 {
		resp.ImageQuotaResetAt = probe.ImageQuotaResetAt * 1000
	}
	if probe.WeeklyResetAt > 0 {
		resp.WeeklyResetAt = probe.WeeklyResetAt * 1000
	}
	return resp, nil
}

// parseGptImport 解析导入文本为 GptPoolCreateReq 列表。
//
// 兼容：
//  1. {"type":"sub2api-data","accounts":[{"name","credentials":{...}}]}
//  2. {"type":"codex","email","access_token","refresh_token","expired","account_id"}
//  3. 扁平单 object / 单 object 数组
func parseGptImport(text string) ([]dto.GptPoolCreateReq, error) {
	trim := strings.TrimSpace(text)
	if trim == "" {
		return nil, fmt.Errorf("导入内容为空")
	}
	// 用流式 decoder 读取「多个相邻的 JSON 文档」——这样同时选多个 .json
	// 文件拼接到一起（中间用换行分隔）也能逐个解析，而不是当成一个非法 JSON。
	dec := json.NewDecoder(strings.NewReader(trim))
	out := make([]dto.GptPoolCreateReq, 0, 8)
	decoded := false
	for {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			if err == io.EOF {
				break
			}
			// 已经成功解出至少一个文档：剩余内容多半是尾部噪声，直接收尾。
			if decoded {
				break
			}
			return nil, fmt.Errorf("无法识别的 JSON 格式")
		}
		decoded = true
		out = append(out, parseGptImportDoc(raw)...)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("无法识别的 JSON 格式")
	}
	return out, nil
}

// parseGptImportDoc 解析单个 JSON 文档（object / sub2api-data / 数组）。
func parseGptImportDoc(raw json.RawMessage) []dto.GptPoolCreateReq {
	b := []byte(strings.TrimSpace(string(raw)))
	if len(b) == 0 {
		return nil
	}
	switch b[0] {
	case '{':
		// sub2api-data：accounts[]
		var top map[string]json.RawMessage
		if err := json.Unmarshal(b, &top); err == nil {
			if accRaw, ok := top["accounts"]; ok {
				var accs []map[string]any
				if err := json.Unmarshal(accRaw, &accs); err == nil {
					res := make([]dto.GptPoolCreateReq, 0, len(accs))
					for _, a := range accs {
						res = append(res, gptReqFromObject(a))
					}
					return res
				}
			}
		}
		// 单 object（codex 单文件 / 扁平）
		var obj map[string]any
		if err := json.Unmarshal(b, &obj); err == nil {
			return []dto.GptPoolCreateReq{gptReqFromObject(obj)}
		}
	case '[':
		var arr []map[string]any
		if err := json.Unmarshal(b, &arr); err == nil {
			res := make([]dto.GptPoolCreateReq, 0, len(arr))
			for _, a := range arr {
				res = append(res, gptReqFromObject(a))
			}
			return res
		}
	}
	return nil
}

func gptReqFromObject(blob map[string]any) dto.GptPoolCreateReq {
	out := dto.GptPoolCreateReq{}
	pick := func(m map[string]any, keys ...string) string {
		for _, k := range keys {
			if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
		return ""
	}
	out.Email = pick(blob, "email", "name", "username")
	out.Password = pick(blob, "password")
	out.AccessToken = pick(blob, "access_token", "accessToken")
	out.RefreshToken = pick(blob, "refresh_token", "refreshToken")
	out.IDToken = pick(blob, "id_token", "idToken")
	out.APIKey = pick(blob, "api_key", "apiKey")
	out.OAuthClientID = pick(blob, "oauth_client_id", "client_id", "clientId")
	out.PlanType = pick(blob, "plan_type", "platform")
	out.ChatGPTAccountID = pick(blob, "chatgpt_account_id", "account_id", "accountId")

	if cred, ok := blob["credentials"].(map[string]any); ok {
		if out.AccessToken == "" {
			out.AccessToken = pick(cred, "access_token", "accessToken")
		}
		if out.RefreshToken == "" {
			out.RefreshToken = pick(cred, "refresh_token", "refreshToken")
		}
		if out.IDToken == "" {
			out.IDToken = pick(cred, "id_token", "idToken")
		}
		if out.Email == "" {
			out.Email = pick(cred, "email")
		}
		if out.ChatGPTAccountID == "" {
			out.ChatGPTAccountID = pick(cred, "chatgpt_account_id", "account_id")
		}
		if out.ExpiresAt == 0 {
			out.ExpiresAt = parseExpiryToMilli(cred["expires_at"])
		}
	}
	if out.PlanType == "openai" {
		out.PlanType = "" // sub2api 的 platform=openai 不是套餐类型
	}
	if out.ExpiresAt == 0 {
		out.ExpiresAt = parseExpiryToMilli(blob["expires_at"])
	}
	if out.ExpiresAt == 0 {
		out.ExpiresAt = parseExpiryToMilli(blob["expired"])
	}
	if out.ExpiresAt == 0 {
		out.ExpiresAt = parseExpiryToMilli(blob["expires"])
	}
	return out
}

// parseExpiryToMilli 把秒/毫秒/字符串/RFC3339 的到期时间统一成 unix 毫秒。
func parseExpiryToMilli(v any) int64 {
	switch x := v.(type) {
	case float64:
		return normalizeUnixToMilli(int64(x))
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return 0
		}
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return normalizeUnixToMilli(n)
		}
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t.UnixMilli()
		}
	}
	return 0
}

func normalizeUnixToMilli(v int64) int64 {
	if v <= 0 {
		return 0
	}
	if v < 1_000_000_000_000 {
		return v * 1000
	}
	return v
}
