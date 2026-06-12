package main

import (
	"context"
	"errors"

	"web2img/internal/reg/dto"
	"web2img/internal/reg/model"
	"web2img/internal/reg/regkit/mailbox"
	"web2img/internal/reg/repo"
)

var errRegUnavailable = errors.New("注册子系统未初始化")

// ---- 分页结果 ----

type MailPoolPage struct {
	Items []*dto.MailPoolResp `json:"items"`
	Total int64               `json:"total"`
}

type RegisterTaskPage struct {
	Items []*dto.RegisterTaskResp `json:"items"`
	Total int64                   `json:"total"`
}

type GptPoolPage struct {
	Items []*dto.GptPoolResp `json:"items"`
	Total int64              `json:"total"`
}

func (a *App) regCtx() context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}

// ======================= 邮箱管理 =======================

func (a *App) RegMailList(status, mode, keyword string, page, pageSize int) (*MailPoolPage, error) {
	if a.reg == nil {
		return nil, errRegUnavailable
	}
	items, total, err := a.reg.MailSvc.List(a.regCtx(), repo.MailPoolFilter{
		Status: status, Mode: mode, Keyword: keyword, Page: page, PageSize: pageSize,
	})
	if err != nil {
		return nil, err
	}
	return &MailPoolPage{Items: items, Total: total}, nil
}

func (a *App) RegMailStats() (*dto.MailPoolStatsResp, error) {
	if a.reg == nil {
		return nil, errRegUnavailable
	}
	return a.reg.MailSvc.Stats(a.regCtx())
}

func (a *App) RegMailImport(text, mode, separator string) (*dto.MailPoolImportResult, error) {
	if a.reg == nil {
		return nil, errRegUnavailable
	}
	return a.reg.MailSvc.Import(a.regCtx(), text, mode, separator)
}

func (a *App) RegMailDelete(id uint64) error {
	if a.reg == nil {
		return errRegUnavailable
	}
	return a.reg.MailSvc.Delete(a.regCtx(), id)
}

// RegMailUpdate 编辑邮箱；password / refreshToken 传空表示不修改。
func (a *App) RegMailUpdate(id uint64, email, password, clientID, refreshToken, mode, status string) error {
	if a.reg == nil {
		return errRegUnavailable
	}
	return a.reg.MailSvc.Update(a.regCtx(), id, email, password, clientID, refreshToken, mode, status)
}

// RegMailFetch 收取指定邮箱最近邮件（只读预览）。
func (a *App) RegMailFetch(id uint64, limit int) ([]mailbox.MailMessage, error) {
	if a.reg == nil {
		return nil, errRegUnavailable
	}
	if limit <= 0 {
		limit = 15
	}
	return a.reg.MailFetch(a.regCtx(), id, limit)
}

func (a *App) RegMailBatchDelete(ids []uint64) (int64, error) {
	if a.reg == nil {
		return 0, errRegUnavailable
	}
	return a.reg.MailSvc.BatchDelete(a.regCtx(), ids)
}

func (a *App) RegMailReset(ids []uint64) (int64, error) {
	if a.reg == nil {
		return 0, errRegUnavailable
	}
	return a.reg.MailSvc.Reset(a.regCtx(), ids)
}

func (a *App) RegMailDeleteByStatus(status string) (int64, error) {
	if a.reg == nil {
		return 0, errRegUnavailable
	}
	return a.reg.MailSvc.DeleteByStatus(a.regCtx(), status)
}

func (a *App) RegMailClearAll() (int64, error) {
	if a.reg == nil {
		return 0, errRegUnavailable
	}
	return a.reg.MailSvc.ClearAll(a.regCtx())
}

// ======================= 号池注册任务 =======================

func (a *App) RegTaskList(provider, status, keyword string, page, pageSize int) (*RegisterTaskPage, error) {
	if a.reg == nil {
		return nil, errRegUnavailable
	}
	items, total, err := a.reg.TaskSvc.List(a.regCtx(), &dto.RegisterTaskListReq{
		Provider: provider, Status: status, Keyword: keyword, Page: page, PageSize: pageSize,
	})
	if err != nil {
		return nil, err
	}
	return &RegisterTaskPage{Items: items, Total: total}, nil
}

func (a *App) RegTaskStats(provider string) (*dto.RegisterTaskStatsResp, error) {
	if a.reg == nil {
		return nil, errRegUnavailable
	}
	return a.reg.TaskSvc.Stats(a.regCtx(), provider)
}

// RegTaskCreate 创建注册任务。provider 默认 gpt。count<=0 视为 1。
func (a *App) RegTaskCreate(provider string, count int, payload map[string]any) (*dto.RegisterTaskCreateResp, error) {
	if a.reg == nil {
		return nil, errRegUnavailable
	}
	if provider == "" {
		provider = "gpt"
	}
	return a.reg.TaskSvc.Create(a.regCtx(), &dto.RegisterTaskCreateReq{
		Provider: provider, Count: count, Payload: payload,
	})
}

func (a *App) RegTaskCancel(id uint64) error {
	if a.reg == nil {
		return errRegUnavailable
	}
	return a.reg.TaskSvc.Cancel(a.regCtx(), id)
}

func (a *App) RegTaskDelete(id uint64) error {
	if a.reg == nil {
		return errRegUnavailable
	}
	return a.reg.TaskSvc.Delete(a.regCtx(), id)
}

func (a *App) RegTaskPurge(provider string) (int64, error) {
	if a.reg == nil {
		return 0, errRegUnavailable
	}
	return a.reg.TaskSvc.Purge(a.regCtx(), provider)
}

func (a *App) RegTaskLogs(taskID uint64, level string, limit int) ([]*dto.RegisterTaskLogResp, error) {
	if a.reg == nil {
		return nil, errRegUnavailable
	}
	if limit <= 0 {
		limit = 200
	}
	rows, err := a.reg.TaskSvc.LogsList(a.regCtx(), taskID, "", level, limit)
	if err != nil {
		return nil, err
	}
	out := make([]*dto.RegisterTaskLogResp, 0, len(rows))
	for _, r := range rows {
		out = append(out, registerLogToResp(r))
	}
	return out, nil
}

func registerLogToResp(m *model.RegisterTaskLog) *dto.RegisterTaskLogResp {
	r := &dto.RegisterTaskLogResp{
		ID:        m.ID,
		TaskID:    m.TaskID,
		Provider:  m.Provider,
		Level:     m.Level,
		CreatedAt: m.CreatedAt.UnixMilli(),
	}
	if m.Step != nil {
		r.Step = *m.Step
	}
	if m.Progress != nil {
		r.Progress = *m.Progress
	}
	if m.Message != nil {
		r.Message = *m.Message
	}
	return r
}

// ======================= GPT 号池 =======================

func (a *App) RegGptList(status, keyword string, page, pageSize int) (*GptPoolPage, error) {
	if a.reg == nil {
		return nil, errRegUnavailable
	}
	items, total, err := a.reg.GptSvc.List(a.regCtx(), repo.PoolGptFilter{
		Status: status, Keyword: keyword, Page: page, PageSize: pageSize,
	})
	if err != nil {
		return nil, err
	}
	return &GptPoolPage{Items: items, Total: total}, nil
}

func (a *App) RegGptStats() (*dto.GptPoolStatsResp, error) {
	if a.reg == nil {
		return nil, errRegUnavailable
	}
	return a.reg.GptSvc.Stats(a.regCtx())
}

func (a *App) RegGptDelete(id uint64) error {
	if a.reg == nil {
		return errRegUnavailable
	}
	return a.reg.GptSvc.Delete(a.regCtx(), id)
}

func (a *App) RegGptBatchDelete(ids []uint64) (int64, error) {
	if a.reg == nil {
		return 0, errRegUnavailable
	}
	return a.reg.GptSvc.DeleteByIDs(a.regCtx(), ids)
}

func (a *App) RegGptExport(scope string, ids []uint64) (string, error) {
	if a.reg == nil {
		return "", errRegUnavailable
	}
	return a.reg.GptExport(a.regCtx(), scope, ids)
}

// RegGptImport 批量导入号池（兼容 sub2api-data / codex 单文件 / 扁平 JSON / 数组）。
func (a *App) RegGptImport(text string) (*dto.GptPoolImportResult, error) {
	if a.reg == nil {
		return nil, errRegUnavailable
	}
	return a.reg.GptImport(a.regCtx(), text)
}

// RegGptUpdate 编辑号池行（token 留空=保留旧值，status 留空=不改）。
func (a *App) RegGptUpdate(id uint64, status, notes, password, accessToken, refreshToken, idToken string, expiresAt int64) error {
	if a.reg == nil {
		return errRegUnavailable
	}
	return a.reg.GptUpdate(a.regCtx(), id, &dto.GptPoolUpdateReq{
		Status:       status,
		Notes:        notes,
		Password:     password,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		IDToken:      idToken,
		ExpiresAt:    expiresAt,
	})
}

// RegGptDeleteInvalid 删除所有失效号。
func (a *App) RegGptDeleteInvalid() (int64, error) {
	if a.reg == nil {
		return 0, errRegUnavailable
	}
	return a.reg.GptDeleteInvalid(a.regCtx())
}

// RegGptRefresh 刷新单个号的有效期。
func (a *App) RegGptRefresh(id uint64) (*dto.GptRefreshResp, error) {
	if a.reg == nil {
		return nil, errRegUnavailable
	}
	return a.reg.GptRefresh(a.regCtx(), id)
}

// RegGptBatchRefresh 批量刷新有效期（ids 为空=全部可刷新）。
func (a *App) RegGptBatchRefresh(ids []uint64) (*dto.GptBatchRefreshResp, error) {
	if a.reg == nil {
		return nil, errRegUnavailable
	}
	return a.reg.GptBatchRefresh(a.regCtx(), ids)
}

// RegGptQuota 查询单个号的额度。
func (a *App) RegGptQuota(id uint64) (*dto.GptQuotaResp, error) {
	if a.reg == nil {
		return nil, errRegUnavailable
	}
	return a.reg.GptProbeQuota(a.regCtx(), id)
}

// RegGptDetail 返回单个号的明文凭证（编辑弹窗预填）。
func (a *App) RegGptDetail(id uint64) (*dto.GptPoolDetailResp, error) {
	if a.reg == nil {
		return nil, errRegUnavailable
	}
	return a.reg.GptDetail(a.regCtx(), id)
}

// ======================= 配置 =======================

func (a *App) RegConfig() (map[string]string, error) {
	if a.reg == nil {
		return nil, errRegUnavailable
	}
	return a.reg.Config(a.regCtx())
}

func (a *App) RegSetConfig(kv map[string]string) error {
	if a.reg == nil {
		return errRegUnavailable
	}
	return a.reg.SetConfig(a.regCtx(), kv)
}
