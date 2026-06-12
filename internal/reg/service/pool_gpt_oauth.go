package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"web2img/internal/reg/regkit/browser"
)

// defaultOpenAIClientID 是 Codex/Platform OAuth client（注册产出的 token 都用它）。
const defaultOpenAIClientID = "app_EMoamEEZ73f0CkXaXp7hrann"

const openAITokenURL = "https://auth.openai.com/oauth/token"

// openAITokenResponse oauth/token 响应。
type openAITokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	ExpiresIn    int64  `json:"expires_in,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// refreshOpenAIToken 用 refresh_token 兑换新的 access_token。proxyURL 可空（直连）。
func refreshOpenAIToken(ctx context.Context, refreshToken, clientID, proxyURL string) (*openAITokenResponse, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return nil, fmt.Errorf("缺少 refresh_token")
	}
	if clientID == "" {
		clientID = defaultOpenAIClientID
	}
	bc, err := browser.New(browser.Options{ProxyURL: proxyURL, Timeout: 30 * time.Second})
	if err != nil {
		return nil, err
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", clientID)
	form.Set("scope", "openid profile email")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openAITokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "codex-cli/0.91.0")

	resp, err := bc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OpenAI 刷新请求失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return nil, fmt.Errorf("OpenAI 返回 %d: %s", resp.StatusCode, msg)
	}
	var tr openAITokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("解析 Token 响应失败: %w", err)
	}
	if tr.AccessToken == "" {
		return nil, fmt.Errorf("OpenAI 响应缺少 access_token")
	}
	return &tr, nil
}

// jwtClaims 解析 JWT payload（不验签）。失败返回 nil。
func jwtClaims(token string) map[string]any {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) < 2 {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		// 兼容带 padding
		if raw, err = base64.URLEncoding.DecodeString(parts[1]); err != nil {
			return nil
		}
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}

// jwtExpUnix 从 JWT 取 exp（unix 秒）。
func jwtExpUnix(token string) (int64, bool) {
	m := jwtClaims(token)
	if m == nil {
		return 0, false
	}
	if v, ok := m["exp"].(float64); ok {
		return int64(v), true
	}
	return 0, false
}

// planTypeFromClaims 从 access_token 的 auth 命名空间取 plan_type。
func planTypeFromClaims(m map[string]any) string {
	if m == nil {
		return ""
	}
	if auth, ok := m["https://api.openai.com/auth"].(map[string]any); ok {
		if pt, ok := auth["chatgpt_plan_type"].(string); ok {
			return pt
		}
	}
	return ""
}

// clientIDFromClaims 从 access_token 取签发它的 OAuth client_id。
// 不同来源（codex / sub2api / 平台）client_id 不同，刷新 RT 必须用对应的 client_id。
func clientIDFromClaims(m map[string]any) string {
	if m == nil {
		return ""
	}
	if v, ok := m["client_id"].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// chatgptAccountIDFromClaims 从 access_token 取 chatgpt_account_id。
func chatgptAccountIDFromClaims(m map[string]any) string {
	if m == nil {
		return ""
	}
	if auth, ok := m["https://api.openai.com/auth"].(map[string]any); ok {
		if id, ok := auth["chatgpt_account_id"].(string); ok {
			return id
		}
	}
	return ""
}

// quotaProbeResult /wham/usage 探测结果。
//
// ImageQuota* 复用为「主窗口（5 小时）剩余百分比」：Remaining=剩余%（0-100），Total=100。
// Weekly* 为「次窗口（每周）剩余百分比」。
type quotaProbeResult struct {
	DefaultModel        string
	PlanType            string
	ImageQuotaRemaining *int
	ImageQuotaTotal     *int
	ImageQuotaResetAt   int64 // unix 秒（主窗口重置时间）
	WeeklyRemaining     *int
	WeeklyResetAt       int64  // unix 秒（次窗口重置时间）
	CreditsBalance      string // 余额（如有）
}

// codex CLI 请求签名（与 codeximg 出图链路一致，可顺利通过 Cloudflare）。
const (
	codexUsageURL = "https://chatgpt.com/backend-api/wham/usage"
	codexProbeUA  = "codex_cli_rs/0.125.0"
	codexProbeVer = "0.125.0"
)

// whamUsageResp 是 /backend-api/wham/usage 的返回（Codex CLI 每 60s 轮询的用量端点）。
type whamUsageResp struct {
	PlanType  string `json:"plan_type"`
	RateLimit struct {
		Allowed         bool        `json:"allowed"`
		LimitReached    bool        `json:"limit_reached"`
		PrimaryWindow   *whamWindow `json:"primary_window"`
		SecondaryWindow *whamWindow `json:"secondary_window"`
	} `json:"rate_limit"`
	Credits struct {
		HasCredits bool   `json:"has_credits"`
		Unlimited  bool   `json:"unlimited"`
		Balance    string `json:"balance"`
	} `json:"credits"`
}

type whamWindow struct {
	UsedPercent       float64 `json:"used_percent"`
	LimitWindowSecond int64   `json:"limit_window_seconds"`
	ResetAfterSeconds int64   `json:"reset_after_seconds"`
	ResetAt           int64   `json:"reset_at"`
}

// remainingPercent 把 used_percent 转成剩余百分比（0-100，向下取整保守显示）。
func (w *whamWindow) remainingPercent() int {
	if w == nil {
		return 0
	}
	rem := 100 - w.UsedPercent
	if rem < 0 {
		rem = 0
	}
	if rem > 100 {
		rem = 100
	}
	return int(rem)
}

// resetUnix 取窗口重置的 unix 秒；优先 reset_at，否则用 now+reset_after_seconds。
func (w *whamWindow) resetUnix(now time.Time) int64 {
	if w == nil {
		return 0
	}
	if w.ResetAt > 0 {
		return w.ResetAt
	}
	if w.ResetAfterSeconds > 0 {
		return now.Add(time.Duration(w.ResetAfterSeconds) * time.Second).Unix()
	}
	return 0
}

// probeChatGPTQuota 读取账号用量配额。
//
// 走 Codex 的 /backend-api/wham/usage（GET，不消耗出图额度），与出图链路同一通道，
// 因此能避开网页版 conversation/init 的 Cloudflare/Sentinel 拦截（403 挑战页）。
// 返回主窗口（5 小时）/ 次窗口（每周）的剩余百分比与重置时间。
func probeChatGPTQuota(ctx context.Context, accessToken, accountID, proxyURL string) (*quotaProbeResult, error) {
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return nil, fmt.Errorf("access_token 为空")
	}
	bc, err := browser.New(browser.Options{ProxyURL: proxyURL, Timeout: 30 * time.Second})
	if err != nil {
		return nil, err
	}
	// 先 GET chatgpt.com 首页拿 cf_clearance，避免被 Cloudflare 拦。
	bootstrapChatGPTCookies(ctx, bc)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, codexUsageURL, nil)
	if err != nil {
		return nil, err
	}
	setCodexProbeHeaders(req, accessToken, accountID)
	resp, err := bc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wham/usage 请求失败: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(data))
		if len(msg) > 200 {
			msg = msg[:200]
		}
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, fmt.Errorf("wham/usage HTTP 401 未授权：该账号 token 不可用于 Codex 用量查询（多为网页/平台 token 或已失效）")
		}
		return nil, fmt.Errorf("wham/usage HTTP %d: %s", resp.StatusCode, msg)
	}
	var payload whamUsageResp
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("解析 wham/usage 响应失败: %w", err)
	}
	now := time.Now()
	out := &quotaProbeResult{PlanType: strings.TrimSpace(payload.PlanType)}
	if w := payload.RateLimit.PrimaryWindow; w != nil {
		rem := w.remainingPercent()
		total := 100
		out.ImageQuotaRemaining = &rem
		out.ImageQuotaTotal = &total
		out.ImageQuotaResetAt = w.resetUnix(now)
	}
	if w := payload.RateLimit.SecondaryWindow; w != nil {
		rem := w.remainingPercent()
		out.WeeklyRemaining = &rem
		out.WeeklyResetAt = w.resetUnix(now)
	}
	if payload.Credits.Unlimited {
		out.CreditsBalance = "unlimited"
	} else if b := strings.TrimSpace(payload.Credits.Balance); b != "" {
		out.CreditsBalance = b
	}
	return out, nil
}

const chatgptUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36 Edg/143.0.0.0"

// bootstrapChatGPTCookies 访问首页拿 Cloudflare cookie（写入 bc 的 jar）。
func bootstrapChatGPTCookies(ctx context.Context, bc *browser.Client) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://chatgpt.com/", nil)
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", chatgptUA)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	resp, err := bc.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
}

// setCodexProbeHeaders 给 /wham/usage 请求挂上 Codex CLI 签名头（与出图链路一致）。
func setCodexProbeHeaders(req *http.Request, accessToken, accountID string) {
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", codexProbeUA)
	req.Header.Set("originator", "codex_cli_rs")
	req.Header.Set("version", codexProbeVer)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	if accountID != "" {
		req.Header.Set("Chatgpt-Account-Id", accountID)
	}
}
