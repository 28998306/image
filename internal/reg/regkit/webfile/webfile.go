// Package webfile 逆向 ChatGPT 网页会话的「可编辑文件生成」能力（PPT / PSD）。
//
// 走 chatgpt.com 网页会话
// （/backend-api/conversation），用 gpt-5-5-thinking + 代码解释器让模型产出
// 可下载的 .pptx / .psd + 素材 .zip，再解析 conversation mapping 里的 sandbox
// 文件指针下载落地。需要 Plus/Team/Pro 订阅账号的 access_token。
//
// 关键步骤：bootstrap → sentinel/chat-requirements(含 PoW) → 上传参考图 →
// f/conversation/prepare 拿 conduit → f/conversation 流式建会话 → 轮询会话
// mapping 找 /mnt/data/*.pptx|psd|zip → interpreter/files download。
package webfile

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	mrand "math/rand"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/sha3"

	"web2img/internal/reg/regkit/browser"
)

const (
	baseURL              = "https://chatgpt.com"
	editableFileModel    = "gpt-5-5-thinking"
	editableThinking     = "extended"
	editableClientVer    = "prod-bede35f9dcd856d080e012478f0c1031faa2588e"
	editableClientBuild  = "6631702"
	editableTimeout      = 1200 * time.Second
	editablePollInterval = 5 * time.Second
	defaultUserAgent     = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36 Edg/143.0.0.0"
	secChUA              = `"Microsoft Edge";v="143", "Chromium";v="143", "Not A(Brand";v="24"`
)

const pptPrompt = `我需要你根据用户的需求，来制作一个可以编辑的PPT，你可以使用Agent来做，你不要再继续询问用户问题，内容风格、版式、配色、内容结构和页面信息你可以自行补充并直接执行。整体的流程如下：
1. 用生图的方式，帮我生成一个精美的产品介绍ppt，5-6个页面
2. 帮我把以上涉及到的所有图像和形状素材拆分成单独png，每个素材单独一张图片，不要有遗漏，让我可以直接在ppt里拼接素材还原，不要文字
3. 利用以上所有图片和形状素材，帮我还原你第一次生成的展示ppt，我需要是可编辑的ppt格式，主要部分需要你单独还原插入，文字需要可以编辑
最后只需要给我生成一个PPT文件，以及生成中遇到的各种素材压缩包zip文件就行。`

const psdPrompt = `帮我生成这个图像，把这张海报分成若干图像，包括背景图，每个元素不要改位置，这样子我可以直接在 平时里无需拖动，底色为白色，不要伪透明底。再帮我将以上拆分的图像拼合成一个psd文件，去除白色底，不要改变每个图层的相应位置，保留每个元素所在图层的相应位置，保留每个元素的图层，最后只需要给我输出psd文件，以及每个图层的zip文件`

var (
	pptExportRe      = regexp.MustCompile(`(?i)(?:sandbox:)?(/mnt/data/[^\s"'\)\]]+\.(?:pptx?|zip))`)
	psdExportRe      = regexp.MustCompile(`(?i)(?:sandbox:)?(/mnt/data/[^\s"'\)\]]+\.(?:psd|zip))`)
	assetPointerRe   = regexp.MustCompile(`(?:file-service|sediment)://([A-Za-z0-9_-]+)`)
	fileIDRe         = regexp.MustCompile(`\bfile[-_][A-Za-z0-9_-]+\b`)
	conversationIDRe = regexp.MustCompile(`"conversation_id"\s*:\s*"([^"]+)"`)
	// 网页出图：生成图片的 file id（DALLE/image 工具产物）与直链。
	imgFileIDRe      = regexp.MustCompile(`file[-_][A-Za-z0-9][A-Za-z0-9_-]{7,}`)
	imgAssetURLRe    = regexp.MustCompile(`https:\\?/\\?/(?:files\.oaiusercontent\.com|oaidalleapiprodscus\.blob\.core\.windows\.net)[^"\\]+`)
	imageGenTaskIDRe = regexp.MustCompile(`"image_gen_task_id"\s*:\s*"([^"]+)"`)
)


// Options 一次可编辑文件生成的入参。
type Options struct {
	AccessToken  string
	AccountID    string
	ProxyURL     string
	Kind         string       // "ppt" / "psd"
	Prompt       string       // 追加到固定模板后的用户补充描述
	Base64Images []string     // psd 必填；ppt 可选（data URL 或裸 base64）
	Log          func(string) // 进度日志回调（可空）
}

// File 下载后的本地文件。
type File struct {
	Path string
	Name string
	Mime string
	Size int64
}

// Result 生成结果。
type Result struct {
	ConversationID string
	Primary        File // .pptx / .psd
	Zip            File // 素材压缩包
}

type chatRequirements struct {
	Token      string
	ProofToken string
}

// ErrTurnstile 表示 chat-requirements 强制 Cloudflare turnstile 验证且本地无法可靠生成令牌。
// 上层可据此切换账号或回退到 Codex 链路。
var ErrTurnstile = fmt.Errorf("chat-requirements 需要 turnstile 验证")

// ErrCloudflare 表示请求被 Cloudflare 边缘直接拦截（返回 HTML 挑战/拦截页，HTTP 403）。
// 网页版 /backend-api/f/* 等路由需要浏览器级 cf_clearance，HTTP 方式无法通过；
// 这与账号无关（同 IP/指纹下换号也会被拦），上层据此给出明确提示。
var ErrCloudflare = fmt.Errorf("请求被 Cloudflare 拦截（需浏览器级验证）")

// isCloudflareBlockHTML 判断响应体是否为 Cloudflare 边缘返回的 HTML 挑战/拦截页。
func isCloudflareBlockHTML(status int, body []byte) bool {
	if status != http.StatusForbidden && status != http.StatusServiceUnavailable {
		return false
	}
	s := strings.ToLower(strings.TrimSpace(string(body)))
	if !strings.HasPrefix(s, "<") {
		return false
	}
	return strings.Contains(s, "<html") ||
		strings.Contains(s, "cloudflare") ||
		strings.Contains(s, "just a moment") ||
		strings.Contains(s, "enlarge-appear") ||
		strings.Contains(s, "cf-")
}

type uploadedImage struct {
	FileID        string
	LibraryFileID string
	FileName      string
	FileSize      int64
	MimeType      string
	Width         int
	Height        int
}

type artifact struct {
	AttachmentID string
	FileID       string
	Name         string
	MimeType     string
	CreateTime   float64
	SandboxPath  string
	MessageID    string
}

type client struct {
	bc        *browser.Client
	token     string
	accountID string
	deviceID  string
	sessionID string
	userAgent string
	logFn     func(string)
}

func (c *client) log(msg string) {
	if c.logFn != nil {
		c.logFn(msg)
	}
}

// Export 生成可编辑 PPT / PSD 文件并下载到 outputDir，返回主文件 + zip。
func Export(ctx context.Context, opt Options, outputDir string) (*Result, error) {
	if strings.TrimSpace(opt.AccessToken) == "" {
		return nil, fmt.Errorf("缺少 access_token")
	}
	kind := strings.ToLower(strings.TrimSpace(opt.Kind))
	if kind != "ppt" && kind != "psd" {
		kind = "ppt"
	}
	if kind == "psd" && len(opt.Base64Images) == 0 {
		return nil, fmt.Errorf("PSD 生成需要至少一张参考图")
	}

	bc, err := browser.New(browser.Options{ProxyURL: opt.ProxyURL, Timeout: 300 * time.Second, ForceHTTP1: true})
	if err != nil {
		return nil, err
	}
	c := &client{
		bc:        bc,
		token:     opt.AccessToken,
		accountID: opt.AccountID,
		deviceID:  uuid.NewString(),
		sessionID: uuid.NewString(),
		userAgent: defaultUserAgent,
		logFn:     opt.Log,
	}

	prompt := editablePrompt(kind, opt.Prompt)

	// 1) 先预热会话拿 cf_clearance，否则后续 /backend-api/* 直接被 Cloudflare 403。
	c.log("预热 chatgpt.com 会话（cf_clearance）…")
	if err := c.bootstrap(ctx); err != nil {
		return nil, err
	}

	// 2) 上传参考图
	if len(opt.Base64Images) > 0 {
		c.log(fmt.Sprintf("上传 %d 张参考图…", len(opt.Base64Images)))
	}
	uploaded := make([]uploadedImage, 0, len(opt.Base64Images))
	mimeTypes := make([]string, 0, len(opt.Base64Images))
	for i, img := range opt.Base64Images {
		u, uerr := c.uploadImage(ctx, img, i+1)
		if uerr != nil {
			return nil, fmt.Errorf("上传参考图失败: %w", uerr)
		}
		uploaded = append(uploaded, *u)
		mimeTypes = append(mimeTypes, u.MimeType)
	}

	// 3) 先取 chat-requirements（token + PoW），prepare/conversation 都要带它的 sentinel 头
	c.log("获取风控令牌（chat-requirements / PoW）…")
	reqs, err := c.getChatRequirements(ctx)
	if err != nil {
		return nil, err
	}

	// 4) prepare 拿 conduit
	c.log("准备会话…")
	conduit, err := c.prepareConversation(ctx, reqs, prompt, mimeTypes, nil)
	if err != nil {
		return nil, err
	}

	// 5) 建会话拿 conversation_id
	c.log("发起会话，等待模型处理…")
	convID, err := c.runConversation(ctx, reqs, prompt, uploaded, conduit)
	if err != nil {
		return nil, err
	}
	c.log("会话已建立，开始轮询产物（这一步最慢，模型需边想边生成文件）…")

	// 4) 轮询会话产出，拿到 主文件 + zip
	primaryRe := pptExportRe
	primarySuffix := []string{".ppt", ".pptx"}
	if kind == "psd" {
		primaryRe = psdExportRe
		primarySuffix = []string{".psd"}
	}
	arts, err := c.waitArtifacts(ctx, convID, primaryRe, primarySuffix)
	if err != nil {
		return nil, err
	}
	c.log(fmt.Sprintf("找到 %d 个产物，开始下载…", len(arts)))

	// 5) 下载落地
	res := &Result{ConversationID: convID}
	for _, a := range arts {
		f, derr := c.downloadArtifact(ctx, convID, a, outputDir, kind)
		if derr != nil {
			return nil, fmt.Errorf("下载产物失败: %w", derr)
		}
		c.log("已下载：" + f.Name)
		lower := strings.ToLower(f.Name)
		if strings.HasSuffix(lower, ".zip") {
			res.Zip = *f
		} else {
			res.Primary = *f
		}
	}
	if res.Primary.Path == "" {
		return nil, fmt.Errorf("生成完成但未取得 %s 主文件", strings.ToUpper(kind))
	}
	return res, nil
}

// ImageOptions 网页出图（套图/多图）入参。
type ImageOptions struct {
	AccessToken string
	AccountID   string
	ProxyURL    string
	Prompt      string
	N           int
	RefImages   []string
	Log         func(string) // 进度日志回调（可空）
}

// ImageOut 网页出图的一张结果（原始字节）。
type ImageOut struct {
	Data []byte
	Mime string
}

// GenerateImages 走 ChatGPT 网页会话出图（gpt-image-2 网页链路），支持一次返回多张「套图」。
func GenerateImages(ctx context.Context, opt ImageOptions) ([]ImageOut, error) {
	if strings.TrimSpace(opt.AccessToken) == "" {
		return nil, fmt.Errorf("缺少 access_token")
	}
	n := opt.N
	if n <= 0 {
		n = 1
	}
	bc, err := browser.New(browser.Options{ProxyURL: opt.ProxyURL, Timeout: 300 * time.Second, ForceHTTP1: true})
	if err != nil {
		return nil, err
	}
	c := &client{
		bc:        bc,
		token:     opt.AccessToken,
		accountID: opt.AccountID,
		deviceID:  uuid.NewString(),
		sessionID: uuid.NewString(),
		userAgent: defaultUserAgent,
		logFn:     opt.Log,
	}

	prompt := strings.TrimSpace(opt.Prompt)
	if n > 1 {
		prompt = fmt.Sprintf("%s\n\n请一次生成 %d 张风格统一的图片（套图）。", prompt, n)
	}

	// 先预热会话拿 cf_clearance，否则后续 /backend-api/* 直接被 Cloudflare 403。
	c.log("预热 chatgpt.com 会话（cf_clearance）…")
	if err := c.bootstrap(ctx); err != nil {
		return nil, err
	}

	if len(opt.RefImages) > 0 {
		c.log(fmt.Sprintf("上传 %d 张参考图…", len(opt.RefImages)))
	}
	uploaded := make([]uploadedImage, 0, len(opt.RefImages))
	mimeTypes := make([]string, 0, len(opt.RefImages))
	for i, img := range opt.RefImages {
		u, uerr := c.uploadImage(ctx, img, i+1)
		if uerr != nil {
			return nil, fmt.Errorf("上传参考图失败: %w", uerr)
		}
		uploaded = append(uploaded, *u)
		mimeTypes = append(mimeTypes, u.MimeType)
	}

	c.log("获取风控令牌（chat-requirements / PoW）…")
	reqs, err := c.getChatRequirements(ctx)
	if err != nil {
		return nil, err
	}
	c.log("准备会话…")
	conduit, err := c.prepareConversation(ctx, reqs, prompt, mimeTypes, []string{"picture_v2"})
	if err != nil {
		return nil, err
	}
	c.log("发起网页会话出图，等待模型生成…")
	ev, err := c.runImageConversation(ctx, reqs, prompt, uploaded, conduit)
	if err != nil {
		return nil, err
	}
	if ev.convID == "" {
		return nil, fmt.Errorf("会话流中未找到 conversation_id")
	}
	c.log("会话已建立：" + ev.convID)
	if ev.taskID != "" {
		c.log("图片生成任务已提交（image_gen_task_id），开始轮询结果…")
	} else {
		c.log(fmt.Sprintf("轮询套图产出（目标 %d 张）…", n))
	}

	// 图片为异步生成：轮询 conversation / stream_status / async-status 收集资产，
	// 其中 async-status(POST) 还会推进异步任务（参考可用实现 GoGPTImg 的做法）。
	if len(ev.urls)+len(ev.fileIDs) < n {
		c.waitImageAssets(ctx, &ev, n)
	}
	if len(ev.urls)+len(ev.fileIDs) == 0 {
		return nil, fmt.Errorf("网页会话出图未返回图片资产（可能被内容策略拦截或额度不足）")
	}
	c.log(fmt.Sprintf("解析到 %d 个图片链接 + %d 个文件，开始下载…", len(ev.urls), len(ev.fileIDs)))

	out := make([]ImageOut, 0, n)
	seen := map[string]bool{}
	for _, u := range ev.urls {
		if seen[u] || len(out) >= n {
			continue
		}
		seen[u] = true
		if data, mime, derr := c.fetchBytes(ctx, u); derr == nil && len(data) > 0 {
			out = append(out, ImageOut{Data: data, Mime: mime})
		}
	}
	for _, id := range ev.fileIDs {
		if len(out) >= n {
			break
		}
		dl, _ := c.resolveDownloadURL(ctx, ev.convID, artifact{FileID: id, AttachmentID: id})
		if dl == "" || seen[dl] {
			continue
		}
		seen[dl] = true
		if data, mime, derr := c.fetchBytes(ctx, dl); derr == nil && len(data) > 0 {
			out = append(out, ImageOut{Data: data, Mime: mime})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("网页会话出图返回 0 张（拿到资产但下载失败）")
	}
	return out, nil
}

// imageEvidence 汇总网页出图链路收集到的图片资产线索。
type imageEvidence struct {
	convID  string
	taskID  string
	urls    []string
	fileIDs []string
	seenURL map[string]bool
	seenID  map[string]bool
}

func (e *imageEvidence) addURL(u string) {
	if e.seenURL == nil {
		e.seenURL = map[string]bool{}
	}
	u = strings.TrimSpace(u)
	if u == "" || e.seenURL[u] {
		return
	}
	e.seenURL[u] = true
	e.urls = append(e.urls, u)
}

func (e *imageEvidence) addID(id string) {
	if e.seenID == nil {
		e.seenID = map[string]bool{}
	}
	id = strings.TrimSpace(id)
	if id == "" || e.seenID[id] {
		return
	}
	e.seenID[id] = true
	e.fileIDs = append(e.fileIDs, id)
}

// collectRefs 从任意 JSON/文本里抽取图片直链与文件资产 id。
func (e *imageEvidence) collectRefs(text string) {
	for _, raw := range imgAssetURLRe.FindAllString(text, -1) {
		u := strings.ReplaceAll(raw, `\/`, `/`)
		u = strings.ReplaceAll(u, `\u0026`, "&")
		e.addURL(u)
	}
	for _, m := range assetPointerRe.FindAllStringSubmatch(text, -1) {
		if len(m) > 1 {
			e.addID(m[1])
		}
	}
	for _, id := range imgFileIDRe.FindAllString(text, -1) {
		if strings.HasPrefix(id, "file-service") || strings.HasPrefix(id, "file_service") {
			continue
		}
		e.addID(id)
	}
}

// runImageConversation 发起 /f/conversation 并读完整 SSE，收集 conversation_id、
// image_gen_task_id 以及流中可能出现的图片资产线索。
func (c *client) runImageConversation(ctx context.Context, reqs *chatRequirements, prompt string, uploaded []uploadedImage, conduit string) (imageEvidence, error) {
	ev := imageEvidence{}
	if reqs == nil {
		return ev, fmt.Errorf("缺少 chat-requirements 令牌")
	}
	message := buildUserMessage(prompt, uploaded)
	p := "/backend-api/f/conversation"
	payload, _ := json.Marshal(map[string]any{
		"action":               "next",
		"messages":             []any{message},
		"parent_message_id":    "client-created-root",
		"model":                editableFileModel,
		"client_prepare_state": "sent",
		"timezone_offset_min":  -480,
		"timezone":             "Asia/Shanghai",
		"conversation_mode":    map[string]any{"kind": "primary_assistant"},
		"enable_message_followups": true,
		"system_hints":         []string{"picture_v2"},
		"supports_buffering":   true,
		"supported_encodings":  []string{"v1"},
		"client_contextual_info": map[string]any{
			"is_dark_mode":      false,
			"time_since_loaded": 401,
			"page_height":       1138,
			"page_width":        803,
			"pixel_ratio":       2,
			"screen_height":     1440,
			"screen_width":      2560,
			"app_name":          "chatgpt.com",
		},
		"paragen_cot_summary_display_override": "allow",
		"force_parallel_switch":                "auto",
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+p, bytes.NewReader(payload))
	c.setCommon(req, p)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("OpenAI-Sentinel-Chat-Requirements-Token", reqs.Token)
	if reqs.ProofToken != "" {
		req.Header.Set("OpenAI-Sentinel-Proof-Token", reqs.ProofToken)
	}
	if conduit != "" {
		req.Header.Set("X-Conduit-Token", conduit)
	}
	req.Header.Set("X-Oai-Turn-Trace-Id", uuid.NewString())
	resp, err := c.bc.Do(req)
	if err != nil {
		return ev, fmt.Errorf("conversation: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if isCloudflareBlockHTML(resp.StatusCode, raw) {
			return ev, fmt.Errorf("%w（conversation HTTP %d）", ErrCloudflare, resp.StatusCode)
		}
		return ev, fmt.Errorf("conversation HTTP %d: %s", resp.StatusCode, snippet(raw, 300))
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ev, ctx.Err()
		default:
		}
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		if ev.convID == "" {
			if m := conversationIDRe.FindStringSubmatch(data); len(m) > 1 {
				ev.convID = m[1]
			}
		}
		if ev.taskID == "" {
			if m := imageGenTaskIDRe.FindStringSubmatch(data); len(m) > 1 {
				ev.taskID = m[1]
			}
		}
		ev.collectRefs(data)
	}
	return ev, nil
}

// waitImageAssets 轮询 conversation / stream_status / async-status 直到拿到图片资产。
func (c *client) waitImageAssets(ctx context.Context, ev *imageEvidence, want int) {
	deadline := time.Now().Add(editableTimeout)
	nextAsync := time.Now()
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if raw, code := c.rawGet(ctx, "/backend-api/conversation/"+ev.convID); code >= 200 && code < 300 {
			ev.collectRefs(string(raw))
		}
		if len(ev.urls)+len(ev.fileIDs) >= want {
			return
		}
		if raw, code := c.rawGet(ctx, "/backend-api/conversation/"+ev.convID+"/stream_status"); code >= 200 && code < 300 {
			ev.collectRefs(string(raw))
		}
		if len(ev.urls)+len(ev.fileIDs) >= want {
			return
		}
		if !time.Now().Before(nextAsync) {
			if raw, code := c.rawPost(ctx, "/backend-api/conversation/"+ev.convID+"/async-status", []byte(`{"status":null}`)); code >= 200 && code < 300 {
				ev.collectRefs(string(raw))
			}
			nextAsync = time.Now().Add(10 * time.Second)
		}
		if len(ev.urls)+len(ev.fileIDs) >= want {
			return
		}
		time.Sleep(editablePollInterval)
	}
}

// rawGet / rawPost：带通用头的简单请求，返回原始响应体与状态码。
func (c *client) rawGet(ctx context.Context, p string) ([]byte, int) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+p, nil)
	c.setCommon(req, p)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Referer", baseURL+"/c/"+strings.TrimPrefix(p, "/backend-api/conversation/"))
	resp, err := c.bc.Do(req)
	if err != nil {
		return nil, 0
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	resp.Body.Close()
	return raw, resp.StatusCode
}

func (c *client) rawPost(ctx context.Context, p string, body []byte) ([]byte, int) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+p, bytes.NewReader(body))
	c.setCommon(req, p)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.bc.Do(req)
	if err != nil {
		return nil, 0
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	resp.Body.Close()
	return raw, resp.StatusCode
}

// buildUserMessage 构造一条用户消息（含可选参考图附件）。
func buildUserMessage(prompt string, uploaded []uploadedImage) map[string]any {
	message := map[string]any{
		"id":          uuid.NewString(),
		"author":      map[string]any{"role": "user"},
		"create_time": float64(time.Now().UnixNano()) / 1e9,
	}
	if len(uploaded) > 0 {
		parts := make([]any, 0, len(uploaded)+1)
		attachments := make([]any, 0, len(uploaded))
		for _, u := range uploaded {
			parts = append(parts, map[string]any{
				"content_type":  "image_asset_pointer",
				"asset_pointer": "sediment://" + u.FileID,
				"size_bytes":    u.FileSize,
				"width":         u.Width,
				"height":        u.Height,
			})
			attachments = append(attachments, map[string]any{
				"id":              u.FileID,
				"size":            u.FileSize,
				"name":            u.FileName,
				"mime_type":       u.MimeType,
				"width":           u.Width,
				"height":          u.Height,
				"source":          "library",
				"library_file_id": u.LibraryFileID,
				"is_big_paste":    false,
			})
		}
		parts = append(parts, prompt)
		message["content"] = map[string]any{"content_type": "multimodal_text", "parts": parts}
		message["metadata"] = map[string]any{
			"attachments":                  attachments,
			"developer_mode_connector_ids": []string{},
			"selected_sources":             []string{},
			"selected_github_repos":        []string{},
			"selected_all_github_repos":    false,
			"serialization_metadata":       map[string]any{"custom_symbol_offsets": []any{}},
		}
	} else {
		message["content"] = map[string]any{"content_type": "text", "parts": []string{prompt}}
	}
	return message
}

func (c *client) fetchBytes(ctx context.Context, rawURL string) ([]byte, string, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	req.Header.Set("User-Agent", c.userAgent)
	// chatgpt 自家域名带上鉴权，预签名 blob 直链不需要。
	if strings.Contains(rawURL, "oaiusercontent.com") || strings.Contains(rawURL, "chatgpt.com") {
		if c.token != "" {
			req.Header.Set("Authorization", "Bearer "+c.token)
		}
	}
	resp, err := c.bc.Do(req)
	if err != nil {
		return nil, "", err
	}
	data, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("fetch HTTP %d", resp.StatusCode)
	}
	mime := cleanMime(resp.Header.Get("Content-Type"))
	if mime == "" || !strings.HasPrefix(mime, "image/") {
		mime = "image/png"
	}
	return data, mime, nil
}

func editablePrompt(kind, userPrompt string) string {
	base := pptPrompt
	if kind == "psd" {
		base = psdPrompt
	}
	userPrompt = strings.TrimSpace(userPrompt)
	if userPrompt == "" {
		return base
	}
	return base + "\n\n用户补充需求：" + userPrompt
}

// ---- HTTP 头 ----

func (c *client) setCommon(req *http.Request, p string) {
	h := req.Header
	h.Set("User-Agent", c.userAgent)
	h.Set("Origin", baseURL)
	h.Set("Referer", baseURL+"/")
	h.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8,en-US;q=0.7")
	h.Set("Cache-Control", "no-cache")
	h.Set("Pragma", "no-cache")
	h.Set("Priority", "u=1, i")
	h.Set("sec-ch-ua", secChUA)
	h.Set("sec-ch-ua-arch", `"x86"`)
	h.Set("sec-ch-ua-bitness", `"64"`)
	h.Set("sec-ch-ua-full-version", `"143.0.3650.96"`)
	h.Set("sec-ch-ua-full-version-list", `"Microsoft Edge";v="143.0.3650.96", "Chromium";v="143.0.7499.147", "Not A(Brand";v="24.0.0.0"`)
	h.Set("sec-ch-ua-mobile", "?0")
	h.Set("sec-ch-ua-model", `""`)
	h.Set("sec-ch-ua-platform", `"Windows"`)
	h.Set("sec-ch-ua-platform-version", `"19.0.0"`)
	h.Set("Sec-Fetch-Dest", "empty")
	h.Set("Sec-Fetch-Mode", "cors")
	h.Set("Sec-Fetch-Site", "same-origin")
	h.Set("OAI-Device-Id", c.deviceID)
	h.Set("OAI-Session-Id", c.sessionID)
	h.Set("OAI-Language", "zh-CN")
	h.Set("OAI-Client-Version", editableClientVer)
	h.Set("OAI-Client-Build-Number", editableClientBuild)
	h.Set("X-OpenAI-Target-Path", p)
	h.Set("X-OpenAI-Target-Route", p)
	if c.token != "" {
		h.Set("Authorization", "Bearer "+c.token)
	}
	if c.accountID != "" {
		h.Set("Chatgpt-Account-Id", c.accountID)
	}
}

// ---- 各步骤 ----

func (c *client) bootstrap(ctx context.Context) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/", nil)
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("sec-ch-ua", secChUA)
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	resp, err := c.bc.Do(req)
	if err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	if isCloudflareBlockHTML(resp.StatusCode, raw) {
		// 预热阶段就被 Cloudflare 拦：拿不到 cf_clearance，后续 /backend-api/f/* 必然 403。
		c.log("预热被 Cloudflare 拦截（未取得 cf_clearance），网页链路大概率不可用")
	}
	return nil
}

func (c *client) getChatRequirements(ctx context.Context) (*chatRequirements, error) {
	// turnstile.required 高度受会话/IP 信誉影响：首次拿到往往因 cf_clearance 未热好而触发，
	// 重新 bootstrap 预热一次后多半会消失。最多重试 3 次。
	var lastCR *chatRequirements
	for attempt := 0; attempt < 3; attempt++ {
		cr, _, turnstile, err := c.chatRequirementsOnce(ctx)
		if err != nil {
			// 403 多为风控临时受限：重新预热后重试，而非直接放弃。
			if attempt < 2 && strings.Contains(err.Error(), "403") {
				c.log(fmt.Sprintf("chat-requirements 403，重新预热后重试（%d/2）…", attempt+1))
				_ = c.bootstrap(ctx)
				time.Sleep(time.Duration(800*(attempt+1)) * time.Millisecond)
				continue
			}
			return nil, err
		}
		if !turnstile {
			return cr, nil
		}
		// 即使标记 turnstile，也保留 token + PoW：实测带 PoW 令牌仍能继续走
		// prepare/conversation（参考可用实现的做法），不再因 turnstile 直接放弃。
		lastCR = cr
		if attempt < 2 {
			_ = c.bootstrap(ctx)
			time.Sleep(800 * time.Millisecond)
		}
	}
	if lastCR != nil {
		c.log("chat-requirements 标记 turnstile，仍用 PoW 令牌继续尝试…")
		return lastCR, nil
	}
	return nil, ErrTurnstile
}

// chatRequirementsOnce 发一次 chat-requirements，返回解析结果 / turnstile dx / 是否要求 turnstile。
func (c *client) chatRequirementsOnce(ctx context.Context) (*chatRequirements, string, bool, error) {
	p := "/backend-api/sentinel/chat-requirements"
	body, _ := json.Marshal(map[string]string{"p": buildLegacyRequirementsToken(c.userAgent)})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+p, bytes.NewReader(body))
	c.setCommon(req, p)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	resp, err := c.bc.Do(req)
	if err != nil {
		return nil, "", false, fmt.Errorf("chat-requirements: %w", err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		if isCloudflareBlockHTML(resp.StatusCode, raw) {
			return nil, "", false, fmt.Errorf("%w（chat-requirements HTTP %d）", ErrCloudflare, resp.StatusCode)
		}
		return nil, "", false, fmt.Errorf("chat-requirements HTTP %d: %s", resp.StatusCode, snippet(raw, 300))
	}
	var out struct {
		Token  string `json:"token"`
		Arkose struct {
			Required bool `json:"required"`
		} `json:"arkose"`
		Turnstile struct {
			Required bool   `json:"required"`
			DX       string `json:"dx"`
		} `json:"turnstile"`
		ProofOfWork struct {
			Required   bool   `json:"required"`
			Seed       string `json:"seed"`
			Difficulty string `json:"difficulty"`
		} `json:"proofofwork"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, "", false, fmt.Errorf("chat-requirements decode: %w", err)
	}
	if out.Arkose.Required {
		return nil, "", false, fmt.Errorf("chat-requirements 需要 arkose 验证，暂不支持")
	}
	if out.Token == "" {
		return nil, "", false, fmt.Errorf("chat-requirements 未返回 token")
	}
	cr := &chatRequirements{Token: out.Token}
	if out.ProofOfWork.Required {
		cr.ProofToken = buildProofToken(out.ProofOfWork.Seed, out.ProofOfWork.Difficulty, c.userAgent)
	}
	if out.Turnstile.Required {
		return cr, out.Turnstile.DX, true, nil
	}
	return cr, "", false, nil
}

func (c *client) uploadImage(ctx context.Context, b64 string, index int) (*uploadedImage, error) {
	data, mimeType, w, h, err := decodeImage(b64)
	if err != nil {
		return nil, err
	}
	ext := ".png"
	switch mimeType {
	case "image/jpeg":
		ext = ".jpg"
	case "image/gif":
		ext = ".gif"
	case "image/webp":
		ext = ".webp"
	}
	fileName := fmt.Sprintf("image_%d%s", index, ext)

	// 1) 申请上传
	p := "/backend-api/files"
	reqBody, _ := json.Marshal(map[string]any{
		"file_name":                 fileName,
		"file_size":                 len(data),
		"use_case":                  "multimodal",
		"timezone_offset_min":       -480,
		"reset_rate_limits":         false,
		"store_in_library":          true,
		"library_persistence_mode":  "opportunistic",
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+p, bytes.NewReader(reqBody))
	c.setCommon(req, p)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	resp, err := c.bc.Do(req)
	if err != nil {
		return nil, err
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode == 402 {
		return nil, fmt.Errorf("files HTTP 402：该账号无可用订阅/额度，网页链路（套图/PPT/PSD）需 Plus/Team/Pro 订阅")
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("files HTTP %d: %s", resp.StatusCode, snippet(raw, 300))
	}
	var up struct {
		UploadURL     string `json:"upload_url"`
		FileID        string `json:"file_id"`
		LibraryFileID string `json:"library_file_id"`
	}
	json.Unmarshal(raw, &up)
	if up.UploadURL == "" || up.FileID == "" {
		return nil, fmt.Errorf("files 返回无效: %s", snippet(raw, 200))
	}

	// 2) PUT 到 Azure blob（无鉴权）
	putReq, _ := http.NewRequestWithContext(ctx, http.MethodPut, up.UploadURL, bytes.NewReader(data))
	putReq.Header.Set("Content-Type", mimeType)
	putReq.Header.Set("x-ms-blob-type", "BlockBlob")
	putReq.Header.Set("x-ms-version", "2020-04-08")
	putReq.Header.Set("Origin", baseURL)
	putReq.Header.Set("Referer", baseURL+"/")
	putReq.Header.Set("User-Agent", c.userAgent)
	putReq.Header.Set("Accept", "application/json, text/plain, */*")
	putResp, err := c.bc.Do(putReq)
	if err != nil {
		return nil, fmt.Errorf("blob upload: %w", err)
	}
	io.Copy(io.Discard, putResp.Body)
	putResp.Body.Close()
	if putResp.StatusCode >= 400 {
		return nil, fmt.Errorf("blob upload HTTP %d", putResp.StatusCode)
	}

	// 3) 标记上传完成
	p2 := "/backend-api/files/" + up.FileID + "/uploaded"
	req2, _ := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+p2, strings.NewReader("{}"))
	c.setCommon(req2, p2)
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Accept", "*/*")
	resp2, err := c.bc.Do(req2)
	if err != nil {
		return nil, err
	}
	io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode == 402 {
		return nil, fmt.Errorf("uploaded HTTP 402：该账号无可用订阅/额度，网页链路（套图/PPT/PSD）需 Plus/Team/Pro 订阅")
	}
	if resp2.StatusCode >= 400 {
		return nil, fmt.Errorf("uploaded HTTP %d", resp2.StatusCode)
	}

	return &uploadedImage{
		FileID:        up.FileID,
		LibraryFileID: up.LibraryFileID,
		FileName:      fileName,
		FileSize:      int64(len(data)),
		MimeType:      mimeType,
		Width:         w,
		Height:        h,
	}, nil
}

func (c *client) prepareConversation(ctx context.Context, reqs *chatRequirements, prompt string, mimeTypes []string, systemHints []string) (string, error) {
	p := "/backend-api/f/conversation/prepare"
	if systemHints == nil {
		systemHints = []string{}
	}
	payload := map[string]any{
		"action":               "next",
		"fork_from_shared_post": false,
		"parent_message_id":    "client-created-root",
		"model":                editableFileModel,
		"client_prepare_state": "success",
		"timezone_offset_min":  -480,
		"timezone":             "Asia/Shanghai",
		"conversation_mode":    map[string]any{"kind": "primary_assistant"},
		"system_hints":         systemHints,
		"partial_query": map[string]any{
			"id":      uuid.NewString(),
			"author":  map[string]any{"role": "user"},
			"content": map[string]any{"content_type": "text", "parts": []string{prompt}},
		},
		"supports_buffering":    true,
		"supported_encodings":   []string{"v1"},
		"client_contextual_info": map[string]any{"app_name": "chatgpt.com"},
		"thinking_effort":       editableThinking,
	}
	if len(mimeTypes) > 0 {
		payload["attachment_mime_types"] = mimeTypes
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+p, bytes.NewReader(body))
	c.setCommon(req, p)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	// prepare 必须带上 chat-requirements 的 sentinel 令牌（token + PoW），
	// 否则 OpenAI 风控直接返回 403 HTML 页（这正是之前误判为 Cloudflare 的根因）。
	if reqs != nil {
		req.Header.Set("OpenAI-Sentinel-Chat-Requirements-Token", reqs.Token)
		if reqs.ProofToken != "" {
			req.Header.Set("OpenAI-Sentinel-Proof-Token", reqs.ProofToken)
		}
	}
	req.Header.Set("X-Oai-Turn-Trace-Id", uuid.NewString())
	resp, err := c.bc.Do(req)
	if err != nil {
		return "", err
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		if isCloudflareBlockHTML(resp.StatusCode, raw) {
			return "", fmt.Errorf("%w（prepare HTTP %d）", ErrCloudflare, resp.StatusCode)
		}
		return "", fmt.Errorf("prepare HTTP %d: %s", resp.StatusCode, snippet(raw, 300))
	}
	var out struct {
		ConduitToken string `json:"conduit_token"`
	}
	json.Unmarshal(raw, &out)
	if out.ConduitToken == "" {
		return "", fmt.Errorf("prepare 未返回 conduit_token: %s", snippet(raw, 200))
	}
	return out.ConduitToken, nil
}

func (c *client) runConversation(ctx context.Context, reqs *chatRequirements, prompt string, uploaded []uploadedImage, conduit string) (string, error) {
	if reqs == nil {
		return "", fmt.Errorf("缺少 chat-requirements 令牌")
	}
	message := buildUserMessage(prompt, uploaded)

	p := "/backend-api/f/conversation"
	payload, _ := json.Marshal(map[string]any{
		"action":               "next",
		"messages":             []any{message},
		"parent_message_id":    "client-created-root",
		"model":                editableFileModel,
		"client_prepare_state": "sent",
		"timezone_offset_min":  -480,
		"timezone":             "Asia/Shanghai",
		"conversation_mode":    map[string]any{"kind": "primary_assistant"},
		"enable_message_followups": true,
		"system_hints":         []string{},
		"supports_buffering":   true,
		"supported_encodings":  []string{"v1"},
		"client_contextual_info": map[string]any{
			"is_dark_mode":     false,
			"time_since_loaded": 401,
			"page_height":      1138,
			"page_width":       803,
			"pixel_ratio":      2,
			"screen_height":    1440,
			"screen_width":     2560,
			"app_name":         "chatgpt.com",
		},
		"paragen_cot_summary_display_override": "allow",
		"force_parallel_switch":                "auto",
		"thinking_effort":                      editableThinking,
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+p, bytes.NewReader(payload))
	c.setCommon(req, p)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("OpenAI-Sentinel-Chat-Requirements-Token", reqs.Token)
	if reqs.ProofToken != "" {
		req.Header.Set("OpenAI-Sentinel-Proof-Token", reqs.ProofToken)
	}
	if conduit != "" {
		req.Header.Set("X-Conduit-Token", conduit)
	}
	req.Header.Set("X-Oai-Turn-Trace-Id", uuid.NewString())
	resp, err := c.bc.Do(req)
	if err != nil {
		return "", fmt.Errorf("conversation: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if isCloudflareBlockHTML(resp.StatusCode, raw) {
			return "", fmt.Errorf("%w（conversation HTTP %d）", ErrCloudflare, resp.StatusCode)
		}
		return "", fmt.Errorf("conversation HTTP %d: %s", resp.StatusCode, snippet(raw, 300))
	}
	convID := ""
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		if convID == "" {
			if m := conversationIDRe.FindStringSubmatch(data); len(m) > 1 {
				convID = m[1]
				break
			}
		}
	}
	if convID == "" {
		return "", fmt.Errorf("会话流中未找到 conversation_id")
	}
	return convID, nil
}

func (c *client) waitArtifacts(ctx context.Context, convID string, exportRe *regexp.Regexp, primarySuffix []string) ([]artifact, error) {
	deadline := time.Now().Add(editableTimeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		conv, err := c.getConversationDetail(ctx, convID)
		if err != nil {
			time.Sleep(editablePollInterval)
			continue
		}
		arts := extractArtifacts(conv, exportRe)
		primary, zip := pickArtifacts(arts, primarySuffix)
		if primary != nil && zip != nil {
			return []artifact{*primary, *zip}, nil
		}
		time.Sleep(editablePollInterval)
	}
	return nil, fmt.Errorf("等待 %s/zip 产物超时（%.0f 分钟）", strings.ToUpper(strings.TrimPrefix(primarySuffix[0], ".")), editableTimeout.Minutes())
}

func (c *client) getConversationDetail(ctx context.Context, convID string) (map[string]any, error) {
	p := "/backend-api/conversation/" + convID
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+p, nil)
	c.setCommon(req, p)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Referer", baseURL+"/c/"+convID)
	resp, err := c.bc.Do(req)
	if err != nil {
		return nil, err
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("conversation detail HTTP %d", resp.StatusCode)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *client) downloadArtifact(ctx context.Context, convID string, a artifact, outputDir, kind string) (*File, error) {
	url, err := c.resolveDownloadURL(ctx, convID, a)
	if err != nil {
		return nil, err
	}
	if url == "" {
		return nil, fmt.Errorf("未找到下载地址")
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.bc.Do(req)
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("download HTTP %d", resp.StatusCode)
	}
	contentType := cleanMime(resp.Header.Get("Content-Type"))
	name := resolveOutputName(a, resp.Header.Get("Content-Disposition"), contentType, kind)
	target := uniquePath(outputDir, name)
	if err := writeFile(target, body); err != nil {
		return nil, err
	}
	return &File{Path: target, Name: path.Base(target), Mime: firstNonEmpty(contentType, a.MimeType), Size: int64(len(body))}, nil
}

func (c *client) resolveDownloadURL(ctx context.Context, convID string, a artifact) (string, error) {
	ids := make([]string, 0, 2)
	for _, id := range []string{a.AttachmentID, a.FileID} {
		if id != "" && !contains(ids, id) {
			ids = append(ids, id)
		}
	}
	// interpreter/download（sandbox 路径）
	if a.SandboxPath != "" && a.MessageID != "" {
		p := "/backend-api/conversation/" + convID + "/interpreter/download"
		u := baseURL + p + "?message_id=" + queryEscape(a.MessageID) + "&sandbox_path=" + queryEscape(a.SandboxPath)
		if url := c.tryDownloadURL(ctx, u, p, convID); url != "" {
			return url, nil
		}
	}
	for _, id := range ids {
		p := "/backend-api/conversation/" + convID + "/attachment/" + id + "/download"
		if url := c.tryDownloadURL(ctx, baseURL+p, p, convID); url != "" {
			return url, nil
		}
	}
	for _, id := range ids {
		p := "/backend-api/files/download/" + id
		u := baseURL + p + "?post_id=&inline=false"
		if url := c.tryDownloadURL(ctx, u, p, convID); url != "" {
			return url, nil
		}
	}
	for _, id := range ids {
		p := "/backend-api/files/" + id + "/download"
		if url := c.tryDownloadURL(ctx, baseURL+p, p, convID); url != "" {
			return url, nil
		}
	}
	return "", nil
}

func (c *client) tryDownloadURL(ctx context.Context, fullURL, p, convID string) string {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	c.setCommon(req, p)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Referer", baseURL+"/c/"+convID)
	resp, err := c.bc.Do(req)
	if err != nil {
		return ""
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ""
	}
	var out struct {
		DownloadURL string `json:"download_url"`
		URL         string `json:"url"`
	}
	json.Unmarshal(raw, &out)
	return firstNonEmpty(out.DownloadURL, out.URL)
}

// ---- 产物解析 ----

func extractArtifacts(conv map[string]any, exportRe *regexp.Regexp) []artifact {
	mapping, _ := conv["mapping"].(map[string]any)
	if mapping == nil {
		return nil
	}
	merged := map[string]artifact{}
	for _, node := range mapping {
		nm, _ := node.(map[string]any)
		if nm == nil {
			continue
		}
		msg, _ := nm["message"].(map[string]any)
		if msg == nil {
			continue
		}
		role := ""
		if author, ok := msg["author"].(map[string]any); ok {
			role, _ = author["role"].(string)
		}
		if role != "assistant" && role != "tool" {
			continue
		}
		messageID, _ := msg["id"].(string)
		createTime := toFloat(msg["create_time"])
		msgJSON, _ := json.Marshal(msg)
		msgText := string(msgJSON)

		// 附件
		if meta, ok := msg["metadata"].(map[string]any); ok {
			if atts, ok := meta["attachments"].([]any); ok {
				for _, it := range atts {
					if m, ok := it.(map[string]any); ok {
						a := artifactFromMap(m, messageID, createTime, exportRe)
						if a != nil {
							mergeInto(merged, *a)
						}
					}
				}
			}
		}

		// 文本里的 sandbox 导出路径
		for _, ep := range exportRe.FindAllStringSubmatch(msgText, -1) {
			if len(ep) > 1 {
				p := ep[1]
				mergeInto(merged, artifact{
					Name:        path.Base(p),
					CreateTime:  createTime,
					SandboxPath: p,
					MessageID:   messageID,
				})
			}
		}
	}
	out := make([]artifact, 0, len(merged))
	for _, a := range merged {
		out = append(out, a)
	}
	return out
}

func artifactFromMap(m map[string]any, messageID string, createTime float64, exportRe *regexp.Regexp) *artifact {
	id, _ := m["id"].(string)
	fileID, _ := m["file_id"].(string)
	name := strFirst(m, "name", "file_name", "filename", "title")
	mime := cleanMime(strFirst(m, "mime_type", "mimeType"))
	assetPointer, _ := m["asset_pointer"].(string)
	for _, ap := range assetPointerRe.FindAllStringSubmatch(assetPointer, -1) {
		if len(ap) > 1 {
			if id == "" {
				id = ap[1]
			}
			if fileID == "" {
				fileID = ap[1]
			}
		}
	}
	attachmentID := matchFileID(id)
	fid := matchFileID(fileID)
	if attachmentID == "" && fid == "" {
		// 在整个 payload 里兜底找 file id
		mj, _ := json.Marshal(m)
		if ids := extractFileIDs(string(mj)); len(ids) > 0 {
			attachmentID = ids[0]
			fid = ids[0]
		}
	}
	sandbox := ""
	mj, _ := json.Marshal(m)
	if ep := exportRe.FindStringSubmatch(string(mj)); len(ep) > 1 {
		sandbox = ep[1]
	}
	if attachmentID == "" && fid == "" && sandbox == "" {
		return nil
	}
	return &artifact{
		AttachmentID: attachmentID,
		FileID:       fid,
		Name:         name,
		MimeType:     mime,
		CreateTime:   createTime,
		SandboxPath:  sandbox,
		MessageID:    messageID,
	}
}

func mergeInto(m map[string]artifact, a artifact) {
	key := firstNonEmpty(a.AttachmentID, a.FileID, a.Name, a.SandboxPath)
	if key == "" {
		return
	}
	cur, ok := m[key]
	if !ok {
		m[key] = a
		return
	}
	m[key] = artifact{
		AttachmentID: firstNonEmpty(a.AttachmentID, cur.AttachmentID),
		FileID:       firstNonEmpty(a.FileID, cur.FileID),
		Name:         firstNonEmpty(a.Name, cur.Name),
		MimeType:     firstNonEmpty(a.MimeType, cur.MimeType),
		CreateTime:   maxFloat(cur.CreateTime, a.CreateTime),
		SandboxPath:  firstNonEmpty(a.SandboxPath, cur.SandboxPath),
		MessageID:    firstNonEmpty(a.MessageID, cur.MessageID),
	}
}

func pickArtifacts(arts []artifact, primarySuffix []string) (*artifact, *artifact) {
	var primary, zip *artifact
	for i := range arts {
		a := arts[i]
		if looksLikePrimary(a, primarySuffix) {
			if primary == nil || a.CreateTime >= primary.CreateTime {
				cp := a
				primary = &cp
			}
		}
		if looksLikeZip(a) {
			if zip == nil || a.CreateTime >= zip.CreateTime {
				cp := a
				zip = &cp
			}
		}
	}
	return primary, zip
}

func looksLikePrimary(a artifact, suffixes []string) bool {
	name := strings.ToLower(a.Name)
	p := strings.ToLower(a.SandboxPath)
	for _, s := range suffixes {
		if strings.HasSuffix(name, s) || strings.HasSuffix(p, s) {
			return true
		}
	}
	mime := a.MimeType
	return strings.Contains(mime, "presentationml.presentation") ||
		strings.Contains(mime, "ms-powerpoint") ||
		strings.Contains(mime, "photoshop")
}

func looksLikeZip(a artifact) bool {
	name := strings.ToLower(a.Name)
	p := strings.ToLower(a.SandboxPath)
	return strings.HasSuffix(name, ".zip") || strings.HasSuffix(p, ".zip") ||
		strings.HasSuffix(a.MimeType, "/zip") || a.MimeType == "application/zip" || a.MimeType == "application/x-zip-compressed"
}

// ---- 工具 ----

func matchFileID(v string) string {
	m := fileIDRe.FindString(v)
	if m == "" {
		return ""
	}
	if strings.HasPrefix(m, "file-service") || strings.HasPrefix(m, "file_service") {
		return ""
	}
	return m
}

func extractFileIDs(text string) []string {
	out := []string{}
	for _, ap := range assetPointerRe.FindAllStringSubmatch(text, -1) {
		if len(ap) > 1 && !contains(out, ap[1]) {
			out = append(out, ap[1])
		}
	}
	for _, m := range fileIDRe.FindAllString(text, -1) {
		if strings.HasPrefix(m, "file-service") || strings.HasPrefix(m, "file_service") {
			continue
		}
		if !contains(out, m) {
			out = append(out, m)
		}
	}
	return out
}

func decodeImage(b64 string) ([]byte, string, int, int, error) {
	raw := strings.TrimSpace(b64)
	mimeType := ""
	if strings.HasPrefix(raw, "data:") {
		if idx := strings.Index(raw, ";base64,"); idx > 0 {
			mimeType = strings.TrimPrefix(raw[:idx], "data:")
			raw = raw[idx+len(";base64,"):]
		}
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(raw))
	if err != nil {
		return nil, "", 0, 0, fmt.Errorf("base64 解码失败: %w", err)
	}
	cfg, format, derr := image.DecodeConfig(bytes.NewReader(data))
	if derr == nil {
		switch format {
		case "jpeg":
			mimeType = "image/jpeg"
		case "png":
			mimeType = "image/png"
		case "gif":
			mimeType = "image/gif"
		}
		return data, firstNonEmpty(mimeType, "image/png"), cfg.Width, cfg.Height, nil
	}
	return data, firstNonEmpty(mimeType, "image/png"), 0, 0, nil
}

func resolveOutputName(a artifact, contentDisposition, contentType, kind string) string {
	name := sanitizeName(a.Name)
	if name == "" && a.SandboxPath != "" {
		name = sanitizeName(path.Base(a.SandboxPath))
	}
	if name == "" {
		name = sanitizeName(filenameFromContentDisposition(contentDisposition))
	}
	if name != "" {
		if path.Ext(name) != "" {
			return name
		}
		return name + extFromMime(contentType, kind)
	}
	return "artifact" + extFromMime(contentType, kind)
}

func extFromMime(mime, kind string) string {
	if strings.Contains(mime, "presentationml.presentation") || strings.Contains(mime, "ms-powerpoint") {
		return ".pptx"
	}
	if strings.Contains(mime, "photoshop") {
		return ".psd"
	}
	if strings.HasSuffix(mime, "/zip") || mime == "application/zip" || mime == "application/x-zip-compressed" {
		return ".zip"
	}
	if kind == "psd" {
		return ".psd"
	}
	return ".pptx"
}

var filenameStarRe = regexp.MustCompile(`(?i)filename\*=UTF-8''([^;]+)`)
var filenamePlainRe = regexp.MustCompile(`(?i)filename="([^"]+)"`)

func filenameFromContentDisposition(cd string) string {
	if m := filenameStarRe.FindStringSubmatch(cd); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	if m := filenamePlainRe.FindStringSubmatch(cd); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// ---- Sentinel Proof-of-Work（sha3-512）----

func buildLegacyRequirementsToken(userAgent string) string {
	seed := fmt.Sprintf("%0.16f", mrand.Float64())
	answer, _ := powGenerate(seed, "0fffff", powConfig(userAgent))
	return "gAAAAAC" + answer
}

func buildProofToken(seed, difficulty, userAgent string) string {
	answer, solved := powGenerate(seed, difficulty, powConfig(userAgent))
	if !solved {
		return "gAAAAAB" + base64.StdEncoding.EncodeToString([]byte(`"`+seed+`"`))
	}
	return "gAAAAAB" + answer
}

func powConfig(userAgent string) []any {
	return []any{
		3000 + mrand.Intn(3)*1000,
		time.Now().In(time.FixedZone("EST", -5*3600)).Format("Mon Jan 02 2006 15:04:05") + " GMT-0500 (Eastern Standard Time)",
		4294705152,
		0,
		userAgent,
		"https://chatgpt.com/backend-api/sentinel/sdk.js",
		"",
		"en-US",
		"en-US,es-US,en,es",
		0,
		"webdriver≭false",
		"location",
		"window",
		float64(time.Now().UnixNano()) / 1e6,
		uuid.NewString(),
		"",
		16,
		float64(time.Now().UnixNano()) / 1e6,
	}
}

func powGenerate(seed, difficulty string, config []any) (string, bool) {
	diffBytes, err := hexToBytes(difficulty)
	if err != nil || len(diffBytes) == 0 {
		return base64.StdEncoding.EncodeToString([]byte(`"` + seed + `"`)), false
	}
	static1 := mustJSON(config[:3])
	static1 = strings.TrimSuffix(static1, "]") + ","
	static2 := "," + strings.TrimPrefix(strings.TrimSuffix(mustJSON(config[4:9]), "]"), "[") + ","
	static3 := "," + strings.TrimPrefix(mustJSON(config[10:]), "[")
	seedBytes := []byte(seed)
	for i := 0; i < 500000; i++ {
		final := static1 + fmt.Sprint(i) + static2 + fmt.Sprint(i>>1) + static3
		encoded := base64.StdEncoding.EncodeToString([]byte(final))
		h := sha3.Sum512(append(seedBytes, []byte(encoded)...))
		if bytes.Compare(h[:len(diffBytes)], diffBytes) <= 0 {
			return encoded, true
		}
	}
	return base64.StdEncoding.EncodeToString([]byte(`"` + seed + `"`)), false
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func hexToBytes(s string) ([]byte, error) {
	if len(s)%2 == 1 {
		s = "0" + s
	}
	out := make([]byte, len(s)/2)
	for i := 0; i < len(out); i++ {
		var x byte
		for j := 0; j < 2; j++ {
			c := s[i*2+j]
			x <<= 4
			switch {
			case c >= '0' && c <= '9':
				x |= c - '0'
			case c >= 'a' && c <= 'f':
				x |= c - 'a' + 10
			case c >= 'A' && c <= 'F':
				x |= c - 'A' + 10
			default:
				return nil, fmt.Errorf("invalid hex")
			}
		}
		out[i] = x
	}
	return out, nil
}

// ---- 小工具 ----

func toFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case json.Number:
		f, _ := t.Float64()
		return f
	}
	return 0
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func strFirst(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func cleanMime(v string) string {
	text := strings.ToLower(strings.TrimSpace(v))
	if !strings.Contains(text, "/") {
		return ""
	}
	if idx := strings.Index(text, ";"); idx > 0 {
		return strings.TrimSpace(text[:idx])
	}
	return text
}

func sanitizeName(v string) string {
	v = strings.ReplaceAll(strings.TrimSpace(v), "\x00", "")
	return strings.TrimSpace(path.Base(v))
}

func queryEscape(s string) string {
	var b strings.Builder
	for _, r := range []byte(s) {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' || r == '~' || r == '/' {
			b.WriteByte(r)
		} else {
			b.WriteString(fmt.Sprintf("%%%02X", r))
		}
	}
	return b.String()
}

func snippet(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string([]rune(string(b))[:n]) + "..."
}

func writeFile(p string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	return os.WriteFile(p, data, 0644)
}

func uniquePath(dir, name string) string {
	_ = os.MkdirAll(dir, 0755)
	base := filepath.Join(dir, name)
	if _, err := os.Stat(base); os.IsNotExist(err) {
		return base
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 1; i < 1000; i++ {
		cand := filepath.Join(dir, fmt.Sprintf("%s_%d%s", stem, i, ext))
		if _, err := os.Stat(cand); os.IsNotExist(err) {
			return cand
		}
	}
	return base
}
