package mailbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"web2img/internal/reg/model"
)

// OutlookGraphBackend 通过 Microsoft Graph API 拉取邮件。
type OutlookGraphBackend struct{}

// NewOutlookGraphBackend 构造。
func NewOutlookGraphBackend() *OutlookGraphBackend {
	return &OutlookGraphBackend{}
}

// Name 实现 Backend。
func (b *OutlookGraphBackend) Name() string { return model.MailModeOutlookGraph }

// Open 通过 client_id + refresh_token 换取 access_token，构造 Graph 客户端。
//
// 关键决策：Microsoft 端点（login.microsoftonline.com / graph.microsoft.com）
// **直连**，不走注册任务的代理。原因：
//
//  1. 风控独立性：Outlook 邮箱只是"被动收件箱"，IP 不参与 OpenAI / Adobe / Grok
//     的注册风控判定（这些 provider 看的是浏览器流程的 IP，跟微软收邮件 IP 毫无
//     耦合）。早期注释里那个"反常信号"担忧是过度防御。
//
//  2. 商用代理（IPRoyal / Lumi / ArxLabs 等）对 login.microsoftonline.com 的
//     HTTPS CONNECT 隧道极不稳定，10 并发即频繁出现 `Post ...: EOF`。一个任务
//     240s 内对 graph.microsoft.com 拉取 80 次，命中失败概率几乎是 100%。
//     直连机器出口反而稳定（实测 token 接口 600ms / Graph list 1.5s）。
//
// 如果有"必须用代理出口收 Outlook 邮件"的特殊场景（例如调试用代理观察），
// 可以在系统配置里加个 outlook.use_proxy 开关，默认 false。
func (b *OutlookGraphBackend) Open(ctx context.Context, m *model.MailPool, secrets Secrets, cfg BackendConfig) (Mailbox, error) {
	if m.ClientID == "" {
		return nil, errors.New("outlook graph: mail_pool 行缺少 client_id")
	}
	if secrets.RefreshToken == "" {
		return nil, errors.New("outlook graph: mail_pool 行缺少 refresh_token")
	}
	hc := HTTPClientWithProxy("", 30*time.Second)

	// 批量 Outlook 账号的 refresh_token 是用某个 client_id 签发的，能换到的 access_token
	// 只能落在"当初被同意过的 scope 子集"里。常见坑：账号商实际同意的是
	// Mail.ReadWrite，而我们请求 Mail.Read，于是返回 AADSTS70000 invalid_grant
	// （requested scopes unauthorized）。解决办法是优先用 `.default`（让 AAD 返回
	// 该 token 实际拥有的全部 scope），再按候选链逐个回退，最大化兼容不同来源的号。
	candidates := buildGraphScopeCandidates(cfg.OutlookScopeGraph)
	var (
		at      string
		lastErr error
	)
	for i, sc := range candidates {
		at, lastErr = refreshOutlookAccessToken(ctx, hc, m.ClientID, secrets.RefreshToken, sc)
		if lastErr == nil {
			if i > 0 {
				log.Printf("[mailbox graph] email=%s scope 回退成功：%q", m.Email, sc)
			}
			return &outlookGraphMailbox{http: hc, accessToken: at, email: m.Email}, nil
		}
		// 仅在"scope/同意"类错误时继续尝试下一个候选；token 真失效就没必要重试。
		if !isScopeConsentError(lastErr) {
			break
		}
	}
	return nil, fmt.Errorf("outlook graph: 刷新 access_token 失败（直连 microsoftonline）：%w", lastErr)
}

// buildGraphScopeCandidates 构造按优先级排列、去重的 Graph scope 候选链。
//
// 顺序：用户显式配置 → `.default`（最稳，返回 token 实际拥有的 scope）→
// Mail.ReadWrite → Mail.Read。`.default` 必须单独请求，不能与其它资源 scope 混用。
func buildGraphScopeCandidates(configured string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, 4)
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	add(configured)
	add("https://graph.microsoft.com/.default")
	add("https://graph.microsoft.com/Mail.ReadWrite offline_access")
	add("https://graph.microsoft.com/Mail.Read offline_access")
	return out
}

// isScopeConsentError 判断是否为"scope 未授权 / 同意问题"类错误，用于决定是否回退下一个候选 scope。
func isScopeConsentError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "invalid_grant") ||
		strings.Contains(s, "AADSTS70000") ||
		strings.Contains(s, "AADSTS65001") ||
		strings.Contains(s, "unauthorized or expired")
}

// refreshOutlookAccessToken 用 refresh_token 换 access_token。
func refreshOutlookAccessToken(ctx context.Context, c *http.Client, clientID, refreshToken, scope string) (string, error) {
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("refresh_token", refreshToken)
	form.Set("grant_type", "refresh_token")
	form.Set("scope", scope)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://login.microsoftonline.com/common/oauth2/v2.0/token",
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var r struct {
		AccessToken      string `json:"access_token"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return "", fmt.Errorf("响应非 JSON: %s", strings.TrimSpace(string(raw)))
	}
	if r.AccessToken == "" {
		return "", fmt.Errorf("响应缺少 access_token: %s", r.ErrorDescription)
	}
	return r.AccessToken, nil
}

// === graph mailbox ===

type outlookGraphMailbox struct {
	http        *http.Client
	accessToken string
	email       string
}

func (m *outlookGraphMailbox) Close() error { return nil }

// graphMailURL 拼接拉取某文件夹邮件的 Graph URL。
//
// 注意：$orderby 的值 "receivedDateTime desc" 里有空格，必须编码成 %20。
// 早期模板直接写了裸空格，Go 会把它原样塞进 HTTP 请求行，被微软前端判为
// "Bad Request. The request is badly formed."（返回 HTML 400 而非 Graph JSON 错误），
// 导致收件轮询每次都 400、永远抓不到验证码。folder 仅取 Inbox / JunkEmail，无需转义。
func graphMailURL(folder string) string {
	return "https://graph.microsoft.com/v1.0/me/mailFolders('" + folder +
		"')/messages?$top=15&$orderby=receivedDateTime%20desc" +
		"&$select=subject,from,receivedDateTime,body,bodyPreview,id"
}

func (m *outlookGraphMailbox) WaitCode(ctx context.Context, opts WaitOptions) (string, error) {
	opts.normalize()
	deadline := time.Now().Add(opts.Timeout)
	seen := map[string]struct{}{}
	folders := []string{"Inbox", "JunkEmail"}
	pollIdx := 0
	var lastFetchErr error
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		pollIdx++
		for _, folder := range folders {
			msgs, err := m.fetchFolder(ctx, folder)
			if err != nil {
				lastFetchErr = err
				// 失败必须可见：以前静默 continue 导致"等了 4 分钟没邮件"看不出代理/
				// MS 风控等真因。每 5 轮打一次（首轮一定打），避免 240s × 0.33Hz 刷屏。
				if pollIdx == 1 || pollIdx%5 == 0 {
					log.Printf("[mailbox graph] poll#%d email=%s folder=%s fetch FAILED: %v",
						pollIdx, m.email, folder, err)
				}
				continue
			}
			if pollIdx == 1 {
				log.Printf("[mailbox graph] poll#%d email=%s folder=%s got %d msgs",
					pollIdx, m.email, folder, len(msgs))
			}
			for _, msg := range msgs {
				id, _ := msg["id"].(string)
				if id == "" {
					continue
				}
				if _, ok := seen[id]; ok {
					continue
				}
				seen[id] = struct{}{}
				rtRaw, _ := msg["receivedDateTime"].(string)
				if !opts.SinceTS.IsZero() && rtRaw != "" {
					if t, err := time.Parse(time.RFC3339, rtRaw); err == nil && t.Before(opts.SinceTS) {
						continue
					}
				}
				subject, _ := msg["subject"].(string)
				sender := ""
				if from, ok := msg["from"].(map[string]any); ok {
					if ea, ok := from["emailAddress"].(map[string]any); ok {
						sender, _ = ea["address"].(string)
					}
				}
				body := ""
				if b, ok := msg["body"].(map[string]any); ok {
					body, _ = b["content"].(string)
				}
				if body == "" {
					body, _ = msg["bodyPreview"].(string)
				}
				if !MatchSender(opts.Provider, sender, subject) {
					log.Printf("[mailbox graph] poll#%d email=%s skip msg (%s/%s) MatchSender=false: from=%s subj=%q",
						pollIdx, m.email, folder, rtRaw, sender, truncate(subject, 60))
					continue
				}
				if code, ok := ExtractCode(opts.Provider, subject, body); ok {
					log.Printf("[mailbox graph] poll#%d email=%s ✅ EXTRACTED code=%s from=%s subj=%q rt=%s",
						pollIdx, m.email, code, sender, truncate(subject, 60), rtRaw)
					return code, nil
				}
				log.Printf("[mailbox graph] poll#%d email=%s match sender but ExtractCode FAIL: from=%s subj=%q body_len=%d",
					pollIdx, m.email, sender, truncate(subject, 60), len(body))
			}
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(opts.PollInterval):
		}
	}
	// 超时退出前打一次终态，方便看是"全程 fetch 失败"还是"fetch 成功但无匹配邮件"
	if lastFetchErr != nil {
		log.Printf("[mailbox graph] WaitCode timeout email=%s polls=%d (last fetch err: %v)",
			m.email, pollIdx, lastFetchErr)
	} else {
		log.Printf("[mailbox graph] WaitCode timeout email=%s polls=%d (fetch ok, no matching mail)",
			m.email, pollIdx)
	}
	return "", ErrCodeNotFound
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ListRecent 拉取收件箱 + 垃圾箱最近若干封邮件，用于「收取邮件」预览。
func (m *outlookGraphMailbox) ListRecent(ctx context.Context, limit int) ([]MailMessage, error) {
	if limit <= 0 {
		limit = 15
	}
	folders := []string{"Inbox", "JunkEmail"}
	seen := map[string]struct{}{}
	out := make([]MailMessage, 0, limit)
	var lastErr error
	for _, folder := range folders {
		msgs, err := m.fetchFolder(ctx, folder)
		if err != nil {
			lastErr = err
			continue
		}
		for _, msg := range msgs {
			id, _ := msg["id"].(string)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			subject, _ := msg["subject"].(string)
			received, _ := msg["receivedDateTime"].(string)
			sender := ""
			if from, ok := msg["from"].(map[string]any); ok {
				if ea, ok := from["emailAddress"].(map[string]any); ok {
					sender, _ = ea["address"].(string)
				}
			}
			preview, _ := msg["bodyPreview"].(string)
			body := ""
			if b, ok := msg["body"].(map[string]any); ok {
				body, _ = b["content"].(string)
			}
			out = append(out, MailMessage{
				ID:       id,
				Folder:   folder,
				Subject:  subject,
				From:     sender,
				Received: received,
				Preview:  preview,
				Body:     body,
			})
		}
	}
	if len(out) == 0 && lastErr != nil {
		return nil, lastErr
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Received > out[j].Received })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *outlookGraphMailbox) fetchFolder(ctx context.Context, folder string) ([]map[string]any, error) {
	u := graphMailURL(folder)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+m.accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := m.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		snippet := strings.ReplaceAll(strings.ReplaceAll(string(raw), "\n", " "), "\r", " ")
		return nil, fmt.Errorf("graph HTTP %d body=%s", resp.StatusCode, snippet)
	}
	var body struct {
		Value []map[string]any `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	return body.Value, nil
}

// 让 bytes 不被 unused 抱怨（保留备用）。
var _ = bytes.NewBuffer
