package usecase

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/bingo/backend/internal/domain"
	"github.com/google/uuid"
)

// BonusUseCase manages play-only bonus money: the free-bet model, where a
// bonus can buy game cards but can never be withdrawn, and any winnings from a
// bonus-funded card are ordinary cash.
//
// Deliberately NOT mirrored into the `transactions` table. That table is the
// CASH ledger, and its player-facing history only hides rows whose reference
// starts with "GAME_", so a bonus grant recorded there would surface in the
// player's deposit history looking like money they could withdraw. The
// bonus_grants table is the bonus ledger; the two are kept apart on purpose.
type BonusUseCase struct {
	bonusRepo domain.BonusRepository
	userRepo  domain.UserRepository
	db        *sql.DB
	// notifier tells the player they received something. Optional: without it
	// bonus still works, players just have to notice it in the app.
	notifier domain.BroadcastSender
}

func NewBonusUseCase(bonusRepo domain.BonusRepository, userRepo domain.UserRepository, db *sql.DB, notifier domain.BroadcastSender) *BonusUseCase {
	return &BonusUseCase{bonusRepo: bonusRepo, userRepo: userRepo, db: db, notifier: notifier}
}

// notifyGrant tells the player on Telegram that they have bonus waiting.
//
// Without this a grant is invisible: nothing pushes it, so a player only finds
// out if they happen to open the app and look — and bonus expires, so one they
// never noticed is one they silently lose.
//
// Deliberately AFTER the commit and never fatal. The money is already theirs;
// a Telegram hiccup must not roll that back, and re-granting to "fix" a failed
// message would double the award. Bilingual to match the bot's existing voice.
func (uc *BonusUseCase) notifyGrant(user *domain.User, grant *domain.BonusGrant) {
	if uc.notifier == nil || user.TelegramID <= 0 {
		return
	}
	expiry := grant.ExpiresAt.Format("Jan 2")
	msg := fmt.Sprintf(
		"🎁 %.0f ብር የጨዋታ ቦነስ አግኝተዋል!\n"+
			"በ%s ጊዜው ያበቃል። ካርድ ለመግዛት ይጠቀሙበት — ገንዘቡ ራሱ አይወጣም፣ ነገር ግን ያሸነፉት ገንዘብ ይወጣል።\n\n"+
			"You received a %.0f birr play bonus! Expires %s.\n"+
			"Use it to buy cards — the bonus itself cannot be withdrawn, but anything you win with it is real cash. 💰",
		grant.Amount, expiry, grant.Amount, expiry,
	)
	if err := uc.notifier.SendMessage(user.TelegramID, msg); err != nil {
		log.Printf("[bonus] granted %.2f to %s but the Telegram notice failed: %v", grant.Amount, user.ID, err)
	}
}

// Grant awards bonus to one player. Admin-triggered.
func (uc *BonusUseCase) Grant(ctx context.Context, req domain.GrantBonusRequest) (*domain.BonusGrant, error) {
	if req.Amount <= 0 {
		return nil, fmt.Errorf("amount must be greater than 0")
	}

	user, err := uc.userRepo.FindByID(ctx, req.UserID)
	if err != nil || user == nil {
		return nil, fmt.Errorf("user not found")
	}
	// Bots are bankrolled from the house float; handing them bonus on top
	// would double-count the giveaway and pollute the liability figure.
	if user.IsBot {
		return nil, fmt.Errorf("cannot grant bonus to a bot account")
	}

	tx, err := uc.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	grant, err := uc.bonusRepo.Grant(ctx, tx, req.UserID, req.Amount, req.Reason)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	uc.notifyGrant(user, grant)
	return grant, nil
}

// GrantMany awards the same bonus to several players, each in its own
// transaction so one bad user id cannot abort the whole campaign. Returns how
// many succeeded and the failures, so the admin sees a partial result honestly
// rather than an all-or-nothing lie.
func (uc *BonusUseCase) GrantMany(ctx context.Context, userIDs []uuid.UUID, amount float64, reason string) (int, map[uuid.UUID]string) {
	failures := make(map[uuid.UUID]string)
	granted := 0
	for _, id := range userIDs {
		if _, err := uc.Grant(ctx, domain.GrantBonusRequest{UserID: id, Amount: amount, Reason: reason}); err != nil {
			failures[id] = err.Error()
			continue
		}
		granted++
	}
	return granted, failures
}

// Balance returns a player's spendable bonus and its soonest expiry.
func (uc *BonusUseCase) Balance(ctx context.Context, userID uuid.UUID) (*domain.BonusBalance, error) {
	return uc.bonusRepo.Balance(ctx, userID)
}

// ListGrants returns a player's bonus history for the admin view.
func (uc *BonusUseCase) ListGrants(ctx context.Context, userID uuid.UUID, limit int) ([]*domain.BonusGrant, error) {
	return uc.bonusRepo.ListGrants(ctx, userID, limit)
}

// TotalOutstanding is the house's live bonus liability.
func (uc *BonusUseCase) TotalOutstanding(ctx context.Context) (float64, error) {
	return uc.bonusRepo.TotalOutstanding(ctx)
}

func (uc *BonusUseCase) GetConfig(ctx context.Context) (*domain.BonusConfig, error) {
	return uc.bonusRepo.GetConfig(ctx)
}

// UpdateConfig applies a partial policy update from the admin dashboard.
//
// Changing expiry_days affects only grants made afterwards: a deadline already
// promised to a player is never moved, in either direction.
func (uc *BonusUseCase) UpdateConfig(ctx context.Context, req domain.UpdateBonusConfigRequest) (*domain.BonusConfig, error) {
	cfg, err := uc.bonusRepo.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	if req.Enabled != nil {
		cfg.Enabled = *req.Enabled
	}
	if req.ExpiryDays != nil {
		if *req.ExpiryDays < 1 {
			return nil, fmt.Errorf("expiry_days must be at least 1")
		}
		if *req.ExpiryDays > 365 {
			return nil, fmt.Errorf("expiry_days cannot exceed 365")
		}
		cfg.ExpiryDays = *req.ExpiryDays
	}
	if req.Announcement != nil {
		if len(*req.Announcement) > 500 {
			return nil, fmt.Errorf("announcement cannot exceed 500 characters")
		}
		cfg.Announcement = *req.Announcement
	}
	if err := uc.bonusRepo.UpdateConfig(ctx, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
