package usecase

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bingo/backend/internal/domain"
	"github.com/google/uuid"
)

// fakeSender records what was sent and can be told to fail for specific chats,
// standing in for players who have blocked the bot.
type fakeSender struct {
	mu       sync.Mutex
	sentTo   []int64
	failFor  map[int64]bool
	failAll  bool
	sendCall int
}

func (f *fakeSender) SendMessage(chatID int64, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sendCall++
	if f.failAll || f.failFor[chatID] {
		return fmt.Errorf("forbidden: bot was blocked by the user")
	}
	f.sentTo = append(f.sentTo, chatID)
	return nil
}

func (f *fakeSender) delivered() []int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int64(nil), f.sentTo...)
}

type fakeBroadcastRepo struct {
	mu         sync.Mutex
	recipients []domain.BroadcastRecipient
	stored     *domain.Broadcast
	progress   int
}

func (r *fakeBroadcastRepo) Create(ctx context.Context, b *domain.Broadcast) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	copyB := *b
	r.stored = &copyB
	return nil
}
func (r *fakeBroadcastRepo) Recipients(ctx context.Context) ([]domain.BroadcastRecipient, error) {
	return r.recipients, nil
}
func (r *fakeBroadcastRepo) UpdateProgress(ctx context.Context, id uuid.UUID, sent, failed int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.progress++
	r.stored.Sent, r.stored.Failed = sent, failed
	return nil
}
func (r *fakeBroadcastRepo) Finish(ctx context.Context, id uuid.UUID, status domain.BroadcastStatus, sent, failed int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stored.Status, r.stored.Sent, r.stored.Failed = status, sent, failed
	return nil
}
func (r *fakeBroadcastRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.Broadcast, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stored, nil
}
func (r *fakeBroadcastRepo) List(ctx context.Context, limit int) ([]*domain.Broadcast, error) {
	return nil, nil
}

func (r *fakeBroadcastRepo) finished(t *testing.T) *domain.Broadcast {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		s := r.stored
		done := s != nil && s.Status != domain.BroadcastStatusSending
		snapshot := *s
		r.mu.Unlock()
		if done {
			return &snapshot
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("broadcast did not finish in time")
	return nil
}

func recipients(ids ...int64) []domain.BroadcastRecipient {
	out := make([]domain.BroadcastRecipient, 0, len(ids))
	for _, id := range ids {
		out = append(out, domain.BroadcastRecipient{UserID: uuid.New(), TelegramID: id})
	}
	return out
}

func TestBroadcastReachesEveryRecipient(t *testing.T) {
	repo := &fakeBroadcastRepo{recipients: recipients(101, 102, 103)}
	sender := &fakeSender{}
	uc := NewBroadcastUseCase(repo, sender)

	if _, err := uc.Send(context.Background(), "Bonus day! ", nil); err != nil {
		t.Fatalf("send: %v", err)
	}
	got := repo.finished(t)

	if got.Sent != 3 || got.Failed != 0 {
		t.Fatalf("sent=%d failed=%d, want 3/0", got.Sent, got.Failed)
	}
	if got.Status != domain.BroadcastStatusCompleted {
		t.Fatalf("status = %s, want completed", got.Status)
	}
	if len(sender.delivered()) != 3 {
		t.Fatalf("delivered to %v, want all three", sender.delivered())
	}
}

// The commonest real failure is a player who blocked the bot. That must not
// stop the message reaching everyone AFTER them in the list — the bug would be
// invisible to the admin, who would see a "completed" run that silently
// reached a fraction of the audience.
func TestBroadcastContinuesPastAFailedRecipient(t *testing.T) {
	repo := &fakeBroadcastRepo{recipients: recipients(201, 202, 203, 204)}
	sender := &fakeSender{failFor: map[int64]bool{202: true}}
	uc := NewBroadcastUseCase(repo, sender)

	if _, err := uc.Send(context.Background(), "hello", nil); err != nil {
		t.Fatalf("send: %v", err)
	}
	got := repo.finished(t)

	if got.Sent != 3 || got.Failed != 1 {
		t.Fatalf("sent=%d failed=%d, want 3/1", got.Sent, got.Failed)
	}
	// Specifically: the recipients after the failure still got it.
	delivered := sender.delivered()
	for _, want := range []int64{203, 204} {
		found := false
		for _, d := range delivered {
			if d == want {
				found = true
			}
		}
		if !found {
			t.Errorf("recipient %d after the failed one never received the message (%v)", want, delivered)
		}
	}
}

// A run where nothing got through is a failure, not a success with zero sent.
// Reporting "completed" there would hide a bad token or a dead network behind
// a green tick.
func TestBroadcastWithNoDeliveriesIsMarkedFailed(t *testing.T) {
	repo := &fakeBroadcastRepo{recipients: recipients(301, 302)}
	uc := NewBroadcastUseCase(repo, &fakeSender{failAll: true})

	if _, err := uc.Send(context.Background(), "hello", nil); err != nil {
		t.Fatalf("send: %v", err)
	}
	got := repo.finished(t)

	if got.Status != domain.BroadcastStatusFailed {
		t.Fatalf("status = %s, want failed when nothing was delivered", got.Status)
	}
	if got.Sent != 0 || got.Failed != 2 {
		t.Fatalf("sent=%d failed=%d, want 0/2", got.Sent, got.Failed)
	}
}

// Sending is paced so the bot token stays inside Telegram's bulk limit. Too
// fast and Telegram starts refusing, which would also disrupt the game's own
// outbound messages sharing the same token.
func TestBroadcastIsPaced(t *testing.T) {
	const n = 5
	ids := make([]int64, n)
	for i := range ids {
		ids[i] = int64(400 + i)
	}
	repo := &fakeBroadcastRepo{recipients: recipients(ids...)}
	uc := NewBroadcastUseCase(repo, &fakeSender{})

	start := time.Now()
	if _, err := uc.Send(context.Background(), "hello", nil); err != nil {
		t.Fatalf("send: %v", err)
	}
	repo.finished(t)
	elapsed := time.Since(start)

	// n-1 gaps between n messages.
	min := time.Duration(n-1) * broadcastSendInterval
	if elapsed < min {
		t.Fatalf("%d messages took %v, faster than the %v pacing floor — the rate limit would be breached",
			n, elapsed, min)
	}
}

// Validation happens before anything is queued, so a bad message never
// produces a half-sent run.
func TestBroadcastRejectsBadMessages(t *testing.T) {
	repo := &fakeBroadcastRepo{recipients: recipients(501)}
	uc := NewBroadcastUseCase(repo, &fakeSender{})

	if _, err := uc.Send(context.Background(), "   ", nil); err == nil {
		t.Error("blank message was accepted")
	}
	if _, err := uc.Send(context.Background(), strings.Repeat("x", broadcastMaxMessageLength+1), nil); err == nil {
		t.Error("over-length message was accepted")
	}
}

// With nobody to send to there is nothing to record; surfacing that as an
// error tells the admin their message went nowhere.
func TestBroadcastWithNoAudienceErrors(t *testing.T) {
	uc := NewBroadcastUseCase(&fakeBroadcastRepo{recipients: nil}, &fakeSender{})
	if _, err := uc.Send(context.Background(), "hello", nil); err == nil {
		t.Fatal("expected an error when there are no recipients")
	}
}
