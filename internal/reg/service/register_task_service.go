package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"web2img/internal/reg/dto"
	"web2img/internal/reg/model"
	"web2img/internal/reg/repo"
)

// RegisterDispatcher 各 provider 的注册执行器。
type RegisterDispatcher interface {
	Run(ctx context.Context, svc *RegisterTaskService, task *model.RegisterTask) error
}

// taskSubmitter 把 taskID 投到 worker pool 的最小接口。
type taskSubmitter interface {
	Submit(taskID uint64)
}

// RegisterTaskService 号池注册任务服务。
type RegisterTaskService struct {
	repo          *repo.RegisterTaskRepo
	logRepo       *repo.RegisterTaskLogRepo
	mailPoolRepo  *repo.MailPoolRepo
	dispatchers   map[string]RegisterDispatcher
	submitter     taskSubmitter
	providerCache sync.Map
}

// NewRegisterTaskService 构造。
func NewRegisterTaskService(r *repo.RegisterTaskRepo, logRepo *repo.RegisterTaskLogRepo, mailPoolRepo *repo.MailPoolRepo) *RegisterTaskService {
	return &RegisterTaskService{
		repo:         r,
		logRepo:      logRepo,
		mailPoolRepo: mailPoolRepo,
		dispatchers:  make(map[string]RegisterDispatcher),
	}
}

// SetSubmitter 注入 worker pool。
func (s *RegisterTaskService) SetSubmitter(sub taskSubmitter) { s.submitter = sub }

// RegisterDispatcher 注册一个 provider 的执行器。
func (s *RegisterTaskService) RegisterDispatcher(provider string, d RegisterDispatcher) {
	s.dispatchers[provider] = d
}

// RecoverPending 启动时把僵死 running 任务标 failed，并重投 pending。
func (s *RegisterTaskService) RecoverPending(ctx context.Context) (running int64, requeued int64, err error) {
	if r, e := s.markRunningAsFailed(ctx, "进程重启时仍在 running，已自动标记失败"); e != nil {
		return 0, 0, e
	} else {
		running = r
	}
	if s.submitter == nil {
		return running, 0, nil
	}
	pendings, _, e := s.repo.List(ctx, repo.RegisterTaskFilter{Status: model.RegisterTaskPending, Page: 1, PageSize: 200})
	if e != nil {
		return running, 0, e
	}
	for _, p := range pendings {
		s.submitter.Submit(p.ID)
		requeued++
	}
	return running, requeued, nil
}

func (s *RegisterTaskService) markRunningAsFailed(ctx context.Context, reason string) (int64, error) {
	now := time.Now().UTC()
	rows, _, err := s.repo.List(ctx, repo.RegisterTaskFilter{Status: model.RegisterTaskRunning, Page: 1, PageSize: 1000})
	if err != nil {
		return 0, err
	}
	var n int64
	for _, r := range rows {
		_ = s.repo.Update(ctx, r.ID, map[string]any{
			"status":      model.RegisterTaskFailed,
			"error":       reason,
			"finished_at": now,
		})
		n++
	}
	return n, nil
}

// List 列表。
func (s *RegisterTaskService) List(ctx context.Context, req *dto.RegisterTaskListReq) ([]*dto.RegisterTaskResp, int64, error) {
	items, total, err := s.repo.List(ctx, repo.RegisterTaskFilter{
		Provider: req.Provider,
		Status:   req.Status,
		Keyword:  strings.TrimSpace(req.Keyword),
		Page:     req.Page,
		PageSize: req.PageSize,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]*dto.RegisterTaskResp, 0, len(items))
	for _, it := range items {
		out = append(out, registerTaskToResp(it))
	}
	return out, total, nil
}

// Stats 状态分布。
func (s *RegisterTaskService) Stats(ctx context.Context, provider string) (*dto.RegisterTaskStatsResp, error) {
	m, err := s.repo.Stats(ctx, provider)
	if err != nil {
		return nil, err
	}
	return &dto.RegisterTaskStatsResp{
		Total:     m["total"],
		Pending:   m[model.RegisterTaskPending],
		Running:   m[model.RegisterTaskRunning],
		Success:   m[model.RegisterTaskSuccess],
		Failed:    m[model.RegisterTaskFailed],
		Cancelled: m[model.RegisterTaskCancelled],
	}, nil
}

// Create 创建一个或多个注册任务。
func (s *RegisterTaskService) Create(ctx context.Context, req *dto.RegisterTaskCreateReq) (*dto.RegisterTaskCreateResp, error) {
	count := req.Count
	if count <= 0 {
		count = 1
	}
	if count > 5000 {
		count = 5000
	}
	if req.MailID != nil && count != 1 {
		return nil, errors.New("指定 mail_id 时只能创建 1 个任务")
	}
	payloadBytes, err := json.Marshal(req.Payload)
	if err != nil {
		return nil, err
	}
	resp := &dto.RegisterTaskCreateResp{IDs: make([]uint64, 0, count)}
	for i := 0; i < count; i++ {
		t := &model.RegisterTask{
			Provider: req.Provider,
			Status:   model.RegisterTaskPending,
			Payload:  payloadBytes,
		}
		if req.MailID != nil {
			t.MailID = req.MailID
		}
		if err := s.repo.Create(ctx, t); err != nil {
			return nil, err
		}
		resp.IDs = append(resp.IDs, t.ID)
		if s.submitter != nil {
			s.submitter.Submit(t.ID)
		} else {
			go s.RunTask(context.Background(), t.ID)
		}
	}
	resp.Created = len(resp.IDs)
	return resp, nil
}

// Cancel 请求取消任务。
func (s *RegisterTaskService) Cancel(ctx context.Context, id uint64) error {
	m, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	switch m.Status {
	case model.RegisterTaskPending:
		return s.repo.Update(ctx, id, map[string]any{
			"status":      model.RegisterTaskCancelled,
			"finished_at": time.Now().UTC(),
		})
	case model.RegisterTaskRunning:
		return s.repo.MarkCancelRequested(ctx, id)
	default:
		return errors.New("任务已结束，无需取消")
	}
}

// Delete 软删任务。
func (s *RegisterTaskService) Delete(ctx context.Context, id uint64) error {
	return s.repo.SoftDelete(ctx, id)
}

// Purge 批量清理已结束任务。
func (s *RegisterTaskService) Purge(ctx context.Context, provider string) (int64, error) {
	return s.repo.Purge(ctx, repo.PurgeFilter{Provider: provider})
}

// MarkRunning worker 取到任务后调用。
func (s *RegisterTaskService) MarkRunning(ctx context.Context, id uint64, step string) error {
	now := time.Now().UTC()
	if err := s.repo.Update(ctx, id, map[string]any{
		"status":     model.RegisterTaskRunning,
		"step":       step,
		"started_at": now,
	}); err != nil {
		return err
	}
	s.appendLog(ctx, id, model.RegisterLogInfo, step, 0, "任务开始执行")
	return nil
}

// UpdateProgress 推进进度。
func (s *RegisterTaskService) UpdateProgress(ctx context.Context, id uint64, step string, progress uint8) error {
	if err := s.repo.Update(ctx, id, map[string]any{"step": step, "progress": progress}); err != nil {
		return err
	}
	s.appendLog(ctx, id, model.RegisterLogInfo, step, progress, "")
	return nil
}

// LogInfo / LogWarn / LogError 自由文本日志。
func (s *RegisterTaskService) LogInfo(ctx context.Context, id uint64, message string) {
	s.appendLog(ctx, id, model.RegisterLogInfo, "", 0, message)
}
func (s *RegisterTaskService) LogWarn(ctx context.Context, id uint64, message string) {
	s.appendLog(ctx, id, model.RegisterLogWarn, "", 0, message)
}
func (s *RegisterTaskService) LogError(ctx context.Context, id uint64, message string) {
	s.appendLog(ctx, id, model.RegisterLogError, "", 0, message)
}

func (s *RegisterTaskService) appendLog(ctx context.Context, taskID uint64, level, step string, progress uint8, message string) {
	if s.logRepo == nil {
		return
	}
	provider := s.lookupProvider(ctx, taskID)
	row := &model.RegisterTaskLog{TaskID: taskID, Provider: provider, Level: level}
	if step != "" {
		st := step
		row.Step = &st
	}
	if progress > 0 {
		p := progress
		row.Progress = &p
	}
	if message != "" {
		if len(message) > 8000 {
			message = message[:8000] + "…"
		}
		row.Message = &message
	}
	_ = s.logRepo.Insert(ctx, row)
}

func (s *RegisterTaskService) lookupProvider(ctx context.Context, taskID uint64) string {
	if v, ok := s.providerCache.Load(taskID); ok {
		if str, ok := v.(string); ok {
			return str
		}
	}
	t, err := s.repo.GetByID(ctx, taskID)
	if err != nil || t == nil {
		return ""
	}
	s.providerCache.Store(taskID, t.Provider)
	return t.Provider
}

// AttachMail 把领取到的邮箱绑定到任务。
func (s *RegisterTaskService) AttachMail(ctx context.Context, id, mailID uint64, email string) error {
	return s.repo.Update(ctx, id, map[string]any{"mail_id": mailID, "email": email})
}

// FinishSuccess 标记成功。
func (s *RegisterTaskService) FinishSuccess(ctx context.Context, id, poolAccountID uint64, result map[string]any) error {
	now := time.Now().UTC()
	resultBytes, _ := json.Marshal(result)
	if err := s.repo.Update(ctx, id, map[string]any{
		"status":          model.RegisterTaskSuccess,
		"step":            "done",
		"progress":        100,
		"result":          resultBytes,
		"finished_at":     now,
		"pool_account_id": poolAccountID,
	}); err != nil {
		return err
	}
	s.appendLog(ctx, id, model.RegisterLogInfo, "done", 100, fmt.Sprintf("注册成功，写入号池行 ID=%d", poolAccountID))
	s.providerCache.Delete(id)
	return nil
}

// FinishFailed 标记失败。
func (s *RegisterTaskService) FinishFailed(ctx context.Context, id uint64, errMsg string) error {
	now := time.Now().UTC()
	short := errMsg
	if len(short) > 480 {
		short = short[:480]
	}
	if err := s.repo.Update(ctx, id, map[string]any{
		"status":      model.RegisterTaskFailed,
		"error":       short,
		"finished_at": now,
	}); err != nil {
		return err
	}
	s.appendLog(ctx, id, model.RegisterLogError, "failed", 0, errMsg)
	s.providerCache.Delete(id)
	return nil
}

// FinishCancelled 标记取消。
func (s *RegisterTaskService) FinishCancelled(ctx context.Context, id uint64) error {
	now := time.Now().UTC()
	if err := s.repo.Update(ctx, id, map[string]any{
		"status":      model.RegisterTaskCancelled,
		"finished_at": now,
	}); err != nil {
		return err
	}
	s.appendLog(ctx, id, model.RegisterLogWarn, "cancelled", 0, "任务被取消")
	s.providerCache.Delete(id)
	return nil
}

// LogsList 拉取最近日志。
func (s *RegisterTaskService) LogsList(ctx context.Context, taskID uint64, provider, level string, limit int) ([]*model.RegisterTaskLog, error) {
	if s.logRepo == nil {
		return nil, nil
	}
	return s.logRepo.List(ctx, repo.RegisterTaskLogFilter{TaskID: taskID, Provider: provider, Level: level, Limit: limit})
}

// LogsPurge 清理日志。
func (s *RegisterTaskService) LogsPurge(ctx context.Context, taskID uint64, provider, level string) (int64, error) {
	if s.logRepo == nil {
		return 0, nil
	}
	return s.logRepo.Purge(ctx, repo.RegisterTaskLogFilter{TaskID: taskID, Provider: provider, Level: level})
}

// MailPoolRepo 暴露给 dispatcher。
func (s *RegisterTaskService) MailPoolRepo() *repo.MailPoolRepo { return s.mailPoolRepo }

// RunTask 执行单个注册任务。
func (s *RegisterTaskService) RunTask(ctx context.Context, id uint64) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("register task panic id=%d: %v", id, r)
			_ = s.FinishFailed(context.Background(), id, fmt.Sprintf("panic: %v", r))
		}
	}()
	if ctx == nil {
		ctx = context.Background()
	}
	m, err := s.repo.GetByID(ctx, id)
	if err != nil {
		log.Printf("register task get failed id=%d err=%v", id, err)
		return
	}
	if m.Status != model.RegisterTaskPending {
		return
	}
	s.providerCache.Store(id, m.Provider)
	d, ok := s.dispatchers[m.Provider]
	if !ok || d == nil {
		_ = s.FinishFailed(ctx, id, "未实现该 provider 的注册 dispatcher")
		return
	}
	if err := s.MarkRunning(ctx, id, "start"); err != nil {
		log.Printf("register task mark running failed id=%d err=%v", id, err)
	}
	if err := d.Run(ctx, s, m); err != nil {
		_ = s.FinishFailed(ctx, id, err.Error())
	}
}

func registerTaskToResp(m *model.RegisterTask) *dto.RegisterTaskResp {
	r := &dto.RegisterTaskResp{
		ID:              m.ID,
		Provider:        m.Provider,
		Status:          m.Status,
		Progress:        m.Progress,
		CancelRequested: m.CancelRequested,
		CreatedAt:       m.CreatedAt.UnixMilli(),
		UpdatedAt:       m.UpdatedAt.UnixMilli(),
	}
	if m.Step != nil {
		r.Step = *m.Step
	}
	if m.MailID != nil {
		r.MailID = *m.MailID
	}
	if m.Email != nil {
		r.Email = *m.Email
	}
	if m.Error != nil {
		r.Error = *m.Error
	}
	if m.PoolAccountID != nil {
		r.PoolAccountID = *m.PoolAccountID
	}
	if m.StartedAt != nil {
		r.StartedAt = m.StartedAt.UnixMilli()
	}
	if m.FinishedAt != nil {
		r.FinishedAt = m.FinishedAt.UnixMilli()
	}
	if len(m.Payload) > 0 {
		var p map[string]any
		_ = json.Unmarshal(m.Payload, &p)
		r.Payload = p
	}
	if len(m.Result) > 0 {
		var p map[string]any
		_ = json.Unmarshal(m.Result, &p)
		r.Result = p
	}
	return r
}
