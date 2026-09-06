package usecase

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/bingo/backend/internal/domain"
	"github.com/bingo/backend/pkg/utils"
	"github.com/google/uuid"
)

const (
	// broadcastMaxMessageLength is Telegram's own limit for a text message.
	// Rejecting up front beats discovering it on recipient one of two thousand.
	broadcastMaxMessageLength = 4000

	// broadcastSendInterval paces sending. Telegram allows roughly 30 messages
	// a second to different users before it starts replying 429; sitting at
	// ~20/sec leaves headroom for the game's own outbound traffic (join
	// notifications, promo replies) which shares the same bot token and the
	// same budget.
	broadcastSendInterval = 50 * time.Millisecond

	// broadcastProgressEvery controls how often progress is persisted. Writing
	// after every message would triple the database traffic of a broadcast for
	// no benefit; this keeps the admin's progress bar honest to within a
	// second of real time.
	broadcastProgressEvery = 25
)

// BroadcastUseCase pushes an admin message to every registered player over
// Telegram.
type BroadcastUseCase struct {
	repo   domain.BroadcastRepository
	sender domain.BroadcastSender
}

func NewBroadcastUseCase(repo domain.BroadcastRepository, sender domain.BroadcastSender) *BroadcastUseCase {
	return &BroadcastUseCase{repo: repo, sender: sender}
}

// Send records the run, then delivers it in the BACKGROUND and returns
// immediately.
//
// It has to be asynchronous: at ~20 messages a second a few thousand players
// take minutes, far past any sensible HTTP timeout. The admin would be left
// with a dead request and no way to tell whether re-submitting would
// double-send. Instead the row exists before the first message goes out, and
// the caller polls it.
//
// The background context is deliberately NOT the request's — that one is
// cancelled the moment the HTTP handler returns, which would kill the
// broadcast after a single message.
func (uc *BroadcastUseCase) Send(ctx context.Context, message string, createdBy *uuid.UUID) (*domain.Broadcast, error) {
	return uc.send(ctx, message, createdBy, nil)
}

// SendWithAction is Send with a single inline button on every message — used to
// put a "Claim" button on a bonus announcement so players claim straight from
// the notification. Falls back to a plain message if the sender can't attach
// buttons.
func (uc *BroadcastUseCase) SendWithAction(ctx context.Context, message string, createdBy *uuid.UUID, action *domain.BroadcastAction) (*domain.Broadcast, error) {
	return uc.send(ctx, message, createdBy, action)
}

func (uc *BroadcastUseCase) send(ctx context.Context, message string, createdBy *uuid.UUID, action *domain.BroadcastAction) (*domain.Broadcast, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return nil, fmt.Errorf("message cannot be empty")
	}
	if len([]rune(message)) > broadcastMaxMessageLength {
		return nil, fmt.Errorf("message cannot exceed %d characters", broadcastMaxMessageLength)
	}
	if uc.sender == nil {
		return nil, fmt.Errorf("telegram bot is not configured")
	}

	recipients, err := uc.repo.Recipients(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list recipients: %w", err)
	}
	if len(recipients) == 0 {
		return nil, fmt.Errorf("no registered players to send to")
	}

	b := &domain.Broadcast{
		ID:         uuid.New(),
		Message:    message,
		Recipients: len(recipients),
		Status:     domain.BroadcastStatusSending,
		CreatedBy:  createdBy,
	}
	if err := uc.repo.Create(ctx, b); err != nil {
		return nil, err
	}

	go uc.deliver(b.ID, message, recipients, action)
	return b, nil
}

// sendOne delivers a single message, attaching the action button when one is
// set and the sender supports it, else sending plain text.
func (uc *BroadcastUseCase) sendOne(chatID int64, message string, action *domain.BroadcastAction) error {
	if action != nil {
		if rich, ok := uc.sender.(domain.ActionBroadcastSender); ok {
			return rich.SendMessageWithAction(chatID, message, *action)
		}
	}
	return uc.sender.SendMessage(chatID, message)
}

// deliver walks the recipient list at a fixed pace.
//
// A failed send is recorded and skipped, never fatal: the commonest failure by
// far is a player who blocked the bot, and one blocked player must not stop
// the message reaching everyone after them in the list.
func (uc *BroadcastUseCase) deliver(id uuid.UUID, message string, recipients []domain.BroadcastRecipient, action *domain.BroadcastAction) {
	defer utils.RecoverPanic("broadcast.deliver")
	// Own context with a generous ceiling, independent of the request that
	// started this. The timeout is a backstop against a wedged run holding a
	// row in "sending" forever, not an expected path.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()

	ticker := time.NewTicker(broadcastSendInterval)
	defer ticker.Stop()

	sent, failed := 0, 0
	for i, rec := range recipients {
		if i > 0 {
			select {
			case <-ticker.C:
			case <-ctx.Done():
				log.Printf("[broadcast %s] aborted after %d sent, %d failed: %v", id, sent, failed, ctx.Err())
				_ = uc.repo.Finish(ctx, id, domain.BroadcastStatusFailed, sent, failed)
				return
			}
		}

		if err := uc.sendOne(rec.TelegramID, message, action); err != nil {
			failed++
			// Logged at low volume rather than per-message-body: a player who
			// blocked the bot is routine, not an incident.
			if failed <= 5 {
				log.Printf("[broadcast %s] send to %d failed: %v", id, rec.TelegramID, err)
			}
		} else {
			sent++
		}

		if (i+1)%broadcastProgressEvery == 0 {
			if err := uc.repo.UpdateProgress(ctx, id, sent, failed); err != nil {
				log.Printf("[broadcast %s] progress update failed: %v", id, err)
			}
		}
	}

	// A run where nothing at all got through is reported as failed — most
	// likely a bad token or a network problem, which the admin needs to see
	// rather than reading "completed, 0 sent" as success.
	status := domain.BroadcastStatusCompleted
	if sent == 0 {
		status = domain.BroadcastStatusFailed
	}
	if err := uc.repo.Finish(ctx, id, status, sent, failed); err != nil {
		log.Printf("[broadcast %s] finish failed: %v", id, err)
	}
	log.Printf("[broadcast %s] done: %d sent, %d failed of %d", id, sent, failed, len(recipients))
}

func (uc *BroadcastUseCase) Get(ctx context.Context, id uuid.UUID) (*domain.Broadcast, error) {
	return uc.repo.FindByID(ctx, id)
}

func (uc *BroadcastUseCase) List(ctx context.Context, limit int) ([]*domain.Broadcast, error) {
	return uc.repo.List(ctx, limit)
}

// RecipientCount lets the dashboard show "this will reach N players" before
// the admin commits to sending.
func (uc *BroadcastUseCase) RecipientCount(ctx context.Context) (int, error) {
	recipients, err := uc.repo.Recipients(ctx)
	if err != nil {
		return 0, err
	}
	return len(recipients), nil
}
