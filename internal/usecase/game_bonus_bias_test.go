package usecase

import (
	"testing"
	"time"

	"github.com/bingo/backend/internal/domain"
	"github.com/bingo/backend/pkg/bingo"
	"github.com/google/uuid"
)

func TestLegacySafeDrawCandidatesReproducesBonusOnlyFullHopper(t *testing.T) {
	card := bingo.GenerateCard(1)
	if card == nil {
		t.Fatal("expected generated card")
	}

	drawnSet := make(map[int]bool)
	for col := 0; col < domain.CardGridSize-1; col++ {
		drawnSet[card.Numbers[0][col]] = true
	}
	completing := card.Numbers[0][domain.CardGridSize-1]

	bonusPlayer := &domain.GamePlayer{UserID: uuid.New(), CardID: card.ID, Paid: true, PaidFromBonus: true}
	if candidates := legacySafeDrawCandidates([]*domain.GamePlayer{bonusPlayer}, drawnSet); containsNumber(candidates, completing) {
		t.Fatalf("legacy mode included bonus-card completing number %d", completing)
	}

	walletPlayer := &domain.GamePlayer{UserID: uuid.New(), CardID: card.ID, Paid: true}
	walletCandidates := legacySafeDrawCandidates([]*domain.GamePlayer{walletPlayer}, drawnSet)
	if !containsNumber(walletCandidates, completing) {
		t.Fatalf("legacy mode should leave wallet-card completing number %d in the hopper", completing)
	}

	cardNumbers := cardNumberSet(card)
	outsideFound := false
	for n := domain.BingoNumberMinB; n <= domain.BingoNumberMaxO; n++ {
		if !drawnSet[n] && !cardNumbers[n] && containsNumber(walletCandidates, n) {
			outsideFound = true
			break
		}
	}
	if !outsideFound {
		t.Fatal("legacy mode must draw from the full hopper, not only card numbers")
	}
}

func TestBotSafeDrawCandidatesProtectsEveryRealPlayerAndUsesOnlyBotCards(t *testing.T) {
	humanCard := bingo.GenerateCard(1)
	if humanCard == nil {
		t.Fatal("expected generated human card")
	}

	drawnSet := make(map[int]bool)
	for col := 0; col < domain.CardGridSize-1; col++ {
		drawnSet[humanCard.Numbers[0][col]] = true
	}
	humanCompleting := humanCard.Numbers[0][domain.CardGridSize-1]
	botCard := findBotCardContainingSafeNumber(t, humanCompleting, drawnSet, humanCard.ID)

	bot := &domain.GamePlayer{UserID: uuid.New(), CardID: botCard.ID, Paid: true, IsBot: true}
	withoutHuman := botSafeDrawCandidates([]*domain.GamePlayer{bot}, drawnSet, false)
	if !containsNumber(withoutHuman, humanCompleting) {
		t.Fatalf("test setup: bot candidate pool should contain %d without a human card", humanCompleting)
	}

	// This is an ordinary wallet-funded human card. All real cards must be
	// protected; the rule is no longer limited to bonus-funded cards.
	human := &domain.GamePlayer{UserID: uuid.New(), CardID: humanCard.ID, Paid: true}
	candidates := botSafeDrawCandidates([]*domain.GamePlayer{human, bot}, drawnSet, false)
	if containsNumber(candidates, humanCompleting) {
		t.Fatalf("human-completing number %d must be excluded", humanCompleting)
	}
	if len(candidates) == 0 {
		t.Fatal("expected at least one safe bot-card candidate")
	}

	botNumbers := cardNumberSet(botCard)
	for _, n := range candidates {
		if drawnSet[n] {
			t.Fatalf("candidate %d was already drawn", n)
		}
		if !botNumbers[n] {
			t.Fatalf("candidate %d does not appear on the active bot card", n)
		}
	}

	for n := domain.BingoNumberMinB; n <= domain.BingoNumberMaxO; n++ {
		if !drawnSet[n] && !botNumbers[n] && containsNumber(candidates, n) {
			t.Fatalf("number %d outside all bot cards entered the candidate pool", n)
		}
	}
}

func TestBotSafeDrawCandidatesBlocksEarlyBotBingoAndForcesItAtTarget(t *testing.T) {
	botCard := bingo.GenerateCard(2)
	if botCard == nil {
		t.Fatal("expected generated bot card")
	}

	drawnSet := make(map[int]bool)
	for col := 0; col < domain.CardGridSize-1; col++ {
		drawnSet[botCard.Numbers[0][col]] = true
	}
	botCompleting := botCard.Numbers[0][domain.CardGridSize-1]
	players := []*domain.GamePlayer{{UserID: uuid.New(), CardID: botCard.ID, Paid: true, IsBot: true}}

	beforeTarget := botSafeDrawCandidates(players, drawnSet, false)
	if containsNumber(beforeTarget, botCompleting) {
		t.Fatalf("bot-completing number %d must be excluded before the target", botCompleting)
	}

	atTarget := botSafeDrawCandidates(players, drawnSet, true)
	if !containsNumber(atTarget, botCompleting) {
		t.Fatalf("bot-completing number %d should be available at the target", botCompleting)
	}
	botDanger := make(map[int]bool)
	appendDangerNumbers(botCard, drawnSet, botDanger)
	for _, n := range atTarget {
		if !botDanger[n] {
			t.Fatalf("forced candidate %d does not complete a bot bingo", n)
		}
	}
}

func TestBotSafeDrawCandidatesProtectsHumanFourCorners(t *testing.T) {
	humanCard := bingo.GenerateCard(1)
	if humanCard == nil {
		t.Fatal("expected generated human card")
	}

	drawnSet := map[int]bool{
		humanCard.Numbers[0][0]: true,
		humanCard.Numbers[0][4]: true,
		humanCard.Numbers[4][0]: true,
	}
	completingCorner := humanCard.Numbers[4][4]
	botCard := findBotCardContainingSafeNumber(t, completingCorner, drawnSet, humanCard.ID)
	players := []*domain.GamePlayer{
		{UserID: uuid.New(), CardID: humanCard.ID, Paid: true},
		{UserID: uuid.New(), CardID: botCard.ID, Paid: true, IsBot: true},
	}

	if candidates := botSafeDrawCandidates(players, drawnSet, false); containsNumber(candidates, completingCorner) {
		t.Fatalf("four-corners completing number %d must be excluded", completingCorner)
	}
}

func TestFullHopperSafeDrawCandidatesKeepsGameMovingWithoutBots(t *testing.T) {
	humanCard := bingo.GenerateCard(1)
	if humanCard == nil {
		t.Fatal("expected generated human card")
	}

	drawnSet := make(map[int]bool)
	for col := 0; col < domain.CardGridSize-1; col++ {
		drawnSet[humanCard.Numbers[0][col]] = true
	}
	humanCompleting := humanCard.Numbers[0][domain.CardGridSize-1]
	players := []*domain.GamePlayer{{UserID: uuid.New(), CardID: humanCard.ID, Paid: true}}

	if candidates := botSafeDrawCandidates(players, drawnSet, false); len(candidates) != 0 {
		t.Fatalf("bot-only pool should be empty without bot cards: %v", candidates)
	}
	fallback := fullHopperSafeDrawCandidates(players, drawnSet)
	if len(fallback) == 0 {
		t.Fatal("full-hopper fallback should keep the game moving")
	}
	if containsNumber(fallback, humanCompleting) {
		t.Fatalf("fallback included human-completing number %d", humanCompleting)
	}
}

func TestFullHopperSafeDrawCandidatesAvoidsEarlyBotCompletion(t *testing.T) {
	botCard := bingo.GenerateCard(2)
	if botCard == nil {
		t.Fatal("expected generated bot card")
	}

	drawnSet := make(map[int]bool)
	for col := 0; col < domain.CardGridSize-1; col++ {
		drawnSet[botCard.Numbers[0][col]] = true
	}
	botCompleting := botCard.Numbers[0][domain.CardGridSize-1]
	players := []*domain.GamePlayer{{UserID: uuid.New(), CardID: botCard.ID, Paid: true, IsBot: true}}

	if candidates := fullHopperSafeDrawCandidates(players, drawnSet); containsNumber(candidates, botCompleting) {
		t.Fatalf("fallback included early bot-completing number %d while stricter choices exist", botCompleting)
	}
}

func TestFullHopperSafeDrawCandidatesEmptyOnlyWhenNothingIsUndrawn(t *testing.T) {
	drawnSet := make(map[int]bool, domain.BingoNumberMaxO-domain.BingoNumberMinB+1)
	for n := domain.BingoNumberMinB; n <= domain.BingoNumberMaxO; n++ {
		drawnSet[n] = true
	}
	if candidates := fullHopperSafeDrawCandidates(nil, drawnSet); len(candidates) != 0 {
		t.Fatalf("exhausted hopper returned candidates: %v", candidates)
	}
}

func TestBotSafeDrawCandidatesIgnoresInactiveCards(t *testing.T) {
	leftAt := time.Now()
	players := []*domain.GamePlayer{
		nil,
		{UserID: uuid.New(), CardID: 1, Paid: true, IsBot: true, IsEliminated: true},
		{UserID: uuid.New(), CardID: 2, Paid: true, IsBot: true, LeftAt: &leftAt},
	}
	if candidates := botSafeDrawCandidates(players, map[int]bool{}, false); len(candidates) != 0 {
		t.Fatalf("inactive bot cards produced candidates: %v", candidates)
	}
}

func TestEffectiveGameDrawPolicyIsDynamicPerRoom(t *testing.T) {
	cfg := domain.BotConfig{
		MinimumRoomPlayers: 20,
		BiasedDrawMode:     domain.BiasedDrawModeProtected,
	}
	human := &domain.GamePlayer{UserID: uuid.New(), CardID: 1, Paid: true}
	bot := &domain.GamePlayer{UserID: uuid.New(), CardID: 2, Paid: true, IsBot: true}

	if policy := effectiveGameDrawPolicy(cfg, []*domain.GamePlayer{human}); policy.mode != domain.BiasedDrawModeDisabled || policy.bonusGuard {
		t.Fatalf("bot-free room policy = %+v, want fair", policy)
	}
	if policy := effectiveGameDrawPolicy(cfg, []*domain.GamePlayer{human, bot}); policy.mode != domain.BiasedDrawModeProtected || policy.bonusGuard {
		t.Fatalf("low-population policy = %+v, want saved protected mode", policy)
	}

	highPopulation := make([]*domain.GamePlayer, 0, 21)
	for i := 0; i < 20; i++ {
		highPopulation = append(highPopulation, &domain.GamePlayer{
			UserID:        uuid.New(),
			CardID:        i + 1,
			Paid:          true,
			PaidFromBonus: i == 0,
		})
	}
	highPopulation = append(highPopulation, bot)
	if policy := effectiveGameDrawPolicy(cfg, highPopulation); policy.mode != domain.BiasedDrawModeDisabled || !policy.bonusGuard {
		t.Fatalf("high-population bonus policy = %+v, want bonus-only guard", policy)
	}

	highPopulation[0].PaidFromBonus = false
	if policy := effectiveGameDrawPolicy(cfg, highPopulation); policy.mode != domain.BiasedDrawModeDisabled || policy.bonusGuard {
		t.Fatalf("high-population wallet policy = %+v, want fully fair", policy)
	}

	cfg.BiasedDrawMode = domain.BiasedDrawModeDisabled
	highPopulation[0].PaidFromBonus = true
	if policy := effectiveGameDrawPolicy(cfg, highPopulation); policy.mode != domain.BiasedDrawModeDisabled || policy.bonusGuard {
		t.Fatalf("disabled saved policy = %+v, want no bonus guard", policy)
	}
}

func TestWinnerEligibleBonusGuardIsPerCardAndFairToWallets(t *testing.T) {
	policy := gameDrawPolicy{mode: domain.BiasedDrawModeDisabled, bonusGuard: true}
	userID := uuid.New()
	bonusCard := &domain.GamePlayer{UserID: userID, CardID: 10, Paid: true, PaidFromBonus: true}
	walletCard := &domain.GamePlayer{UserID: userID, CardID: 11, Paid: true}
	botCard := &domain.GamePlayer{UserID: uuid.New(), CardID: 12, Paid: true, IsBot: true}

	if winnerEligible(policy, bonusCard) {
		t.Fatal("bonus-funded card remained eligible under the high-population guard")
	}
	if !winnerEligible(policy, walletCard) {
		t.Fatal("wallet-funded card was excluded under the high-population guard")
	}
	if !winnerEligible(policy, botCard) {
		t.Fatal("bot card was excluded under the high-population guard")
	}

	all := []winnerCard{{UserID: walletCard.UserID, CardID: walletCard.CardID}, {UserID: botCard.UserID, CardID: botCard.CardID}}
	bots := all[1:]
	selected := selectMixedWinners(all, bots, policy.mode != domain.BiasedDrawModeDisabled)
	if len(selected) != len(all) {
		t.Fatalf("high-population wallet/bot tie returned %d winners, want %d", len(selected), len(all))
	}
}

func findBotCardContainingSafeNumber(t *testing.T, target int, drawnSet map[int]bool, excludedCardID int) *bingo.BingoCard {
	t.Helper()
	for cardID := domain.MinCardID; cardID <= domain.MaxCardID; cardID++ {
		if cardID == excludedCardID {
			continue
		}
		card := bingo.GenerateCard(cardID)
		if card == nil || !cardNumberSet(card)[target] {
			continue
		}
		danger := make(map[int]bool)
		appendDangerNumbers(card, drawnSet, danger)
		if !danger[target] {
			return card
		}
	}
	t.Fatalf("could not find bot card containing non-completing number %d", target)
	return nil
}

func cardNumberSet(card *bingo.BingoCard) map[int]bool {
	numbers := make(map[int]bool, domain.CardTotalPositions)
	for row := 0; row < domain.CardGridSize; row++ {
		for col := 0; col < domain.CardGridSize; col++ {
			n := card.Numbers[row][col]
			if n != domain.CardCenterValue {
				numbers[n] = true
			}
		}
	}
	return numbers
}

func containsNumber(numbers []int, target int) bool {
	for _, n := range numbers {
		if n == target {
			return true
		}
	}
	return false
}
