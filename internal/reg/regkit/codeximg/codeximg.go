// Package codeximg 逆向 ChatGPT Codex 的画图接口（chatgpt.com/backend-api/codex/responses），
// 用 Plus/Team/Pro 订阅账号的 access_token 出图，不消耗官方 API token 配额。
//
// 走 codex responses 流式接口，挂 image_generation 工具，
// 解析 SSE 拿 image_generation_call 的 b64/url。支持 n 多图（循环请求）、编辑（带参考图）。
package codeximg

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"web2img/internal/reg/regkit/browser"
)

const (
	codexResponsesURL = "https://chatgpt.com/backend-api/codex/responses"
	codexCLIUserAgent = "codex_cli_rs/0.125.0"
	codexCLIVersion   = "0.125.0"
)

// Options 一次出图请求参数。
type Options struct {
	AccessToken  string
	AccountID    string // chatgpt_account_id（部分账号必须带 Chatgpt-Account-Id 头）
	ProxyURL     string
	Prompt       string
	ToolModel    string   // gpt-image-2（默认）
	MainModel    string   // gpt-5.5（默认）
	Size         string   // 例如 1024x1024
	Quality      string   // low/medium/high
	OutputFormat string   // png/jpeg/webp（默认 png）
	Action       string   // generate / edit
	RefImages    []string // 编辑用参考图（data URL 或 http URL）
	N            int      // 张数
	Instructions string
	Log          func(string) // 进度日志回调（可空）
}

func (o Options) log(msg string) {
	if o.Log != nil {
		o.Log(msg)
	}
}

// Image 出图结果。
type Image struct {
	B64    string
	URL    string
	Mime   string
	Width  int
	Height int
}

type responseInputItem struct {
	Type    string           `json:"type"`
	Role    string           `json:"role"`
	Content []map[string]any `json:"content"`
}

type responseReq struct {
	Instructions      string           `json:"instructions"`
	Stream            bool             `json:"stream"`
	Reasoning         map[string]any   `json:"reasoning,omitempty"`
	ParallelToolCalls bool             `json:"parallel_tool_calls"`
	Include           []string         `json:"include,omitempty"`
	Model             string           `json:"model"`
	Store             bool             `json:"store"`
	ToolChoice        any              `json:"tool_choice,omitempty"`
	Input             any              `json:"input"`
	Tools             []map[string]any `json:"tools"`
}

type responseCompletedEvent struct {
	Type     string `json:"type"`
	Response struct {
		Output []responseOutputItem `json:"output"`
	} `json:"response"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

type responseOutputItem struct {
	Type          string `json:"type"`
	Result        string `json:"result"`
	B64JSON       string `json:"b64_json"`
	ImageB64      string `json:"image_b64"`
	URL           string `json:"url"`
	OutputFormat  string `json:"output_format"`
	Size          string `json:"size"`
	RevisedPrompt string `json:"revised_prompt"`
	Content       []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		Result   string `json:"result"`
		B64JSON  string `json:"b64_json"`
		ImageB64 string `json:"image_b64"`
		URL      string `json:"url"`
	} `json:"content"`
}

// Generate 调用 codex responses 出 N 张图。
func Generate(ctx context.Context, opt Options) ([]Image, error) {
	if strings.TrimSpace(opt.AccessToken) == "" {
		return nil, fmt.Errorf("缺少 access_token")
	}
	toolModel := strings.TrimSpace(opt.ToolModel)
	if toolModel == "" {
		toolModel = "gpt-image-2"
	}
	mainModel := strings.TrimSpace(opt.MainModel)
	if mainModel == "" {
		mainModel = "gpt-5.5"
	}
	size := strings.TrimSpace(opt.Size)
	if size == "" {
		size = "1024x1024"
	}
	action := strings.TrimSpace(opt.Action)
	if action == "" {
		if len(opt.RefImages) > 0 {
			action = "edit"
		} else {
			action = "generate"
		}
	}
	n := opt.N
	if n <= 0 {
		n = 1
	}
	instructions := strings.TrimSpace(opt.Instructions)
	if instructions == "" {
		instructions = "You are a helpful AI assistant. When the user describes or asks for an image, " +
			"or asks to edit/transform a reference image, use the image_generation tool to create the image."
	}

	content := []map[string]any{{"type": "input_text", "text": opt.Prompt}}
	for _, ref := range opt.RefImages {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		content = append(content, map[string]any{"type": "input_image", "image_url": ref})
	}
	tool := map[string]any{
		"type":   "image_generation",
		"action": action,
		"model":  toolModel,
		"size":   size,
	}
	if opt.Quality != "" {
		tool["quality"] = opt.Quality
	}
	if opt.OutputFormat != "" {
		tool["output_format"] = opt.OutputFormat
	}
	body := responseReq{
		Instructions:      instructions,
		Stream:            true,
		Reasoning:         map[string]any{"effort": "medium", "summary": "auto"},
		ParallelToolCalls: true,
		Include:           []string{"reasoning.encrypted_content"},
		Model:             mainModel,
		Store:             false,
		ToolChoice:        "auto",
		Input:             []responseInputItem{{Type: "message", Role: "user", Content: content}},
		Tools:             []map[string]any{tool},
	}

	bc, err := browser.New(browser.Options{ProxyURL: opt.ProxyURL, Timeout: 240 * time.Second})
	if err != nil {
		return nil, err
	}
	// 预热 chatgpt.com 拿 cf_clearance，避免 codex/responses 被 Cloudflare 拦。
	opt.log("预热 chatgpt.com 会话（cf_clearance）…")
	bootstrapCodex(ctx, bc)
	width, height := parseSize(size)
	images := make([]Image, 0, n)
	var lastErr error
	opt.log(fmt.Sprintf("开始 Codex 出图：模型 %s，尺寸 %s，共 %d 张", toolModel, size, n))
	for i := 0; i < n && len(images) < n; i++ {
		opt.log(fmt.Sprintf("请求第 %d/%d 张…", i+1, n))
		got, err := doOnce(ctx, bc, body, opt)
		if err != nil {
			lastErr = err
			opt.log(fmt.Sprintf("第 %d 张失败：%v", i+1, err))
			continue
		}
		for _, im := range got {
			if im.Width == 0 {
				im.Width = width
			}
			if im.Height == 0 {
				im.Height = height
			}
			images = append(images, im)
			opt.log(fmt.Sprintf("已取得 %d 张", len(images)))
			if len(images) >= n {
				break
			}
		}
	}
	if len(images) == 0 {
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, fmt.Errorf("codex 出图返回 0 张")
	}
	return images, nil
}

// bootstrapCodex 先 GET 一次 chatgpt.com 主页，让 cookie jar 拿到 cf_clearance，
// 避免后续 codex/responses 被 Cloudflare 直接拦截。失败忽略（不影响主流程报错）。
func bootstrapCodex(ctx context.Context, bc *browser.Client) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://chatgpt.com/", nil)
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36 Edg/143.0.0.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	resp, err := bc.Do(req)
	if err != nil {
		return
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
}

func doOnce(ctx context.Context, bc *browser.Client, body responseReq, opt Options) ([]Image, error) {
	attempt := body
	retriedNoToolChoice := false
	for {
		payload, _ := json.Marshal(attempt)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, codexResponsesURL, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+opt.AccessToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("User-Agent", codexCLIUserAgent)
		req.Header.Set("OpenAI-Beta", "responses=experimental")
		req.Header.Set("originator", "codex_cli_rs")
		req.Header.Set("version", codexCLIVersion)
		req.Header.Set("session_id", uuid.NewString())
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		if opt.AccountID != "" {
			req.Header.Set("Chatgpt-Account-Id", opt.AccountID)
		}
		opt.log("已发送请求，等待 codex 响应…")
		resp, err := bc.Do(req)
		if err != nil {
			return nil, fmt.Errorf("codex 请求失败: %w", err)
		}
		opt.log(fmt.Sprintf("收到响应头 HTTP %d（content-type: %s）", resp.StatusCode, resp.Header.Get("Content-Type")))
		if resp.StatusCode >= 400 {
			raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			_ = resp.Body.Close()
			if !retriedNoToolChoice && shouldRetryWithoutToolChoice(raw) {
				opt.log("服务端拒绝 tool_choice，移除后重试…")
				attempt.ToolChoice = nil
				retriedNoToolChoice = true
				continue
			}
			if resp.StatusCode == 401 {
				return nil, fmt.Errorf("codex HTTP 401 未授权：该账号 token 不可用于 Codex 画图（多为网页/平台 token，或已失效）。请改用「网页 GPT-Image-2 套图」链路，或导入 Codex 类型账号")
			}
			return nil, fmt.Errorf("codex HTTP %d: %s", resp.StatusCode, snippet(raw, 320))
		}
		opt.log("开始接收事件流（SSE）…")
		completed, err := parseCompletedResponse(resp.Body, opt.log)
		_ = resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if completed.Error != nil && completed.Error.Message != "" {
			return nil, fmt.Errorf("codex: %s", completed.Error.Message)
		}
		out := make([]Image, 0, 2)
		for _, item := range completed.Response.Output {
			b64, u := outputImagePayload(item)
			if b64 == "" && u == "" {
				continue
			}
			mime := mimeForImageFormat(item.OutputFormat)
			w, h := 0, 0
			if item.Size != "" {
				w, h = parseSize(item.Size)
			}
			out = append(out, Image{B64: b64, URL: u, Mime: mime, Width: w, Height: h})
		}
		return out, nil
	}
}

func shouldRetryWithoutToolChoice(raw []byte) bool {
	msg := strings.ToLower(string(raw))
	return strings.Contains(msg, "tool choice") &&
		strings.Contains(msg, "image_generation") &&
		strings.Contains(msg, "not found") &&
		strings.Contains(msg, "tools")
}

func outputImagePayload(out responseOutputItem) (string, string) {
	if out.Result != "" {
		return out.Result, ""
	}
	if out.B64JSON != "" {
		return out.B64JSON, ""
	}
	if out.ImageB64 != "" {
		return out.ImageB64, ""
	}
	if out.URL != "" {
		return "", out.URL
	}
	for _, c := range out.Content {
		if c.Result != "" {
			return c.Result, ""
		}
		if c.B64JSON != "" {
			return c.B64JSON, ""
		}
		if c.ImageB64 != "" {
			return c.ImageB64, ""
		}
		if c.URL != "" {
			return "", c.URL
		}
	}
	return "", ""
}

func parseCompletedResponse(r io.Reader, logFn func(string)) (*responseCompletedEvent, error) {
	log := func(msg string) {
		if logFn != nil {
			logFn(msg)
		}
	}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	var dataLines []string
	var last *responseCompletedEvent
	var outputItems []responseOutputItem
	var partialItems []responseOutputItem
	seen := map[string]bool{}
	partialCount := 0
	flush := func() error {
		if len(dataLines) == 0 {
			return nil
		}
		data := strings.TrimSpace(strings.Join(dataLines, "\n"))
		dataLines = nil
		if data == "" || data == "[DONE]" {
			return nil
		}
		var ev responseCompletedEvent
		err := json.Unmarshal([]byte(data), &ev)
		var direct struct {
			Output []responseOutputItem `json:"output"`
			Item   responseOutputItem   `json:"item"`
		}
		if err2 := json.Unmarshal([]byte(data), &direct); err2 == nil {
			if len(ev.Response.Output) == 0 && len(direct.Output) > 0 {
				ev.Type = "response.completed"
				ev.Response.Output = direct.Output
			}
			if direct.Item.Type != "" && ev.Type == "" {
				ev.Type = "response.output_item.done"
			}
		}
		if err != nil && len(ev.Response.Output) == 0 && direct.Item.Type == "" {
			return err
		}
		// 流式事件进度日志：首次出现的事件类型记一条；部分预览帧单独计数。
		if ev.Type != "" {
			switch ev.Type {
			case "response.created":
				if !seen[ev.Type] {
					log("模型已建立响应会话…")
				}
			case "response.in_progress":
				if !seen[ev.Type] {
					log("模型处理中（生成图像）…")
				}
			case "response.image_generation_call.partial_image":
				partialCount++
				if partialCount == 1 || partialCount%3 == 0 {
					log(fmt.Sprintf("已接收图像预览帧 ×%d…", partialCount))
				}
			case "response.completed":
				log("响应完成，正在收尾…")
			default:
				if strings.Contains(ev.Type, "failed") || strings.Contains(ev.Type, "error") {
					log("事件：" + ev.Type)
				}
			}
			seen[ev.Type] = true
		}
		switch ev.Type {
		case "response.output_item.done":
			if direct.Item.Type != "" {
				outputItems = append(outputItems, direct.Item)
			}
		case "response.image_generation_call.partial_image":
			var partial struct {
				OutputFormat string `json:"output_format"`
				PartialB64   string `json:"partial_image_b64"`
			}
			if err := json.Unmarshal([]byte(data), &partial); err == nil && partial.PartialB64 != "" {
				partialItems = append(partialItems, responseOutputItem{
					Type:         "image_generation_call",
					Result:       partial.PartialB64,
					OutputFormat: partial.OutputFormat,
				})
			}
		}
		if ev.Error != nil && ev.Error.Message != "" {
			log("服务端返回错误：" + ev.Error.Message)
		}
		if ev.Type == "response.completed" || len(ev.Response.Output) > 0 || ev.Error != nil {
			last = &ev
		}
		return nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := flush(); err != nil {
				return nil, fmt.Errorf("codex stream decode: %w", err)
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	streamErr := scanner.Err()
	if err := flush(); err != nil && streamErr == nil {
		return nil, fmt.Errorf("codex stream decode: %w", err)
	}
	if last == nil {
		last = &responseCompletedEvent{Type: "response.completed"}
	}
	if len(last.Response.Output) == 0 && len(outputItems) > 0 {
		last.Response.Output = outputItems
	}
	if len(last.Response.Output) == 0 && len(partialItems) > 0 {
		last.Response.Output = partialItems
	}
	if streamErr != nil && len(last.Response.Output) == 0 {
		return nil, fmt.Errorf("codex stream read: %w", streamErr)
	}
	return last, nil
}

func mimeForImageFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "jpeg", "jpg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	default:
		return "image/png"
	}
}

func parseSize(size string) (int, int) {
	parts := strings.SplitN(strings.TrimSpace(size), "x", 2)
	if len(parts) != 2 {
		return 1024, 1024
	}
	var w, h int
	fmt.Sscanf(parts[0], "%d", &w)
	fmt.Sscanf(parts[1], "%d", &h)
	if w <= 0 {
		w = 1024
	}
	if h <= 0 {
		h = 1024
	}
	return w, h
}

func snippet(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	r := []rune(string(b))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "...(truncated)"
}
