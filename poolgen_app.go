package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"web2img/internal/reg/regkit/codeximg"
	"web2img/internal/reg/regkit/webfile"
)

// poolLog 把一条任务日志推送给前端（poolgen:log 事件）。taskID 为空时忽略。
func (a *App) poolLog(taskID, msg string) {
	if taskID == "" || a.ctx == nil {
		return
	}
	wailsruntime.EventsEmit(a.ctx, "poolgen:log", map[string]any{
		"taskId": taskID,
		"ts":     time.Now().UnixMilli(),
		"msg":    msg,
	})
}

// isTurnstileErr 判断是否为 chat-requirements turnstile 拦截（可切号重试）。
func isTurnstileErr(err error) bool {
	return err != nil && (errors.Is(err, webfile.ErrTurnstile) || strings.Contains(err.Error(), "turnstile"))
}

// isCloudflareErr 判断是否被 Cloudflare 边缘直接拦截（HTML 挑战页）。
// 该拦截与账号无关（绑 IP/TLS 指纹），换号无用，应停止重试。
func isCloudflareErr(err error) bool {
	return err != nil && (errors.Is(err, webfile.ErrCloudflare) || strings.Contains(err.Error(), "Cloudflare"))
}

// isAuthErr 判断是否为账号 token 不可用（401/未授权），可切号重试。
func isAuthErr(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "401") || strings.Contains(s, "未授权") || strings.Contains(s, "Unauthorized")
}

// PoolGenImage 号池生图单张结果。
type PoolGenImage struct {
	DataURL   string `json:"dataUrl"`
	LocalPath string `json:"localPath"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Mime      string `json:"mime"`
}

// PoolGenResult 号池生图结果。
type PoolGenResult struct {
	Account string         `json:"account"`
	Model   string         `json:"model"`
	Images  []PoolGenImage `json:"images"`
}

// PoolGenModel 号池生图可选模型。
type PoolGenModel struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// RegPoolGenModels 返回号池生图支持的模型（Codex 逆向画图）。
func (a *App) RegPoolGenModels() []PoolGenModel {
	return []PoolGenModel{
		{ID: "codex-gpt-image-2", Name: "Codex GPT-Image-2", Description: "Codex 逆向画图（responses 接口），Plus/Team/Pro 订阅额度；n 张为多次独立出图"},
		{ID: "gpt-image-2", Name: "网页 GPT-Image-2（套图）", Description: "ChatGPT 网页会话出图，一次返回多张风格统一的套图；耗时更长，用官网画图额度"},
	}
}

// RegPoolGenerate 用号池账号通过 Codex 逆向画图接口出图。
//
//   - model：codex-gpt-image-2（会映射回 gpt-image-2 作为工具模型）
//   - n：张数；refs：参考图（编辑模式，data URL 或 http URL）
//   - accountID：0 表示自动挑选可用号池账号
func (a *App) RegPoolGenerate(prompt, model, size, quality, outputFormat string, n int, refs []string, accountID uint64, taskID string) (*PoolGenResult, error) {
	if a.reg == nil {
		return nil, errRegUnavailable
	}
	if strings.TrimSpace(prompt) == "" {
		return nil, fmt.Errorf("提示词不能为空")
	}
	ctx := a.regCtx()
	proxyURL := a.reg.PoolProxyURL(ctx)
	lg := func(msg string) { a.poolLog(taskID, msg) }
	if n <= 0 {
		n = 1
	}

	dir := outputDir()
	_ = os.MkdirAll(dir, 0755)
	res := &PoolGenResult{Model: model, Images: make([]PoolGenImage, 0, n)}

	isWeb := strings.EqualFold(strings.TrimSpace(model), "gpt-image-2")
	// 模型别名：codex-gpt-image-2 → 工具模型 gpt-image-2
	toolModel := strings.TrimPrefix(strings.TrimSpace(model), "codex-")
	if toolModel == "" {
		toolModel = "gpt-image-2"
	}
	action := "generate"
	if len(refs) > 0 {
		action = "edit"
	}

	// 申请账号（一号一并发，随机挑号）→ 出图；失败换号重试，最多重试 2 次（共 3 个号）。
	var tried []uint64
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt == 0 {
			lg("申请号池账号（随机挑号，一号一并发）…")
		} else {
			lg(fmt.Sprintf("第 %d 次换号重试…", attempt))
		}
		tok, aerr := a.reg.PoolAcquireToken(ctx, accountID, tried...)
		if aerr != nil {
			lastErr = aerr
			lg("申请账号失败：" + aerr.Error())
			break
		}
		res.Account = tok.Email
		res.Images = res.Images[:0]
		lg("使用账号：" + tok.Email)

		genErr := func() error {
			if isWeb {
				imgs, werr := webfile.GenerateImages(ctx, webfile.ImageOptions{
					AccessToken: tok.AccessToken,
					AccountID:   tok.AccountID,
					ProxyURL:    proxyURL,
					Prompt:      prompt,
					N:           n,
					RefImages:   refs,
					Log:         lg,
				})
				if werr != nil {
					return werr
				}
				for i, im := range imgs {
					out := PoolGenImage{Mime: im.Mime}
					ext := extForMime(im.Mime)
					name := fmt.Sprintf("web-%s-%d%s", time.Now().Format("20060102-150405"), i+1, ext)
					p := filepath.Join(dir, name)
					if e := os.WriteFile(p, im.Data, 0644); e == nil {
						out.LocalPath = p
					}
					out.DataURL = "data:" + im.Mime + ";base64," + base64.StdEncoding.EncodeToString(im.Data)
					res.Images = append(res.Images, out)
					_ = saveHistoryItem(HistoryItem{Title: "号池生图·套图", Mode: "image", Prompt: prompt, Model: model, Size: size, ImageURL: out.DataURL, LocalPath: out.LocalPath})
				}
				if len(res.Images) == 0 {
					return fmt.Errorf("出图成功但未取得可用图片")
				}
				return nil
			}

			images, gerr := codeximg.Generate(ctx, codeximg.Options{
				AccessToken:  tok.AccessToken,
				AccountID:    tok.AccountID,
				ProxyURL:     proxyURL,
				Prompt:       prompt,
				ToolModel:    toolModel,
				Size:         strings.TrimSpace(size),
				Quality:      strings.ToLower(strings.TrimSpace(quality)),
				OutputFormat: strings.ToLower(strings.TrimSpace(outputFormat)),
				Action:       action,
				RefImages:    refs,
				N:            n,
				Log:          lg,
			})
			if gerr != nil {
				return gerr
			}
			for i, im := range images {
				out := PoolGenImage{Width: im.Width, Height: im.Height, Mime: im.Mime}
				if im.B64 != "" {
					if raw, derr := base64.StdEncoding.DecodeString(im.B64); derr == nil {
						ext := extForMime(im.Mime)
						name := fmt.Sprintf("codex-%s-%d%s", time.Now().Format("20060102-150405"), i+1, ext)
						p := filepath.Join(dir, name)
						if e := os.WriteFile(p, raw, 0644); e == nil {
							out.LocalPath = p
						}
					}
					mime := im.Mime
					if mime == "" {
						mime = "image/png"
					}
					out.DataURL = "data:" + mime + ";base64," + im.B64
				} else if im.URL != "" {
					out.DataURL = im.URL
					if p := a.SaveRemoteFile(im.URL, "codex-"+tok.Email); p != "" {
						out.LocalPath = p
					}
				}
				if out.DataURL == "" && out.LocalPath == "" {
					continue
				}
				res.Images = append(res.Images, out)
				_ = saveHistoryItem(HistoryItem{Title: "号池生图", Mode: "image", Prompt: prompt, Model: model, Quality: quality, Size: size, ImageURL: out.DataURL, LocalPath: out.LocalPath})
			}
			if len(res.Images) == 0 {
				return fmt.Errorf("出图成功但未取得可用图片")
			}
			return nil
		}()

		if genErr == nil {
			a.reg.PoolMarkUsed(ctx, tok.ID)
			a.reg.PoolReleaseAccount(tok.ID)
			lg(fmt.Sprintf("完成：成功 %d 张", len(res.Images)))
			return res, nil
		}
		a.reg.PoolReleaseAccount(tok.ID)
		lastErr = genErr
		lg("出错：" + genErr.Error())
		if isNetErr(genErr) {
			break // 网络/代理连不通，切号走同一代理也没用
		}
		if isCloudflareErr(genErr) {
			break // 被 Cloudflare 边缘拦截，绑 IP/指纹，换号无用
		}
		tried = append(tried, tok.ID)
		if accountID != 0 {
			break // 用户指定了账号，不切号
		}
	}
	fe := poolGenFriendlyErr(isWeb, lastErr)
	lg("最终失败：" + fe.Error())
	return nil, fe
}

// poolGenFriendlyErr 把底层报错转成更友好的提示。
func poolGenFriendlyErr(isWeb bool, err error) error {
	if err == nil {
		return fmt.Errorf("出图失败")
	}
	if isCloudflareErr(err) {
		return fmt.Errorf("网页套图链路被 Cloudflare 边缘拦截（返回挑战页 HTTP 403）。该拦截需要浏览器级验证（cf_clearance），当前 HTTP 方式无法通过，且与账号无关（换号无用）。请改用「Codex GPT-Image-2」链路出图，或更换更干净的代理 IP / 稍后重试")
	}
	if isTurnstileErr(err) {
		return fmt.Errorf("网页套图链路被 Cloudflare turnstile 拦截，已换号重试仍失败；建议改用 Codex GPT-Image-2，或换更干净的代理 IP / 稍后重试")
	}
	if isAuthErr(err) {
		return fmt.Errorf("账号 token 不可用（401/未授权），已换号重试仍失败；请确认号池里有可用的 Codex 类型账号")
	}
	if strings.Contains(err.Error(), "402") || strings.Contains(err.Error(), "订阅") {
		return fmt.Errorf("账号无可用订阅/额度（HTTP 402），已换号重试仍失败；网页套图/PPT/PSD 需 Plus/Team/Pro 订阅账号，Codex 链路也需订阅额度")
	}
	if isNetErr(err) {
		return fmt.Errorf("无法连接 chatgpt.com（TLS 握手/连接超时）：通常是没配代理、代理已失效、或代理无法访问 chatgpt.com。请到「系统设置 → 公共代理网关」检查并更换可用代理")
	}
	return err
}

// isNetErr 判断是否为网络/代理连不通（连 chatgpt.com 失败）。
func isNetErr(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "tls handshake") ||
		strings.Contains(s, "context deadline exceeded") ||
		strings.Contains(s, "Client.Timeout") ||
		strings.Contains(s, "connect proxy") ||
		strings.Contains(s, "i/o timeout") ||
		strings.Contains(s, "connection refused") ||
		strings.Contains(s, "no such host")
}

// PoolFileResult 号池生成可编辑文件（PPT/PSD）的结果。
type PoolFileResult struct {
	Account        string `json:"account"`
	Kind           string `json:"kind"`
	ConversationID string `json:"conversationId"`
	PrimaryPath    string `json:"primaryPath"`
	PrimaryName    string `json:"primaryName"`
	ZipPath        string `json:"zipPath"`
	ZipName        string `json:"zipName"`
}

// RegPoolGenerateFile 用号池账号通过 ChatGPT 网页会话生成可编辑 PPT / PSD 文件。
//
//   - kind：ppt / psd
//   - refs：参考图（PSD 必填；PPT 可选），data URL 或 http URL
//   - accountID：0 表示自动挑选可用号池账号
func (a *App) RegPoolGenerateFile(kind, prompt string, refs []string, accountID uint64, taskID string) (*PoolFileResult, error) {
	if a.reg == nil {
		return nil, errRegUnavailable
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind != "ppt" && kind != "psd" {
		return nil, fmt.Errorf("不支持的文件类型：%s（仅 ppt / psd）", kind)
	}
	ctx := a.regCtx()
	proxyURL := a.reg.PoolProxyURL(ctx)
	lg := func(msg string) { a.poolLog(taskID, msg) }

	dir := filepath.Join(outputDir(), kind)
	_ = os.MkdirAll(dir, 0755)

	// 申请账号（一号一并发，随机挑号）→ 生成；失败换号重试，最多重试 2 次（共 3 个号）。
	var tried []uint64
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt == 0 {
			lg("申请号池账号（随机挑号，一号一并发）…")
		} else {
			lg(fmt.Sprintf("第 %d 次换号重试…", attempt))
		}
		tok, aerr := a.reg.PoolAcquireToken(ctx, accountID, tried...)
		if aerr != nil {
			lastErr = aerr
			lg("申请账号失败：" + aerr.Error())
			break
		}
		lg("使用账号：" + tok.Email)
		res, err := webfile.Export(ctx, webfile.Options{
			AccessToken:  tok.AccessToken,
			AccountID:    tok.AccountID,
			ProxyURL:     proxyURL,
			Kind:         kind,
			Prompt:       prompt,
			Base64Images: refs,
			Log:          lg,
		}, dir)
		if err == nil {
			a.reg.PoolMarkUsed(ctx, tok.ID)
			a.reg.PoolReleaseAccount(tok.ID)
			lg("完成：" + res.Primary.Name)
			_ = saveHistoryItem(HistoryItem{
				Title:     "号池生图·" + strings.ToUpper(kind),
				Mode:      "image",
				Prompt:    prompt,
				Model:     "gpt-5-5-thinking",
				LocalPath: res.Primary.Path,
			})
			return &PoolFileResult{
				Account:        tok.Email,
				Kind:           kind,
				ConversationID: res.ConversationID,
				PrimaryPath:    res.Primary.Path,
				PrimaryName:    res.Primary.Name,
				ZipPath:        res.Zip.Path,
				ZipName:        res.Zip.Name,
			}, nil
		}
		a.reg.PoolReleaseAccount(tok.ID)
		lastErr = err
		lg("出错：" + err.Error())
		if isNetErr(err) {
			break
		}
		tried = append(tried, tok.ID)
		if accountID != 0 {
			break
		}
	}
	if isNetErr(lastErr) {
		return nil, fmt.Errorf("无法连接 chatgpt.com（TLS 握手/连接超时）：通常是没配代理、代理已失效、或代理无法访问 chatgpt.com。请到「系统设置 → 公共代理网关」检查并更换可用代理")
	}
	if isTurnstileErr(lastErr) {
		return nil, fmt.Errorf("可编辑 %s 生成被 Cloudflare turnstile 拦截，已换号重试仍失败；请换更干净的代理 IP 或稍后重试", strings.ToUpper(kind))
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("生成失败")
	}
	return nil, lastErr
}

func extForMime(mime string) string {
	switch strings.ToLower(mime) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	default:
		return ".png"
	}
}
