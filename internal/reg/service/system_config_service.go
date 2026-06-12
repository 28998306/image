// Package service 注册子系统的业务服务层。
package service

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"web2img/internal/reg/repo"
)

// 系统配置 key 常量（与上游对齐，仅保留注册流程用到的）。
const (
	SettingProxyGlobalEnabled = "proxy.global_enabled"
	SettingProxyGlobalID      = "proxy.global_id"
	SettingProxyDynamicURL    = "proxy.dynamic_url"

	SettingMailDefaultBackend = "mail.default_backend"
	SettingMailOutlook        = "mail.outlook"
	SettingMailTempmail       = "mail.tempmail"
	SettingMailCF             = "mail.cf"

	SettingCaptchaProvider          = "captcha.provider"
	SettingCaptchaAPIKey            = "captcha.api_key"
	SettingCaptchaEndpoint          = "captcha.endpoint"
	SettingCaptchaArkoseProvider    = "captcha.arkose.provider"
	SettingCaptchaArkoseAPIKey      = "captcha.arkose.api_key"
	SettingCaptchaArkoseEndpoint    = "captcha.arkose.endpoint"
	SettingCaptchaArkoseFallbacks   = "captcha.arkose.fallbacks"
	SettingCaptchaTurnstileProvider = "captcha.turnstile.provider"
	SettingCaptchaTurnstileAPIKey   = "captcha.turnstile.api_key"
	SettingCaptchaTurnstileEndpoint = "captcha.turnstile.endpoint"
	SettingCaptchaTurnstileFallback = "captcha.turnstile.fallbacks"
)

// CaptchaProviderEntry 单家打码配置（链式 fallback 用）。
type CaptchaProviderEntry struct {
	Provider string `json:"provider"`
	APIKey   string `json:"api_key"`
	Endpoint string `json:"endpoint,omitempty"`
}

// SystemConfigService 系统配置服务，基于 system_config 键值表。
type SystemConfigService struct {
	repo *repo.SystemConfigRepo
}

// NewSystemConfigService 构造。
func NewSystemConfigService(r *repo.SystemConfigRepo) *SystemConfigService {
	return &SystemConfigService{repo: r}
}

// GetString 读字符串，缺失返回 fallback。
func (s *SystemConfigService) GetString(ctx context.Context, key, fallback string) string {
	if s == nil || s.repo == nil {
		return fallback
	}
	v, err := s.repo.Get(ctx, key)
	if err != nil || strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

// GetInt 读整数，缺失/非法返回 fallback。
func (s *SystemConfigService) GetInt(ctx context.Context, key string, fallback int64) int64 {
	raw := s.GetString(ctx, key, "")
	if raw == "" {
		return fallback
	}
	n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return fallback
	}
	return n
}

// GetBool 读布尔。
func (s *SystemConfigService) GetBool(ctx context.Context, key string, fallback bool) bool {
	raw := strings.ToLower(strings.TrimSpace(s.GetString(ctx, key, "")))
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

// GetJSON 读 JSON 对象。
func (s *SystemConfigService) GetJSON(ctx context.Context, key string) map[string]any {
	raw := s.GetString(ctx, key, "")
	if raw == "" {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

// Set 写入配置。
func (s *SystemConfigService) Set(ctx context.Context, key, value string) error {
	if s == nil || s.repo == nil {
		return nil
	}
	return s.repo.Set(ctx, key, value)
}

// GlobalProxyEnabled 是否启用全局代理。
func (s *SystemConfigService) GlobalProxyEnabled(ctx context.Context) bool {
	return s.GetBool(ctx, SettingProxyGlobalEnabled, false)
}

// GlobalProxyID 全局代理 ID。
func (s *SystemConfigService) GlobalProxyID(ctx context.Context) uint64 {
	return uint64(s.GetInt(ctx, SettingProxyGlobalID, 0))
}

// DynamicProxyURL 动态/轮换代理网关 URL（形如 http://user:pass@gateway:port）。
// 配置后，未显式指定 proxy_id 的任务会统一走这个网关，由网关侧每次连接轮换出口 IP。
func (s *SystemConfigService) DynamicProxyURL(ctx context.Context) string {
	return strings.TrimSpace(s.GetString(ctx, SettingProxyDynamicURL, ""))
}

// MailDefaultBackend 默认收件后端。
func (s *SystemConfigService) MailDefaultBackend(ctx context.Context) string {
	return s.GetString(ctx, SettingMailDefaultBackend, "")
}

// captchaTriple 读一组 provider/api_key/endpoint，缺失回落到旧版单组配置。
func (s *SystemConfigService) captchaTriple(ctx context.Context, pKey, kKey, eKey string) (string, string, string) {
	provider := s.GetString(ctx, pKey, "")
	apiKey := s.GetString(ctx, kKey, "")
	endpoint := s.GetString(ctx, eKey, "")
	if provider == "" && apiKey == "" {
		provider = s.GetString(ctx, SettingCaptchaProvider, "")
		apiKey = s.GetString(ctx, SettingCaptchaAPIKey, "")
		endpoint = s.GetString(ctx, SettingCaptchaEndpoint, "")
	}
	return provider, apiKey, endpoint
}

// CaptchaArkose Arkose/FunCaptcha 主配置。
func (s *SystemConfigService) CaptchaArkose(ctx context.Context) (string, string, string) {
	return s.captchaTriple(ctx, SettingCaptchaArkoseProvider, SettingCaptchaArkoseAPIKey, SettingCaptchaArkoseEndpoint)
}

// CaptchaArkoseFallbacks Arkose 备用打码链。
func (s *SystemConfigService) CaptchaArkoseFallbacks(ctx context.Context) []CaptchaProviderEntry {
	return s.captchaFallbacks(ctx, SettingCaptchaArkoseFallbacks)
}

// CaptchaTurnstile Turnstile 主配置。
func (s *SystemConfigService) CaptchaTurnstile(ctx context.Context) (string, string, string) {
	return s.captchaTriple(ctx, SettingCaptchaTurnstileProvider, SettingCaptchaTurnstileAPIKey, SettingCaptchaTurnstileEndpoint)
}

// CaptchaTurnstileFallbacks Turnstile 备用打码链。
func (s *SystemConfigService) CaptchaTurnstileFallbacks(ctx context.Context) []CaptchaProviderEntry {
	return s.captchaFallbacks(ctx, SettingCaptchaTurnstileFallback)
}

func (s *SystemConfigService) captchaFallbacks(ctx context.Context, key string) []CaptchaProviderEntry {
	raw := s.GetString(ctx, key, "")
	if raw == "" {
		return nil
	}
	var out []CaptchaProviderEntry
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}
