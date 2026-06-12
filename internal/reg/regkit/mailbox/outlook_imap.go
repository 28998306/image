package mailbox

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message/mail"
	_ "github.com/emersion/go-message/charset"

	"web2img/internal/reg/model"
)

// OutlookIMAPBackend Outlook IMAP（XOAUTH2）。
//
// 用 client_id + refresh_token 换取 outlook.office.com 资源的 access_token，
// 通过 SASL XOAUTH2 登录 outlook.office365.com:993，轮询 INBOX / Junk 收验证码。
//
// 与 Graph 一样：连接微软直连、不走注册任务代理（收信 IP 不参与 OpenAI 风控，
// 且商用代理对 office365 IMAP 隧道不稳定）。
type OutlookIMAPBackend struct{}

// NewOutlookIMAPBackend 构造。
func NewOutlookIMAPBackend() *OutlookIMAPBackend { return &OutlookIMAPBackend{} }

// Name 实现 Backend。
func (b *OutlookIMAPBackend) Name() string { return model.MailModeOutlookIMAP }

// Open 刷新 IMAP access_token 并建立已认证的 IMAP 会话。
func (b *OutlookIMAPBackend) Open(ctx context.Context, m *model.MailPool, secrets Secrets, cfg BackendConfig) (Mailbox, error) {
	if m.ClientID == "" {
		return nil, errors.New("outlook imap: mail_pool 行缺少 client_id")
	}
	if secrets.RefreshToken == "" {
		return nil, errors.New("outlook imap: mail_pool 行缺少 refresh_token")
	}

	hc := HTTPClientWithProxy("", 30*time.Second)
	candidates := buildIMAPScopeCandidates(cfg.OutlookScopeIMAP)
	var (
		at      string
		lastErr error
	)
	for i, sc := range candidates {
		at, lastErr = refreshOutlookAccessToken(ctx, hc, m.ClientID, secrets.RefreshToken, sc)
		if lastErr == nil {
			if i > 0 {
				log.Printf("[mailbox imap] email=%s scope 回退成功：%q", m.Email, sc)
			}
			break
		}
		if !isScopeConsentError(lastErr) {
			break
		}
	}
	if at == "" {
		return nil, fmt.Errorf("outlook imap: 刷新 access_token 失败（直连 microsoftonline）：%w", lastErr)
	}

	c, err := client.DialTLS("outlook.office365.com:993", &tls.Config{ServerName: "outlook.office365.com"})
	if err != nil {
		return nil, fmt.Errorf("outlook imap: 连接 office365 失败：%w", err)
	}
	c.Timeout = 30 * time.Second
	if err := c.Authenticate(&xoauth2Client{user: m.Email, token: at}); err != nil {
		_ = c.Logout()
		return nil, fmt.Errorf("outlook imap: XOAUTH2 认证失败：%w", err)
	}
	return &outlookIMAPMailbox{c: c, email: m.Email}, nil
}

// buildIMAPScopeCandidates IMAP access_token 的 scope 候选链（去重、按优先级）。
func buildIMAPScopeCandidates(configured string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, 3)
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	add(configured)
	add("https://outlook.office.com/IMAP.AccessAsUser.All offline_access")
	add("https://outlook.office.com/.default")
	return out
}

// xoauth2Client 实现 SASL XOAUTH2（go-imap Authenticate 接受结构化匹配的 sasl.Client）。
type xoauth2Client struct {
	user  string
	token string
}

// Start 返回 XOAUTH2 初始响应。
func (a *xoauth2Client) Start() (mech string, ir []byte, err error) {
	return "XOAUTH2", []byte("user=" + a.user + "\x01auth=Bearer " + a.token + "\x01\x01"), nil
}

// Next 服务端只有在认证失败时才发 challenge，这里直接当错误返回。
func (a *xoauth2Client) Next(challenge []byte) ([]byte, error) {
	return nil, fmt.Errorf("xoauth2 被拒：%s", strings.TrimSpace(string(challenge)))
}

// === imap mailbox ===

type outlookIMAPMailbox struct {
	c     *client.Client
	email string
}

func (m *outlookIMAPMailbox) Close() error {
	if m.c != nil {
		return m.c.Logout()
	}
	return nil
}

// folders 轮询的文件夹：收件箱 + 垃圾箱（Outlook IMAP 垃圾箱名为 Junk）。
func (m *outlookIMAPMailbox) folders() []string { return []string{"INBOX", "Junk"} }

func (m *outlookIMAPMailbox) WaitCode(ctx context.Context, opts WaitOptions) (string, error) {
	opts.normalize()
	deadline := time.Now().Add(opts.Timeout)
	seen := map[string]struct{}{}
	folders := m.folders()
	pollIdx := 0
	var lastErr error
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		pollIdx++
		for _, folder := range folders {
			msgs, err := m.listFolder(folder, 15)
			if err != nil {
				lastErr = err
				if pollIdx == 1 || pollIdx%5 == 0 {
					log.Printf("[mailbox imap] poll#%d email=%s folder=%s fetch FAILED: %v", pollIdx, m.email, folder, err)
				}
				continue
			}
			for _, msg := range msgs {
				key := folder + "|" + msg.ID
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				if !opts.SinceTS.IsZero() && msg.Received != "" {
					if t, err := time.Parse(time.RFC3339, msg.Received); err == nil && t.Before(opts.SinceTS) {
						continue
					}
				}
				if !MatchSender(opts.Provider, msg.From, msg.Subject) {
					continue
				}
				if code, ok := ExtractCode(opts.Provider, msg.Subject, msg.Body); ok {
					log.Printf("[mailbox imap] poll#%d email=%s ✅ EXTRACTED code from=%s subj=%q", pollIdx, m.email, msg.From, truncate(msg.Subject, 60))
					return code, nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(opts.PollInterval):
		}
	}
	if lastErr != nil {
		log.Printf("[mailbox imap] WaitCode timeout email=%s polls=%d (last fetch err: %v)", m.email, pollIdx, lastErr)
	}
	return "", ErrCodeNotFound
}

// ListRecent 列出 INBOX + Junk 最近若干封，用于「收取邮件」预览。
func (m *outlookIMAPMailbox) ListRecent(ctx context.Context, limit int) ([]MailMessage, error) {
	if limit <= 0 {
		limit = 15
	}
	out := make([]MailMessage, 0, limit*2)
	var lastErr error
	for _, folder := range m.folders() {
		msgs, err := m.listFolder(folder, limit)
		if err != nil {
			lastErr = err
			continue
		}
		out = append(out, msgs...)
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

// listFolder SELECT 指定文件夹并拉取最后 limit 封邮件。
func (m *outlookIMAPMailbox) listFolder(folder string, limit int) ([]MailMessage, error) {
	mbox, err := m.c.Select(folder, true)
	if err != nil {
		return nil, err
	}
	if mbox.Messages == 0 {
		return nil, nil
	}
	from := uint32(1)
	if mbox.Messages > uint32(limit) {
		from = mbox.Messages - uint32(limit) + 1
	}
	seqset := new(imap.SeqSet)
	seqset.AddRange(from, mbox.Messages)
	section := &imap.BodySectionName{}
	items := []imap.FetchItem{imap.FetchEnvelope, imap.FetchInternalDate, section.FetchItem()}

	ch := make(chan *imap.Message, limit+2)
	done := make(chan error, 1)
	go func() { done <- m.c.Fetch(seqset, items, ch) }()

	out := make([]MailMessage, 0, limit)
	for msg := range ch {
		if msg == nil || msg.Envelope == nil {
			continue
		}
		subject := msg.Envelope.Subject
		sender := ""
		if len(msg.Envelope.From) > 0 {
			sender = msg.Envelope.From[0].Address()
		}
		body := ""
		if r := msg.GetBody(section); r != nil {
			body = decodeMailBody(r)
		}
		id := strings.TrimSpace(msg.Envelope.MessageId)
		if id == "" {
			id = fmt.Sprintf("%s-%d", folder, msg.SeqNum)
		}
		out = append(out, MailMessage{
			ID:       id,
			Folder:   folder,
			Subject:  subject,
			From:     sender,
			Received: msg.InternalDate.UTC().Format(time.RFC3339),
			Body:     body,
			Preview:  imapPreview(body),
		})
	}
	if e := <-done; e != nil {
		return out, e
	}
	return out, nil
}

// decodeMailBody 解析 RFC822 原文，拼接 text/plain + text/html 的解码后正文。
// 解析失败则回退为原始字节串（quoted-printable / 纯文本仍可被验证码正则命中）。
func decodeMailBody(r io.Reader) string {
	raw, _ := io.ReadAll(r)
	if mr, err := mail.CreateReader(bytes.NewReader(raw)); err == nil {
		var sb strings.Builder
		for {
			p, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				break
			}
			if _, ok := p.Header.(*mail.InlineHeader); ok {
				b, _ := io.ReadAll(p.Body)
				sb.Write(b)
				sb.WriteByte('\n')
			}
		}
		if sb.Len() > 0 {
			return sb.String()
		}
	}
	return string(raw)
}

func imapPreview(body string) string {
	clean := reHTMLStripper.ReplaceAllString(body, " ")
	clean = strings.Join(strings.Fields(clean), " ")
	return truncate(clean, 160)
}
