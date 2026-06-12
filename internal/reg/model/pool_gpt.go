package model

import "time"

// PoolGpt 状态。
const (
	GPTStatusValid    = "valid"
	GPTStatusInvalid  = "invalid"
	GPTStatusDisabled = "disabled"
	GPTStatusCooldown = "cooldown"
)

// PoolGpt GPT 号池实体。表 `pool_gpt`。
type PoolGpt struct {
	ID              uint64  `gorm:"primaryKey;column:id" json:"id"`
	Email           string  `gorm:"column:email;size:255;not null" json:"email"`
	PasswordEnc     []byte  `gorm:"column:password_enc;type:blob" json:"-"`
	AccessTokenEnc  []byte  `gorm:"column:access_token_enc;type:blob" json:"-"`
	RefreshTokenEnc []byte  `gorm:"column:refresh_token_enc;type:blob" json:"-"`
	IDTokenEnc      []byte  `gorm:"column:id_token_enc;type:blob" json:"-"`
	APIKeyEnc       []byte  `gorm:"column:api_key_enc;type:blob" json:"-"`
	OAuthIssuer     *string `gorm:"column:oauth_issuer;size:255" json:"oauth_issuer,omitempty"`
	OAuthClientID   *string `gorm:"column:oauth_client_id;size:128" json:"oauth_client_id,omitempty"`

	PlanType         *string `gorm:"column:plan_type;size:32" json:"plan_type,omitempty"`
	ChatGPTAccountID *string `gorm:"column:chatgpt_account_id;size:64" json:"chatgpt_account_id,omitempty"`

	ProxyID       *uint64    `gorm:"column:proxy_id" json:"proxy_id,omitempty"`
	Weight        int        `gorm:"column:weight;not null;default:10" json:"weight"`
	SuccessCount  uint64     `gorm:"column:success_count;not null;default:0" json:"success_count"`
	Status        string     `gorm:"column:status;size:32;not null;default:valid" json:"status"`
	ExpiresAt     *time.Time `gorm:"column:expires_at" json:"expires_at,omitempty"`
	LastRefreshAt *time.Time `gorm:"column:last_refresh_at" json:"last_refresh_at,omitempty"`
	LastUsedAt    *time.Time `gorm:"column:last_used_at" json:"last_used_at,omitempty"`

	// 额度探测结果（来自 chatgpt.com/backend-api/conversation/init 的 limits_progress）。
	ImageQuotaRemaining *int       `gorm:"column:image_quota_remaining" json:"image_quota_remaining,omitempty"`
	ImageQuotaTotal     *int       `gorm:"column:image_quota_total" json:"image_quota_total,omitempty"`
	ImageQuotaResetAt   *time.Time `gorm:"column:image_quota_reset_at" json:"image_quota_reset_at,omitempty"`
	LastQuotaCheckAt    *time.Time `gorm:"column:last_quota_check_at" json:"last_quota_check_at,omitempty"`

	FailureCount  int        `gorm:"column:failure_count;not null;default:0" json:"failure_count"`
	ErrorMessage  *string    `gorm:"column:error_message;size:500" json:"error_message,omitempty"`
	Notes         *string    `gorm:"column:notes;size:500" json:"notes,omitempty"`
	RegisteredAt  time.Time  `gorm:"column:registered_at;autoCreateTime" json:"registered_at"`
	CreatedAt     time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
	DeletedAt     *time.Time `gorm:"column:deleted_at;index" json:"-"`
}

// TableName 表名。
func (PoolGpt) TableName() string { return "pool_gpt" }
