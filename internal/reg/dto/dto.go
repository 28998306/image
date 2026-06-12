// Package dto 注册子系统的数据传输对象。
package dto

// GptPoolCreateReq 新建 GPT 号池行（dispatcher 注册成功后调用）。
type GptPoolCreateReq struct {
	Email            string
	Password         string
	AccessToken      string
	RefreshToken     string
	IDToken          string
	APIKey           string
	OAuthIssuer      string
	OAuthClientID    string
	PlanType         string
	ChatGPTAccountID string
	Status           string
	Notes            string
	ExpiresAt        int64 // unix 毫秒
}

// RegisterTaskListReq 注册任务列表请求。
type RegisterTaskListReq struct {
	Provider string
	Status   string
	Keyword  string
	Page     int
	PageSize int
}

// RegisterTaskCreateReq 创建注册任务请求。
type RegisterTaskCreateReq struct {
	Provider string
	MailID   *uint64
	Count    int
	Payload  map[string]any
}

// RegisterTaskCreateResp 创建结果。
type RegisterTaskCreateResp struct {
	Created int      `json:"created"`
	IDs     []uint64 `json:"ids"`
}

// RegisterTaskResp 任务详情。
type RegisterTaskResp struct {
	ID              uint64         `json:"id"`
	Provider        string         `json:"provider"`
	Status          string         `json:"status"`
	Step            string         `json:"step,omitempty"`
	Progress        uint8          `json:"progress"`
	MailID          uint64         `json:"mail_id,omitempty"`
	Email           string         `json:"email,omitempty"`
	Payload         map[string]any `json:"payload,omitempty"`
	Result          map[string]any `json:"result,omitempty"`
	Error           string         `json:"error,omitempty"`
	PoolAccountID   uint64         `json:"pool_account_id,omitempty"`
	CancelRequested bool           `json:"cancel_requested"`
	CreatedAt       int64          `json:"created_at"`
	StartedAt       int64          `json:"started_at,omitempty"`
	FinishedAt      int64          `json:"finished_at,omitempty"`
	UpdatedAt       int64          `json:"updated_at"`
}

// RegisterTaskStatsResp 状态分布。
type RegisterTaskStatsResp struct {
	Total     int64 `json:"total"`
	Pending   int64 `json:"pending"`
	Running   int64 `json:"running"`
	Success   int64 `json:"success"`
	Failed    int64 `json:"failed"`
	Cancelled int64 `json:"cancelled"`
}

// RegisterTaskLogResp 日志条目。
type RegisterTaskLogResp struct {
	ID        uint64 `json:"id"`
	TaskID    uint64 `json:"task_id"`
	Provider  string `json:"provider,omitempty"`
	Level     string `json:"level"`
	Step      string `json:"step,omitempty"`
	Progress  uint8  `json:"progress,omitempty"`
	Message   string `json:"message,omitempty"`
	CreatedAt int64  `json:"created_at"`
}

// MailPoolImportResult 邮箱导入结果。
type MailPoolImportResult struct {
	Imported int      `json:"imported"`
	Skipped  int      `json:"skipped"`
	Errors   []string `json:"errors,omitempty"`
}

// MailPoolResp 邮箱行。
type MailPoolResp struct {
	ID              uint64 `json:"id"`
	Email           string `json:"email"`
	ClientID        string `json:"client_id"`
	Mode            string `json:"mode"`
	Status          string `json:"status"`
	FailureCount    int    `json:"failure_count"`
	LastError       string `json:"last_error,omitempty"`
	UsedByProvider  string `json:"used_by_provider,omitempty"`
	UsedByAccountID uint64 `json:"used_by_account_id,omitempty"`
	ImportedAt      int64  `json:"imported_at"`
	UsedAt          int64  `json:"used_at,omitempty"`
	RegisteredAt    int64  `json:"registered_at,omitempty"`
}

// MailPoolStatsResp 邮箱池统计。
type MailPoolStatsResp struct {
	Total      int64 `json:"total"`
	Available  int64 `json:"available"`
	InUse      int64 `json:"in_use"`
	Registered int64 `json:"registered"`
	Failed     int64 `json:"failed"`
	Disabled   int64 `json:"disabled"`
}

// GptPoolResp GPT 号池行。
type GptPoolResp struct {
	ID               uint64 `json:"id"`
	Email            string `json:"email"`
	Status           string `json:"status"`
	PlanType         string `json:"plan_type,omitempty"`
	ChatGPTAccountID string `json:"chatgpt_account_id,omitempty"`
	SuccessCount     uint64 `json:"success_count"`
	FailureCount     int    `json:"failure_count"`
	Notes            string `json:"notes,omitempty"`
	HasAccessToken   bool   `json:"has_access_token"`
	HasRefreshToken  bool   `json:"has_refresh_token"`
	RegisteredAt     int64  `json:"registered_at"`
	ExpiresAt        int64  `json:"expires_at,omitempty"`
	LastRefreshAt    int64  `json:"last_refresh_at,omitempty"`

	ImageQuotaRemaining *int  `json:"image_quota_remaining,omitempty"`
	ImageQuotaTotal     *int  `json:"image_quota_total,omitempty"`
	ImageQuotaResetAt   int64 `json:"image_quota_reset_at,omitempty"`
	LastQuotaCheckAt    int64 `json:"last_quota_check_at,omitempty"`
}

// GptPoolUpdateReq 编辑号池行（空字符串=不改；token 留空=保留旧值）。
type GptPoolUpdateReq struct {
	Status       string
	Notes        string
	Password     string
	AccessToken  string
	RefreshToken string
	IDToken      string
	APIKey       string
	ExpiresAt    int64
}

// GptPoolImportResult 号池导入结果。
type GptPoolImportResult struct {
	Imported int      `json:"imported"`
	Updated  int      `json:"updated"`
	Skipped  int      `json:"skipped"`
	Errors   []string `json:"errors,omitempty"`
}

// GptRefreshResp 单账号刷新结果。
type GptRefreshResp struct {
	OK          bool   `json:"ok"`
	ExpiresAt   int64  `json:"expires_at,omitempty"`
	RefreshedAt int64  `json:"refreshed_at,omitempty"`
	Message     string `json:"message,omitempty"`
}

// GptPoolDetailResp 号池行明文凭证（编辑弹窗用）。
type GptPoolDetailResp struct {
	ID           uint64 `json:"id"`
	Email        string `json:"email"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
}

// GptBatchRefreshResp 批量刷新结果。
type GptBatchRefreshResp struct {
	Refreshed int      `json:"refreshed"`
	Failed    int      `json:"failed"`
	Errors    []string `json:"errors,omitempty"`
}

// GptQuotaResp 单账号额度查询结果。
//
// ImageQuota* 复用为主窗口（5 小时）剩余百分比：Remaining=剩余%，Total=100。
// Weekly* 为次窗口（每周）剩余百分比与重置时间。
type GptQuotaResp struct {
	OK                  bool   `json:"ok"`
	PlanType            string `json:"plan_type,omitempty"`
	ImageQuotaRemaining *int   `json:"image_quota_remaining,omitempty"`
	ImageQuotaTotal     *int   `json:"image_quota_total,omitempty"`
	ImageQuotaResetAt   int64  `json:"image_quota_reset_at,omitempty"`
	WeeklyRemaining     *int   `json:"weekly_remaining,omitempty"`
	WeeklyResetAt       int64  `json:"weekly_reset_at,omitempty"`
	CreditsBalance      string `json:"credits_balance,omitempty"`
	DefaultModel        string `json:"default_model,omitempty"`
	CheckedAt           int64  `json:"checked_at,omitempty"`
	Message             string `json:"message,omitempty"`
}

// GptPoolStatsResp GPT 号池统计。
type GptPoolStatsResp struct {
	Total    int64 `json:"total"`
	Valid    int64 `json:"valid"`
	Invalid  int64 `json:"invalid"`
	Disabled int64 `json:"disabled"`
	Cooldown int64 `json:"cooldown"`
}
