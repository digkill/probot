package domain

import (
	"github.com/google/uuid"
	"time"
)

type TelegramAccount struct {
	ID              uuid.UUID  `json:"id"`
	WorkspaceID     uuid.UUID  `json:"workspace_id"`
	Name            string     `json:"name"`
	Enabled         bool       `json:"enabled"`
	UserID          int64      `json:"telegram_user_id"`
	Phone           string     `json:"phone,omitempty"`
	Username        string     `json:"username,omitempty"`
	FirstName       string     `json:"first_name,omitempty"`
	LastName        string     `json:"last_name,omitempty"`
	Status          string     `json:"status"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	LastConnectedAt *time.Time `json:"last_connected_at,omitempty"`
}

const (
	TelegramStopped      = "stopped"
	TelegramConnecting   = "connecting"
	TelegramAuthRequired = "auth_required"
	TelegramConnected    = "connected"
	TelegramFloodWait    = "flood_wait"
	TelegramFailed       = "failed"
)

type TelegramMessage struct {
	ID        int       `json:"id"`
	AccountID uuid.UUID `json:"account_id"`
	ChatID    int64     `json:"chat_id"`
	SenderID  int64     `json:"sender_id"`
	Text      string    `json:"text"`
	Date      time.Time `json:"date"`
	Outgoing  bool      `json:"outgoing"`
	Service   bool      `json:"service,omitempty"`
	HasMedia  bool      `json:"has_media"`
}

type TelegramPeer struct {
	ID       int64  `json:"id"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	Username string `json:"username,omitempty"`
}

type TelegramDialog struct {
	TelegramPeer
	UnreadCount int `json:"unread_count"`
}

type TelegramDialogs struct {
	Items      []TelegramDialog `json:"items"`
	NextCursor string           `json:"next_cursor,omitempty"`
}

type TelegramSendRequest struct {
	Peer string `json:"peer"`
	Text string `json:"text"`
}

type TelegramLogin struct {
	ID             string    `json:"login_id,omitempty"`
	State          string    `json:"state"`
	URL            string    `json:"url,omitempty"`
	ExpiresAt      time.Time `json:"expires_at,omitempty"`
	TokenExpiresAt time.Time `json:"token_expires_at,omitempty"`
	Error          string    `json:"error,omitempty"`
}

// Events contain application DTOs only. Consumers use ID as a durable cursor.
type TelegramEvent struct {
	ID        int64           `json:"id"`
	AccountID uuid.UUID       `json:"account_id"`
	Type      string          `json:"type"`
	Message   TelegramMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

const (
	TelegramMessageReceived = "telegram.message.received"
	TelegramMessageSent     = "telegram.message.sent"
)
