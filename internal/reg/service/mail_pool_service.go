package service

import (
	"context"
	"fmt"
	"strings"

	"web2img/internal/reg/crypto"
	"web2img/internal/reg/dto"
	"web2img/internal/reg/model"
	"web2img/internal/reg/repo"
)

// MailPoolService 共享邮箱池服务。
type MailPoolService struct {
	repo *repo.MailPoolRepo
	aes  *crypto.AESGCM
}

// NewMailPoolService 构造。
func NewMailPoolService(r *repo.MailPoolRepo, aes *crypto.AESGCM) *MailPoolService {
	return &MailPoolService{repo: r, aes: aes}
}

// List 列表。
func (s *MailPoolService) List(ctx context.Context, f repo.MailPoolFilter) ([]*dto.MailPoolResp, int64, error) {
	items, total, err := s.repo.List(ctx, f)
	if err != nil {
		return nil, 0, err
	}
	out := make([]*dto.MailPoolResp, 0, len(items))
	for _, it := range items {
		out = append(out, mailPoolToResp(it))
	}
	return out, total, nil
}

// Stats 状态统计。
func (s *MailPoolService) Stats(ctx context.Context) (*dto.MailPoolStatsResp, error) {
	m, err := s.repo.Stats(ctx)
	if err != nil {
		return nil, err
	}
	return &dto.MailPoolStatsResp{
		Total:      m["total"],
		Available:  m[model.MailStatusAvailable],
		InUse:      m[model.MailStatusInUse],
		Registered: m[model.MailStatusRegistered],
		Failed:     m[model.MailStatusFailed],
		Disabled:   m[model.MailStatusDisabled],
	}, nil
}

// Import 批量导入，4 段或 7 段格式。
func (s *MailPoolService) Import(ctx context.Context, text, mode, separator string) (*dto.MailPoolImportResult, error) {
	sep := separator
	if sep == "" {
		sep = "----"
	}
	if mode == "" {
		mode = model.MailModeOutlookGraph
	}
	res := &dto.MailPoolImportResult{}
	batch := make([]*model.MailPool, 0, 64)
	seen := map[string]struct{}{}
	for i, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, sep)
		var email, password, clientID, refresh string
		switch {
		case len(parts) >= 7:
			email = strings.ToLower(strings.TrimSpace(parts[0]))
			password = strings.TrimSpace(parts[1])
			clientID = strings.TrimSpace(parts[5])
			refresh = strings.TrimSpace(strings.Join(parts[6:], sep))
		case len(parts) == 4:
			email = strings.ToLower(strings.TrimSpace(parts[0]))
			password = strings.TrimSpace(parts[1])
			clientID = strings.TrimSpace(parts[2])
			refresh = strings.TrimSpace(parts[3])
		default:
			res.Skipped++
			res.Errors = append(res.Errors, fmt.Sprintf("第 %d 行字段数 %d 不支持（要 4 段或 7 段）", i+1, len(parts)))
			continue
		}
		if email == "" || password == "" || clientID == "" || refresh == "" {
			res.Skipped++
			res.Errors = append(res.Errors, fmt.Sprintf("第 %d 行有空字段", i+1))
			continue
		}
		if _, dup := seen[email]; dup {
			res.Skipped++
			continue
		}
		seen[email] = struct{}{}

		pwEnc, err := s.aes.Encrypt([]byte(password))
		if err != nil {
			return nil, err
		}
		rtEnc, err := s.aes.Encrypt([]byte(refresh))
		if err != nil {
			return nil, err
		}
		batch = append(batch, &model.MailPool{
			Email:           email,
			PasswordEnc:     pwEnc,
			ClientID:        clientID,
			RefreshTokenEnc: rtEnc,
			Mode:            mode,
			Status:          model.MailStatusAvailable,
		})
	}
	if len(batch) > 0 {
		n, err := s.repo.UpsertMany(ctx, batch)
		if err != nil {
			return nil, err
		}
		res.Imported = int(n)
	}
	return res, nil
}

// Update 编辑一条邮箱。password / refreshToken 传空表示保持原值不变。
func (s *MailPoolService) Update(ctx context.Context, id uint64, email, password, clientID, refreshToken, mode, status string) error {
	fields := map[string]any{}
	if e := strings.ToLower(strings.TrimSpace(email)); e != "" {
		fields["email"] = e
	}
	if v := strings.TrimSpace(clientID); v != "" {
		fields["client_id"] = v
	}
	if v := strings.TrimSpace(mode); v != "" {
		fields["mode"] = v
	}
	if v := strings.TrimSpace(status); v != "" {
		fields["status"] = v
	}
	if pw := strings.TrimSpace(password); pw != "" {
		enc, err := s.aes.Encrypt([]byte(pw))
		if err != nil {
			return err
		}
		fields["password_enc"] = enc
	}
	if rt := strings.TrimSpace(refreshToken); rt != "" {
		enc, err := s.aes.Encrypt([]byte(rt))
		if err != nil {
			return err
		}
		fields["refresh_token_enc"] = enc
	}
	if len(fields) == 0 {
		return nil
	}
	return s.repo.Update(ctx, id, fields)
}

// Delete 删除单条。
func (s *MailPoolService) Delete(ctx context.Context, id uint64) error {
	return s.repo.SoftDelete(ctx, id)
}

// BatchDelete 批量删除。
func (s *MailPoolService) BatchDelete(ctx context.Context, ids []uint64) (int64, error) {
	return s.repo.SoftDeleteByIDs(ctx, ids)
}

// Reset 批量重置可用。
func (s *MailPoolService) Reset(ctx context.Context, ids []uint64) (int64, error) {
	return s.repo.ResetByIDs(ctx, ids)
}

// DeleteByStatus 按状态清理。
func (s *MailPoolService) DeleteByStatus(ctx context.Context, status string) (int64, error) {
	return s.repo.SoftDeleteByStatus(ctx, status)
}

// ClearAll 清空全部邮箱（软删）。
func (s *MailPoolService) ClearAll(ctx context.Context) (int64, error) {
	return s.repo.SoftDeleteByFilter(ctx, repo.MailPoolFilter{})
}

func mailPoolToResp(m *model.MailPool) *dto.MailPoolResp {
	r := &dto.MailPoolResp{
		ID:           m.ID,
		Email:        m.Email,
		ClientID:     m.ClientID,
		Mode:         m.Mode,
		Status:       m.Status,
		FailureCount: m.FailureCount,
		ImportedAt:   m.ImportedAt.UnixMilli(),
	}
	if m.LastError != nil {
		r.LastError = *m.LastError
	}
	if m.UsedByProvider != nil {
		r.UsedByProvider = *m.UsedByProvider
	}
	if m.UsedByAccountID != nil {
		r.UsedByAccountID = *m.UsedByAccountID
	}
	if m.UsedAt != nil {
		r.UsedAt = m.UsedAt.UnixMilli()
	}
	if m.RegisteredAt != nil {
		r.RegisteredAt = m.RegisteredAt.UnixMilli()
	}
	return r
}
