package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// BroadcastStatus tracks a run's lifecycle. A run left in "sending" with an
// old updated_at was interrupted (deploy, crash) rather than still working.
type BroadcastStatus string

const (
	BroadcastStatusSending   BroadcastStatus = "sending"
	BroadcastStatusCompleted BroadcastStatus = "completed"
	BroadcastStatusFailed    BroadcastStatus = "failed"
)

// Broadcast is one admin message pushed to every registered player.
type Broadcast struct {
	ID      uuid.UUID `json:"id"`
	Message string    `json:"message"`
	// Recipients is how many players the run targeted when it started.
	Recipients int             `json:"recipients"`
	Sent       int             `json:"sent"`
	Failed     int             `json:"failed"`
	Status     BroadcastStatus `json:"status"`
	CreatedBy  *uuid.UUID      `json:"created_by,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
	FinishedAt *time.Time      `json:"finished_at,omitempty"`
}

// SendBroadcastRequest is an admin composing a message.
type SendBroadcastRequest struct {
	Message string `json:"message" binding:"required"`
}

// BroadcastRecipient is one player to message.
type BroadcastRecipient struct {
	UserID     uuid.UUID
	TelegramID int64
}

// BroadcastRepository stores broadcast runs and finds who to send to.
type BroadcastRepository interface {
	// Create records a run in "sending" before any message goes out, so an
	// interrupted run is still visible rather than vanishing.
	Create(ctx context.Context, b *Broadcast) error
	// Recipients lists players who can actually receive a Telegram message:
	// real accounts (not filler bots) with a genuine positive Telegram id.
	Recipients(ctx context.Context) ([]BroadcastRecipient, error)
	// UpdateProgress writes the running totals mid-flight so the admin sees
	// movement and an interrupted run leaves an honest partial record.
	UpdateProgress(ctx context.Context, id uuid.UUID, sent, failed int) error
	// Finish marks the run done.
	Finish(ctx context.Context, id uuid.UUID, status BroadcastStatus, sent, failed int) error
	FindByID(ctx context.Context, id uuid.UUID) (*Broadcast, error)
	List(ctx context.Context, limit int) ([]*Broadcast, error)
}

// BroadcastSender is the slice of the bot API a broadcast needs: plain text to
// one chat. Deliberately narrower than the bot's own SendMessage (which also
// takes a reply markup) so `domain` does not have to import the telegram
// package — the concrete bot is adapted to this at wiring time — and so the
// send loop can be tested against a fake without a real bot token.
type BroadcastSender interface {
	SendMessage(chatID int64, text string) error
}

// BroadcastAction is an optional inline button attached to every message of a
// broadcast — a single in-chat callback the bot handles, e.g. a "Claim" button
// on a bonus announcement. Kept to a callback only: a broadcast reaches
// thousands of chats, and a callback carries no per-recipient data.
type BroadcastAction struct {
	Text         string
	CallbackData string
}

// ActionBroadcastSender is a BroadcastSender that can also attach a single
// inline callback button. The Telegram adapter implements it; the send loop
// uses it only when an action is set and falls back to plain SendMessage
// otherwise, so a sender without button support still works.
type ActionBroadcastSender interface {
	SendMessageWithAction(chatID int64, text string, action BroadcastAction) error
}
