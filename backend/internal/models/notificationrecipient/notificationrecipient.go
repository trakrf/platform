// Package notificationrecipient holds the contacts that can receive asset
// notifications and the subscriptions that link them to assets (TRA-1275).
package notificationrecipient

import "time"

const (
	ChannelEmail = "email"
	ChannelSMS   = "sms"
)

type Recipient struct {
	ID        int        `json:"id"`
	OrgID     int        `json:"org_id"`
	Name      string     `json:"name"`
	Email     *string    `json:"email,omitempty"`
	Phone     *string    `json:"phone,omitempty"`
	IsActive  bool       `json:"is_active"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

type CreateRecipientRequest struct {
	Name     string  `json:"name" validate:"required,max=255"`
	Email    *string `json:"email,omitempty" validate:"omitempty,email,max=320"`
	Phone    *string `json:"phone,omitempty" validate:"omitempty,max=32"`
	IsActive *bool   `json:"is_active,omitempty"`
}

// UpdateRecipientRequest omits clearing: a set field replaces the stored value.
// Clearing the last contact would break the recipient's contact invariant.
type UpdateRecipientRequest struct {
	Name     *string `json:"name,omitempty" validate:"omitempty,min=1,max=255"`
	Email    *string `json:"email,omitempty" validate:"omitempty,email,max=320"`
	Phone    *string `json:"phone,omitempty" validate:"omitempty,min=1,max=32"`
	IsActive *bool   `json:"is_active,omitempty"`
}

type Subscription struct {
	ID          int       `json:"id"`
	OrgID       int       `json:"org_id"`
	AssetID     int       `json:"asset_id"`
	RecipientID int       `json:"recipient_id"`
	Channel     string    `json:"channel"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CreateSubscriptionRequest struct {
	RecipientID int     `json:"recipient_id" validate:"required,gt=0"`
	Channel     *string `json:"channel,omitempty" validate:"omitempty,oneof=email sms"`
}

// UpdateSubscriptionRequest changes a subscription in place. Subscriptions are
// never deleted: set is_active false to switch one off.
type UpdateSubscriptionRequest struct {
	Channel  *string `json:"channel,omitempty" validate:"omitempty,oneof=email sms"`
	IsActive *bool   `json:"is_active,omitempty"`
}
