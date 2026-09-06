//go:build integration

package usecase

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/bingo/backend/internal/domain"
	"github.com/bingo/backend/internal/repository/postgres"
	"github.com/google/uuid"
)

func (h *harness) campaignUC() *BonusCampaignUseCase {
	h.t.Helper()
	return NewBonusCampaignUseCase(
		postgres.NewBonusCampaignRepository(h.db),
		postgres.NewBonusRepository(h.db),
		postgres.NewUserRepository(h.db),
		h.db,
		nil, // no broadcaster: these tests must not message real players
		nil, // no notifier, same reason
		nil, // no redis: publish() is a no-op, the claim logic is unaffected
	)
}

// dropCampaign removes a campaign and everything it produced. Claims cascade,
// but the grants do not — they are ordinary bonus rows keyed by user.
func (h *harness) dropCampaign(id uuid.UUID) {
	h.t.Helper()
	h.db.Exec(`DELETE FROM bonus_grants WHERE reason = $1`, "campaign "+id.String())
	h.db.Exec(`DELETE FROM bonus_campaigns WHERE id = $1`, id)
}

// TestIntegration_Campaign_StampedeHandsOutExactlyNSlots is the test this
// feature exists for: an announcement goes out and everyone claims at once.
// Twenty eligible players race for ten slots.
func TestIntegration_Campaign_StampedeHandsOutExactlyNSlots(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()
	uc := h.campaignUC()

	const players, slots = 20, 10
	users := make([]uuid.UUID, players)
	for i := 0; i < players; i++ {
		users[i] = h.seedUser("Racer", int64(700+i))
		// Eligibility: every racer is a real depositing customer.
		h.addCompletedDeposit(users[i], 50, "CAMPRACE")
	}

	campaign, err := uc.Create(ctx, domain.CreateBonusCampaignRequest{
		TotalAmount: 1000,
		Slots:       slots,
	}, nil)
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	defer h.dropCampaign(campaign.ID)

	if campaign.AmountPerSlot != 100 {
		t.Fatalf("1000 over 10 slots should be 100 each, got %v", campaign.AmountPerSlot)
	}
	// The admin screen renders this straight from the create response, so a
	// zero time here is a visible "started year 1" bug, not a cosmetic one.
	if campaign.CreatedAt.IsZero() {
		t.Fatal("Create returned a zero created_at")
	}

	// All twenty press Claim at the same instant.
	var wg sync.WaitGroup
	start := make(chan struct{})
	claims := make([]*domain.BonusCampaignClaim, players)
	errs := make([]error, players)
	for i := 0; i < players; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			claims[i], errs[i] = uc.Claim(ctx, users[i])
		}(i)
	}
	close(start)
	wg.Wait()

	won, exhausted, other := 0, 0, 0
	positions := map[int]int{}
	for i := range users {
		switch {
		case errs[i] == nil:
			won++
			positions[claims[i].Position]++
		case errors.Is(errs[i], domain.ErrCampaignExhausted):
			exhausted++
		default:
			other++
			t.Errorf("player %d got an unexpected error: %v", i, errs[i])
		}
	}

	if won != slots {
		t.Fatalf("expected exactly %d winners, got %d (exhausted=%d other=%d)", slots, won, exhausted, other)
	}
	if exhausted != players-slots {
		t.Fatalf("expected %d players turned away, got %d", players-slots, exhausted)
	}
	// Positions must be 1..N with no duplicates — a duplicate would mean two
	// players were told they were the same place in the queue.
	for p := 1; p <= slots; p++ {
		if positions[p] != 1 {
			t.Fatalf("position %d was handed out %d times, want exactly 1", p, positions[p])
		}
	}

	// The money actually landed, and only for winners.
	granted := 0
	for i := range users {
		bal := h.bonusBalance(users[i])
		if errs[i] == nil {
			if bal != 100 {
				t.Errorf("winner %d has bonus %v, want 100", i, bal)
			}
			granted++
		} else if bal != 0 {
			t.Errorf("turned-away player %d received bonus %v, want 0", i, bal)
		}
	}
	if granted != slots {
		t.Fatalf("granted bonus to %d players, want %d", granted, slots)
	}

	// An exhausted campaign stays ACTIVE, so the losers of the race are told
	// the slots are gone rather than that nothing was running.
	after, err := uc.repo.FindByID(ctx, campaign.ID)
	if err != nil {
		t.Fatalf("reload campaign: %v", err)
	}
	if after.Status != domain.BonusCampaignStatusActive {
		t.Fatalf("campaign status after exhaustion = %q, want it to stay active", after.Status)
	}
	if after.ClaimedCount != slots {
		t.Fatalf("claimed_count = %d, want %d", after.ClaimedCount, slots)
	}

	// And a latecomer's screen says so, rather than showing an empty state.
	latecomer := h.seedUser("Latecomer", 799)
	h.addCompletedDeposit(latecomer, 50, "CAMPLATE")
	status, err := uc.Status(ctx, latecomer)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Campaign == nil {
		t.Fatal("latecomer sees no campaign at all; want the exhausted one")
	}
	if status.CanClaim || status.Reason != domain.ReasonExhausted {
		t.Fatalf("latecomer status = can_claim:%v reason:%q, want exhausted", status.CanClaim, status.Reason)
	}
}

// TestIntegration_Campaign_NextCampaignRetiresSpentOne covers the handover:
// because an exhausted campaign stays active for messaging, starting tomorrow's
// must retire it automatically rather than making the admin do it by hand.
func TestIntegration_Campaign_NextCampaignRetiresSpentOne(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()
	uc := h.campaignUC()

	user := h.seedUser("OnlySlot", 780)
	h.addCompletedDeposit(user, 50, "CAMPHANDOVER")

	today, err := uc.Create(ctx, domain.CreateBonusCampaignRequest{TotalAmount: 100, Slots: 1}, nil)
	if err != nil {
		t.Fatalf("create today's campaign: %v", err)
	}
	defer h.dropCampaign(today.ID)

	if _, err := uc.Claim(ctx, user); err != nil {
		t.Fatalf("claim the only slot: %v", err)
	}

	tomorrow, err := uc.Create(ctx, domain.CreateBonusCampaignRequest{TotalAmount: 200, Slots: 2}, nil)
	if err != nil {
		t.Fatalf("create tomorrow's campaign after today's was spent: %v", err)
	}
	defer h.dropCampaign(tomorrow.ID)

	spent, _ := uc.repo.FindByID(ctx, today.ID)
	if spent.Status != domain.BonusCampaignStatusEnded {
		t.Fatalf("spent campaign status = %q, want ended once the next one started", spent.Status)
	}
	active, _ := uc.repo.Active(ctx)
	if active == nil || active.ID != tomorrow.ID {
		t.Fatal("the active campaign is not tomorrow's")
	}
}

// TestIntegration_Campaign_SamePlayerCannotClaimTwice covers the double-tapped
// button: the second press must be refused and must not mint a second grant.
func TestIntegration_Campaign_SamePlayerCannotClaimTwice(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()
	uc := h.campaignUC()

	user := h.seedUser("DoubleTap", 750)
	h.addCompletedDeposit(user, 50, "CAMPTWICE")

	campaign, err := uc.Create(ctx, domain.CreateBonusCampaignRequest{TotalAmount: 500, Slots: 5}, nil)
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	defer h.dropCampaign(campaign.ID)

	if _, err := uc.Claim(ctx, user); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	_, err = uc.Claim(ctx, user)
	if !errors.Is(err, domain.ErrCampaignAlreadyClaimed) {
		t.Fatalf("second claim error = %v, want ErrCampaignAlreadyClaimed", err)
	}

	if bal := h.bonusBalance(user); bal != 100 {
		t.Fatalf("bonus after a double claim = %v, want 100 (one grant only)", bal)
	}
	// The refused attempt must not have burned a slot either.
	after, _ := uc.repo.FindByID(ctx, campaign.ID)
	if after.ClaimedCount != 1 {
		t.Fatalf("claimed_count = %d after a double claim, want 1", after.ClaimedCount)
	}
}

// TestIntegration_Campaign_RequiresDeposit is the anti-multi-account rule: an
// account that never put money in cannot take a slot.
func TestIntegration_Campaign_RequiresDeposit(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()
	uc := h.campaignUC()

	freeloader := h.seedUser("NoDeposit", 760)
	// A system credit is NOT a deposit: keying on transaction type rather than
	// category would wrongly let this through.
	h.addSystemDeposit(freeloader, domain.TransactionCategoryAdminCredit, 25, "ADMINGIFT-760")

	campaign, err := uc.Create(ctx, domain.CreateBonusCampaignRequest{TotalAmount: 500, Slots: 5}, nil)
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	defer h.dropCampaign(campaign.ID)

	_, err = uc.Claim(ctx, freeloader)
	if !errors.Is(err, domain.ErrCampaignNotEligible) {
		t.Fatalf("claim error = %v, want ErrCampaignNotEligible", err)
	}
	if bal := h.bonusBalance(freeloader); bal != 0 {
		t.Fatalf("ineligible player received bonus %v, want 0", bal)
	}

	status, err := uc.Status(ctx, freeloader)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.CanClaim {
		t.Fatal("status says an ineligible player can claim")
	}
	// The app switches on this CODE to pick its Amharic wording. If it ever
	// becomes English prose again, the player sees a dead button with no
	// explanation — so assert the contract, not just that something is set.
	if status.Reason != domain.ReasonNotEligible {
		t.Fatalf("status reason = %q, want the %q code", status.Reason, domain.ReasonNotEligible)
	}
}

// TestIntegration_Campaign_OnlyOneActiveAtATime guards the rule that makes
// "the active campaign" well defined.
func TestIntegration_Campaign_OnlyOneActiveAtATime(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()
	uc := h.campaignUC()

	first, err := uc.Create(ctx, domain.CreateBonusCampaignRequest{TotalAmount: 300, Slots: 3}, nil)
	if err != nil {
		t.Fatalf("create first campaign: %v", err)
	}
	defer h.dropCampaign(first.ID)

	if _, err := uc.Create(ctx, domain.CreateBonusCampaignRequest{TotalAmount: 200, Slots: 2}, nil); err == nil {
		t.Fatal("a second campaign was created while one was already running")
	}

	// Ending the first frees the slot for tomorrow's.
	if _, err := uc.End(ctx, first.ID); err != nil {
		t.Fatalf("end campaign: %v", err)
	}
	second, err := uc.Create(ctx, domain.CreateBonusCampaignRequest{TotalAmount: 200, Slots: 2}, nil)
	if err != nil {
		t.Fatalf("create after ending the first: %v", err)
	}
	defer h.dropCampaign(second.ID)
}

// TestIntegration_Campaign_ExpiryOverride proves a campaign's own short expiry
// lands on the granted bonus — the urgency lever ("claim it, use it tonight").
func TestIntegration_Campaign_ExpiryOverride(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()
	uc := h.campaignUC()

	user := h.seedUser("QuickExpiry", 770)
	h.addCompletedDeposit(user, 50, "CAMPEXP")

	threeHours := 180
	campaign, err := uc.Create(ctx, domain.CreateBonusCampaignRequest{
		TotalAmount:   200,
		Slots:         2,
		ExpiryMinutes: &threeHours,
	}, nil)
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	defer h.dropCampaign(campaign.ID)
	if campaign.ExpiryMinutes == nil || *campaign.ExpiryMinutes != threeHours {
		t.Fatalf("campaign expiry not persisted: %v", campaign.ExpiryMinutes)
	}

	claim, err := uc.Claim(ctx, user)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}

	// The granted bonus must expire ~3 hours out, not the 7-day policy default.
	var expiresAt, grantedAt time.Time
	if err := h.db.QueryRow(
		`SELECT expires_at, granted_at FROM bonus_grants WHERE id = $1`, *claim.GrantID,
	).Scan(&expiresAt, &grantedAt); err != nil {
		t.Fatalf("read grant: %v", err)
	}
	gotMinutes := expiresAt.Sub(grantedAt).Minutes()
	if gotMinutes < 179 || gotMinutes > 181 {
		t.Fatalf("bonus expiry = %.1f minutes, want ~180 (campaign override ignored?)", gotMinutes)
	}
}

// TestIntegration_Campaign_RejectsUnpayableSplit stops a campaign whose slots
// are so thin the per-player award is worthless.
func TestIntegration_Campaign_RejectsUnpayableSplit(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()
	uc := h.campaignUC()

	if _, err := uc.Create(ctx, domain.CreateBonusCampaignRequest{TotalAmount: 10, Slots: 100}, nil); err == nil {
		t.Fatal("10 birr across 100 slots was accepted; want a refusal")
	}
}
