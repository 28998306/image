# Web2Img AI Studio

> 企业级桌面 AI 生图 / 生视频工作台 —— 集**号池管理**、**自动注册**、**批量出图**于一体。

[![Go](https://img.shields.io/badge/Go-1.24-00ADD8?logo=go)](https://go.dev/)
[![Wails](https://img.shields.io/badge/Wails-v2.11-DF0000)](https://wails.io/)
[![React](https://img.shields.io/badge/React-18-61DAFB?logo=react)](https://react.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](./LICENSE)

Web2Img AI Studio 是一个基于 **Go + Wails + React** 的 Windows 桌面应用。它把「批量账号注册 → 账号池管理 → 调度出图/出视频」整条链路打包成一个开箱即用的本地工具，所有数据（账号、令牌、历史）均**保存在本地**并加密存储。

---

## ⚠️ 免责声明 / Disclaimer

**请在使用前务必阅读并同意以下条款：**

1. 本项目**仅供个人学习、研究与技术交流使用**，旨在探索桌面应用开发、网络协议、自动化与逆向工程相关技术。
2. 本项目涉及**第三方账号的自动注册**以及对相关平台 **Web / API 接口的逆向调用**。这些行为**可能违反相关服务提供商的服务条款（ToS）**。是否使用、如何使用由使用者自行决定，并**自行承担全部法律与账号风险**（包括但不限于账号封禁、服务中断、数据损失等）。
3. 使用者必须遵守所在国家或地区的**法律法规**，不得将本项目用于任何**非法、商业牟利、批量薅羊毛、侵犯他人权益或破坏平台正常运营**的用途。
4. 本项目**不提供、不附带**任何账号、令牌、代理、邮箱、API Key 等资源；不存储、不上传任何用户数据到作者服务器。所有运行数据仅保存在使用者本地设备。
5. 作者与贡献者对因使用本项目而导致的**任何直接或间接损失不承担任何责任**。下载、克隆或使用本项目即视为您**已完全理解并同意**本免责声明。

> 如果你不同意以上任意一条，请**立即停止使用并删除本项目**。

---

## ✨ 功能特性

### 号池工作流（基于自有账号池）
- **📬 邮箱管理**：导入/编辑邮箱池，支持 **Outlook Graph / Outlook IMAP / TempMail / Cloudflare Worker** 多种后端，自动收取 OpenAI 验证码。
- **🤖 自动注册**：批量自动注册 ChatGPT 账号（1–5000 个任务），自动取邮箱、自动生成用户名/密码、自动收码验证、自动落库（**当前主链路无需验证码**）。支持并发与失败重试。
- **🗂 号池管理**：账号增删改查、批量导入/导出（`email----password----access----refresh`）、额度查询、令牌刷新、状态管理（有效/失效/停用/冷却中）。
- **🎨 号池生图**：用池内订阅账号出图，支持
  - **Codex GPT-Image-2** 链路（`gpt-image-2`，稳定）
  - **网页 GPT-Image-2（套图）** 链路（一次会话产出多张套图）
  - **可编辑 PPT**（导出 `.pptx` + 素材 `.zip`）
  - **可编辑 PSD**（导出 `.psd` + 图层 `.zip`）
  - 多任务队列、参考图、失败自动换号、GPT Image 2 完整尺寸表（1K/2K/4K，最长边 ≤ 3840）。

### API Studio 工作流（基于 gpt2api）
- **文生图 / 图生图 / 视频生成**，支持 `nano-banana-pro` / `nano-banana-v2` / `nano-banana` / `gpt-image-2` 等图像模型与 `sora` / `veo3.1` / `grok-imagine-video` 等视频模型。
- 异步轮询、画廊预览、并发任务队列。

### 其它
- 📜 **历史记录**：本地分页保存所有出图/出视频结果。
- ⚙️ **系统设置**：接口连接、图片/视频默认参数、公共代理网关、输出目录等。
- 🔒 账号密码与令牌使用 **AES-256 加密**存储于本地数据库。

---

## 🧱 技术栈

| 层 | 技术 |
|----|------|
| 后端 | Go 1.24、[Wails v2.11](https://wails.io/) |
| 前端 | React 18 + TypeScript + Vite + Tailwind CSS |
| 存储 | SQLite（GORM）、本地 JSON、AES-256 加密 |
| 网络 | uTLS（浏览器指纹）、go-imap、HTTP 代理 |

---

## 🚀 快速开始

### 环境要求
- [Go 1.24+](https://go.dev/dl/)
- [Wails CLI v2](https://wails.io/docs/gettingstarted/installation)（`go install github.com/wailsapp/wails/v2/cmd/wails@latest`）
- [Node.js](https://nodejs.org/) 16+ 与 npm
- Windows + WebView2 运行时

### 开发模式
```bash
wails dev
```

### 打包构建
```bash
wails build
# 产物：build/bin/web2img.exe
```

> 也可仅构建前端：`cd frontend && npm install && npm run build`

---

## 🔧 使用配置

首次运行后，程序会在以下位置创建本地数据（**均不在仓库内，切勿提交**）：

| 路径 | 用途 |
|------|------|
| `%AppData%\Web2Img AI Studio\config.json` | gpt2api Key、默认参数、输出目录 |
| `%AppData%\Web2Img AI Studio\reg\reg.db` | 邮箱池、号池、任务、`system_config` |
| `%AppData%\Web2Img AI Studio\reg\reg.key` | 数据库字段 AES 加密密钥 |
| `%AppData%\Web2Img AI Studio\history.json` | 生成历史 |

需自行准备/配置：
1. **动态代理网关**（强烈建议，用于注册与号池出图）：`系统设置 → 公共代理网关`，格式 `http://user:pass@host:port`。
2. **gpt2api API Key**（用于 API Studio）：`系统设置 → 接口连接`。
3. **邮箱后端**（用于自动注册）：在邮箱管理导入邮箱，或在 `reg.db` 的 `system_config` 表配置 Outlook / TempMail / Cloudflare。
4. **短信/验证码服务**（可选）：仅在触发手机验证时需要。

---

## 🔐 开源安全提醒（重要）

发布到公共仓库前，请确认**绝不要提交以下内容**：
- `config.json`（含 gpt2api Key）
- `reg.db`（含加密的邮箱密码、刷新令牌、账号令牌）
- `reg.key`（解密密钥）
- 任何代理凭据、邮箱/账号导出文件、`.env`

上述文件已在 [`.gitignore`](./.gitignore) 中排除。建议发布前执行 `git status` 再次核对。

---

## 📄 许可证

本项目采用 [MIT License](./LICENSE) 开源。

---

## 💬 交流

- GitHub: <https://github.com/28998306/image>

> 再次提醒：请合法、合规、负责任地使用本项目。
