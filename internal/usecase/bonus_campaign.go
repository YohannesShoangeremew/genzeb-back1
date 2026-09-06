package usecase

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"

	"github.com/bingo/backend/internal/domain"
	redisPkg "github.com/bingo/backend/pkg/redis"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	// campaignMaxSlots caps a single giveaway. A typo of 100000 slots would
	// otherwise create a campaign nobody can exhaust and an announcement
	// promising a share to everyone.
	campaignMaxSlots = 10000

	// campaignMinPerSlot is the smallest award worth making. Below this the
	// per-slot figure rounds to something a player cannot buy a card with, and
	// the giveaway reads as an insult rather than a promotion.
	campaignMinPerSlot = 1.0

	// campaignMaxExpiryMinutes caps a campaign bonus lifetime at a year — a
	// backstop against a typo, not a real limit anyone reaches. The floor is
	// one minute, enforced where the value is read.
	campaignMaxExpiryMinutes = 365 * 24 * 60
)

// BonusCampaignUseCase runs "today's bonus is N birr for the first M players"
// giveaways.
//
// The award itself is an ordinary bonus grant — this layer only decides WHO is
// allowed one and enforces that no more than M are handed out. Keeping it on
// top of the existing grant primitive means expiry, play-only-ness and the
// house's liability figure need no special cases for campaign money.
type BonusCampaignUseCase struct {
	repo      domain.BonusCampaignRepository
	bonusRepo domain.BonusRepository
	userRepo  domain.UserRepository
	db        *sql.DB
	// broadcaster announces a new campaign to every player. Optional: without
	// it the campaign still runs, it just is not advertised.
	broadcaster *BroadcastUseCase
	// notifier confirms an individual claim. Optional.
	notifier domain.BroadcastSender
	// rdb pushes live claim/create/end events to subscribed admin dashboards.
	// Optional: nil simply means the dashboard falls back to its own refresh.
	rdb *redis.Client
}

func NewBonusCampaignUseCase(
	repo domain.BonusCampaignRepository,
	bonusRepo domain.BonusRepository,
	userRepo domain.UserRepository,
	db *sql.DB,
	broadcaster *BroadcastUseCase,
	notifier domain.BroadcastSender,
	rdb *redis.Client,
) *BonusCampaignUseCase {
	return &BonusCampaignUseCase{
		repo:        repo,
		bonusRepo:   bonusRepo,
		userRepo:    userRepo,
		db:          db,
		broadcaster: broadcaster,
		notifier:    notifier,
		rdb:         rdb,
	}
}

// publish pushes a live admin event. After-the-fact and never fatal: a claim is
// already committed and the money already granted, so a Redis hiccup must not
// fail the operation — the dashboard reconciles on its next load regardless.
func (uc *BonusCampaignUseCase) publish(event string, data any) {
	if uc.rdb == nil {
		return
	}
	payload, err := json.Marshal(map[string]any{"event": event, "data": data})
	if err != nil {
		log.Printf("[campaign] failed to marshal %s event: %v", event, err)
		return
	}
	if err := uc.rdb.Publish(context.Background(), redisPkg.BonusCampaignChannel, payload).Err(); err != nil {
		log.Printf("[campaign] failed to publish %s event: %v", event, err)
	}
}

// Create starts a giveaway, and optionally announces it.
//
// The per-slot amount is computed and frozen here rather than divided at
// payout time, so every claimer receives an identical, already-rounded figure
// and the player who claims first sees their money immediately.
func (uc *BonusCampaignUseCase) Create(ctx context.Context, req domain.CreateBonusCampaignRequest, createdBy *uuid.UUID) (*domain.BonusCampaign, error) {
	if req.TotalAmount <= 0 {
		return nil, fmt.Errorf("total amount must be greater than 0")
	}
	if req.Slots <= 0 {
		return nil, fmt.Errorf("slots must be greater than 0")
	}
	if req.Slots > campaignMaxSlots {
		return nil, fmt.Errorf("slots cannot exceed %d", campaignMaxSlots)
	}
	if len([]rune(req.Announcement)) > 1000 {
		return nil, fmt.Errorf("announcement cannot exceed 1000 characters")
	}
	// An explicit expiry, when given, must be sane. Nil means "use the policy
	// default", which is the pre-flexible-expiry behaviour.
	if req.ExpiryMinutes != nil {
		if *req.ExpiryMinutes < 1 {
			return nil, fmt.Errorf("expiry must be at least 1 minute")
		}
		if *req.ExpiryMinutes > campaignMaxExpiryMinutes {
			return nil, fmt.Errorf("expiry cannot exceed 1 year")
		}
	}

	// Rounded DOWN to the cent: rounding up would let slots * amount_per_slot
	// exceed the pot the admin authorised.
	perSlot := math.Floor(req.TotalAmount/float64(req.Slots)*100) / 100
	if perSlot < campaignMinPerSlot {
		return nil, fmt.Errorf(
			"%.2f birr across %d slots is only %.2f each — raise the amount or lower the slots",
			req.TotalAmount, req.Slots, perSlot)
	}

	// The bonus master switch governs every grant (bonus_config.enabled), so a
	// campaign created while it is off would announce a giveaway whose every
	// claim then fails. Refuse up front instead.
	cfg, err := uc.bonusRepo.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return nil, fmt.Errorf("bonus granting is disabled — enable it before starting a campaign")
	}

	// Retire a spent campaign automatically. Claims leave the exhausted
	// campaign active on purpose (see Claim), so without this the admin would
	// have to end yesterday's by hand before every new one. Only an EXHAUSTED
	// campaign is retired silently — one with slots left is a live promise to
	// players, and replacing it is a decision the admin must make explicitly.
	if current, err := uc.repo.Active(ctx); err == nil && current != nil && current.SlotsLeft() <= 0 {
		if err := uc.repo.End(ctx, nil, current.ID); err != nil {
			return nil, err
		}
	}

	c := &domain.BonusCampaign{
		ID:            uuid.New(),
		Amount:        req.TotalAmount,
		Slots:         req.Slots,
		AmountPerSlot: perSlot,
		Announcement:  req.Announcement,
		ExpiryMinutes: req.ExpiryMinutes,
		CreatedBy:     createdBy,
	}
	if err := uc.repo.Create(ctx, c); err != nil {
		return nil, err
	}

	if req.Broadcast {
		uc.announce(ctx, c)
	}
	// Tell any open dashboard a new campaign is live, so its scoreboard appears
	// without a manual refresh.
	uc.publish("created", c)
	return c, nil
}

// announce pushes the campaign to every player on Telegram.
//
// Never fatal: the campaign exists and is claimable regardless, and failing the
// create call because Telegram hiccuped would tempt the admin to re-create it
// — which the one-active-campaign rule would then refuse, leaving them stuck.
func (uc *BonusCampaignUseCase) announce(ctx context.Context, c *domain.BonusCampaign) {
	if uc.broadcaster == nil {
		log.Printf("[campaign %s] broadcast requested but no broadcaster is configured", c.ID)
		return
	}
	msg := c.Announcement
	if msg == "" {
		msg = fmt.Sprintf(
			"🎁 የዛሬ ቦነስ: %.0f ብር!\n"+
				"ለመጀመሪያዎቹ %d ተጫዋቾች ብቻ — እያንዳንዳቸው %.0f ብር።\n"+
				"አፕሊኬሽኑን ከፍተው አሁኑኑ ይውሰዱ! ⚡\n\n"+
				"Today's bonus: %.0f birr!\n"+
				"First %d players only — %.0f birr each. Open the app and claim it now! ⚡",
			c.Amount, c.Slots, c.AmountPerSlot,
			c.Amount, c.Slots, c.AmountPerSlot,
		)
	}
	// The announcement carries a Claim button, so a player claims straight from
	// the notification — no menu digging. The callback is handled in-chat by the
	// Telegram webhook; double-claim is refused there as everywhere.
	action := &domain.BroadcastAction{Text: "🎁 ውሰድ / Claim", CallbackData: "bonus:claim"}
	if _, err := uc.broadcaster.SendWithAction(ctx, msg, c.CreatedBy, action); err != nil {
		log.Printf("[campaign %s] created but the announcement failed: %v", c.ID, err)
	}
}

// notifyClaim confirms a claim on Telegram. After the commit and never fatal —
// the money is already theirs.
func (uc *BonusCampaignUseCase) notifyClaim(user *domain.User, amount float64, position, slots int) {
	if uc.notifier == nil || user.TelegramID <= 0 {
		return
	}
	msg := fmt.Sprintf(
		"🎉 %.0f ብር የዛሬ ቦነስ ወስደዋል! (%d ከ%d)\n"+
			"ካርድ ለመግዛት ይጠቀሙበት — ገንዘቡ ራሱ አይወጣም፣ ያሸነፉት ግን ይወጣል።\n\n"+
			"You claimed %.0f birr of today's bonus! (%d of %d)\n"+
			"Use it to buy cards — the bonus itself cannot be withdrawn, but anything you win with it is real cash. 💰",
		amount, position, slots, amount, position, slots,
	)
	if err := uc.notifier.SendMessage(user.TelegramID, msg); err != nil {
		log.Printf("[campaign] claim by %s succeeded but the Telegram notice failed: %v", user.ID, err)
	}
}

// Claim takes one slot of the running campaign for this player.
//
// Everything happens inside a single transaction that holds a lock on the
// campaign row, so simultaneous claimers are serialised: with ten slots and
// twenty people pressing at the same instant, exactly ten succeed and the
// eleventh onwards get ErrCampaignExhausted rather than an oversold pot.
func (uc *BonusCampaignUseCase) Claim(ctx context.Context, userID uuid.UUID) (*domain.BonusCampaignClaim, error) {
	user, err := uc.userRepo.FindByID(ctx, userID)
	if err != nil || user == nil {
		return nil, fmt.Errorf("user not found")
	}
	// Bots are bankrolled from the house float; letting them claim would burn
	// real slots on accounts the house already owns.
	if user.IsBot {
		return nil, domain.ErrCampaignNotEligible
	}
	if user.Banned {
		return nil, domain.ErrCampaignNotEligible
	}

	eligible, err := uc.repo.HasCompletedDeposit(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !eligible {
		return nil, domain.ErrCampaignNotEligible
	}

	tx, err := uc.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	campaign, err := uc.repo.ActiveForUpdate(ctx, tx)
	if err != nil {
		return nil, err
	}
	if campaign == nil {
		return nil, domain.ErrNoActiveCampaign
	}

	// Friendly pre-check. The primary key in RecordClaim is the actual
	// guarantee; this exists so a player who already claimed gets a clear
	// message instead of a constraint error.
	if existing, ferr := uc.repo.FindClaim(ctx, campaign.ID, userID); ferr == nil && existing != nil {
		return nil, domain.ErrCampaignAlreadyClaimed
	}
	if campaign.SlotsLeft() <= 0 {
		return nil, domain.ErrCampaignExhausted
	}

	// A campaign with its own expiry mints a bonus on that clock (urgency: good
	// only for the next few hours); one without falls back to the general
	// policy lifetime, exactly as before per-campaign expiry existed.
	reason := fmt.Sprintf("campaign %s", campaign.ID)
	var grant *domain.BonusGrant
	if campaign.ExpiryMinutes != nil {
		grant, err = uc.bonusRepo.GrantWithExpiry(ctx, tx, userID, campaign.AmountPerSlot, reason, *campaign.ExpiryMinutes)
	} else {
		grant, err = uc.bonusRepo.Grant(ctx, tx, userID, campaign.AmountPerSlot, reason)
	}
	if err != nil {
		return nil, err
	}

	position, err := uc.repo.RecordClaim(ctx, tx, campaign.ID, userID, campaign.AmountPerSlot, grant.ID)
	if err != nil {
		return nil, err
	}

	// Deliberately NOT ended when the last slot goes.
	//
	// An exhausted campaign stays active so the players who lost the race are
	// told "all slots claimed" rather than "no campaign is running" — under a
	// stampede that is most of the audience, and telling them nothing was on
	// makes the promotion look broken at the exact moment it worked. It also
	// keeps the campaign on the player's screen showing 0 of N left, which is
	// the proof the giveaway was real. Create() retires it when the next one
	// starts; an admin can also end it explicitly.

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	uc.notifyClaim(user, campaign.AmountPerSlot, position, campaign.Slots)

	claim := &domain.BonusCampaignClaim{
		CampaignID: campaign.ID,
		UserID:     userID,
		GrantID:    &grant.ID,
		Amount:     campaign.AmountPerSlot,
		Position:   position,
		ClaimedAt:  grant.GrantedAt,
		// Identity carried on the live event so the admin scoreboard can show
		// the new row without a lookup, exactly as ListClaims would.
		Name:  user.FirstName,
		Phone: user.PhoneNumber,
	}
	// Live update for the admin scoreboard: the new claim plus the running
	// count, so a stampede fills the bar in real time.
	uc.publish("claim", map[string]any{
		"campaign_id":   campaign.ID,
		"claimed_count": position,
		"slots":         campaign.Slots,
		"claim":         claim,
	})
	return claim, nil
}

// Status is what the player's bonus screen shows: the running campaign, and
// whether this player can take a slot.
//
// Returns a nil Campaign rather than an error when nothing is running, so the
// client renders an empty state instead of treating a normal quiet day as a
// failure.
func (uc *BonusCampaignUseCase) Status(ctx context.Context, userID uuid.UUID) (*domain.BonusCampaignStatus, error) {
	campaign, err := uc.repo.Active(ctx)
	if err != nil {
		return nil, err
	}
	status := &domain.BonusCampaignStatus{Campaign: campaign}
	if campaign == nil {
		status.Reason = domain.ReasonCode(domain.ErrNoActiveCampaign)
		return status, nil
	}

	if claim, err := uc.repo.FindClaim(ctx, campaign.ID, userID); err == nil && claim != nil {
		status.Claimed = true
		status.ClaimedAmount = claim.Amount
		status.Reason = domain.ReasonCode(domain.ErrCampaignAlreadyClaimed)
		return status, nil
	}

	if campaign.SlotsLeft() <= 0 {
		status.Reason = domain.ReasonCode(domain.ErrCampaignExhausted)
		return status, nil
	}

	eligible, err := uc.repo.HasCompletedDeposit(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !eligible {
		status.Reason = domain.ReasonCode(domain.ErrCampaignNotEligible)
		return status, nil
	}

	status.CanClaim = true
	return status, nil
}

// End stops a campaign early. Slots already claimed are untouched — the money
// is granted and gone.
func (uc *BonusCampaignUseCase) End(ctx context.Context, id uuid.UUID) (*domain.BonusCampaign, error) {
	campaign, err := uc.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if campaign == nil {
		return nil, fmt.Errorf("campaign not found")
	}
	if err := uc.repo.End(ctx, nil, id); err != nil {
		return nil, err
	}
	// Tell open dashboards the campaign is over, so the scoreboard clears.
	uc.publish("ended", map[string]any{"campaign_id": id})
	return uc.repo.FindByID(ctx, id)
}

func (uc *BonusCampaignUseCase) Active(ctx context.Context) (*domain.BonusCampaign, error) {
	return uc.repo.Active(ctx)
}

func (uc *BonusCampaignUseCase) List(ctx context.Context, limit int) ([]*domain.BonusCampaign, error) {
	return uc.repo.List(ctx, limit)
}

func (uc *BonusCampaignUseCase) ListClaims(ctx context.Context, id uuid.UUID) ([]*domain.BonusCampaignClaim, error) {
	return uc.repo.ListClaims(ctx, id)
}

// IsClaimRefusal reports whether an error is a player-facing refusal (slots
// gone, already claimed, not eligible) rather than a fault. The HTTP layer uses
// it to answer 409 instead of 500 — a refused claim is a normal outcome of a
// race, not a bug.
func IsClaimRefusal(err error) bool {
	return errors.Is(err, domain.ErrNoActiveCampaign) ||
		errors.Is(err, domain.ErrCampaignExhausted) ||
		errors.Is(err, domain.ErrCampaignAlreadyClaimed) ||
		errors.Is(err, domain.ErrCampaignNotEligible)
}
