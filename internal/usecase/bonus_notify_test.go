package usecase

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bingo/backend/internal/domain"
	"github.com/google/uuid"
)

// recordingNotifier captures what a player would be told.
type recordingNotifier struct {
	mu     sync.Mutex
	chatID int64
	text   string
	calls  int
	fail   bool
}

func (n *recordingNotifier) SendMessage(chatID int64, text string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.calls++
	if n.fail {
		return fmt.Errorf("telegram unavailable")
	}
	n.chatID, n.text = chatID, text
	return nil
}

func TestBonusGrantNotificationContent(t *testing.T) {
	n := &recordingNotifier{}
	uc := &BonusUseCase{notifier: n}
	user := &domain.User{ID: uuid.New(), TelegramID: 777001}
	grant := &domain.BonusGrant{
		Amount:    50,
		ExpiresAt: time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
	}

	uc.notifyGrant(user, grant)

	if n.chatID != 777001 {
		t.Fatalf("sent to chat %d, want 777001", n.chatID)
	}
	// The player has to learn three things: how much, until when, and that it
	// cannot be cashed out. Omitting the last would be misleading, since the
	// figure otherwise reads like ordinary money.
	for _, want := range []string{"50", "Jul 25", "cannot be withdrawn"} {
		if !strings.Contains(n.text, want) {
			t.Errorf("notification is missing %q:\n%s", want, n.text)
		}
	}
	// Bilingual, matching the bot's existing voice.
	if !strings.Contains(n.text, "ብር") {
		t.Error("notification has no Amharic; the bot speaks to players bilingually")
	}
}

// The money is committed before the notice goes out. A Telegram failure must
// not surface as a grant failure: an admin who retried on that error would
// award the bonus twice.
func TestBonusGrantNotificationFailureIsNotFatal(t *testing.T) {
	n := &recordingNotifier{fail: true}
	uc := &BonusUseCase{notifier: n}

	// notifyGrant returns nothing and must not panic on a failing sender.
	uc.notifyGrant(&domain.User{ID: uuid.New(), TelegramID: 777002},
		&domain.BonusGrant{Amount: 25, ExpiresAt: time.Now().Add(72 * time.Hour)})

	if n.calls != 1 {
		t.Fatalf("notifier called %d times, want 1", n.calls)
	}
}

// Filler bots carry negative synthetic Telegram ids; those chats do not exist,
// so messaging them is a guaranteed error.
func TestBonusGrantSkipsNonTelegramAccounts(t *testing.T) {
	n := &recordingNotifier{}
	uc := &BonusUseCase{notifier: n}

	uc.notifyGrant(&domain.User{ID: uuid.New(), TelegramID: -1000000042},
		&domain.BonusGrant{Amount: 10, ExpiresAt: time.Now().Add(time.Hour)})
	uc.notifyGrant(&domain.User{ID: uuid.New(), TelegramID: 0},
		&domain.BonusGrant{Amount: 10, ExpiresAt: time.Now().Add(time.Hour)})

	if n.calls != 0 {
		t.Fatalf("notifier called %d times for accounts with no real Telegram chat", n.calls)
	}
}

// A nil notifier is a supported configuration (no bot token); granting must
// still work, silently.
func TestBonusGrantWithoutNotifierDoesNotPanic(t *testing.T) {
	uc := &BonusUseCase{notifier: nil}
	uc.notifyGrant(&domain.User{ID: uuid.New(), TelegramID: 777003},
		&domain.BonusGrant{Amount: 10, ExpiresAt: time.Now().Add(time.Hour)})
}
