//go:build integration

package usecase

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bingo/backend/internal/domain"
	"github.com/google/uuid"
)

func (h *harness) activeCardCount(gameID, userID uuid.UUID) int {
	h.t.Helper()
	var count int
	if err := h.db.QueryRow(
		`SELECT count(*) FROM game_players WHERE game_id=$1 AND user_id=$2 AND left_at IS NULL`,
		gameID, userID,
	).Scan(&count); err != nil {
		h.t.Fatalf("active card count: %v", err)
	}
	return count
}

func (h *harness) txCountByReference(userID uuid.UUID, reference string) int {
	h.t.Helper()
	var count int
	if err := h.db.QueryRow(
		`SELECT count(*) FROM transactions WHERE user_id=$1 AND reference=$2`,
		userID, reference,
	).Scan(&count); err != nil {
		h.t.Fatalf("transaction count by reference: %v", err)
	}
	return count
}

func (h *harness) addPaidCard(gameID, userID uuid.UUID, cardID, order int) {
	h.t.Helper()
	joinedAt := time.Now().Add(time.Duration(order) * time.Second)
	if _, err := h.db.Exec(
		`INSERT INTO game_players (id, game_id, user_id, card_id, paid, is_eliminated, joined_at)
		 VALUES ($1,$2,$3,$4,true,false,$5)`,
		uuid.New(), gameID, userID, cardID, joinedAt,
	); err != nil {
		h.t.Fatalf("add paid card: %v", err)
	}
}

func TestIntegration_GameRoom_JoinValidationTakenCardsAndMaxCards(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	user := h.seedUser("CardBuyer", 201)
	other := h.seedUser("CardTaker", 202)
	h.setBalance(user, float64(domain.MaxCardsPerPlayer)*domain.BetAmountRegular)
	h.setBalance(other, domain.BetAmountRegular)
	gameID := h.seedWaitingGame()

	for _, cardID := range []int{domain.MinCardID - 1, domain.MaxCardID + 1} {
		if _, err := h.uc.JoinGame(ctx, gameID, domain.JoinGameRequest{UserID: user, CardID: cardID}); err == nil {
			t.Fatalf("card id %d should be rejected", cardID)
		}
	}

	first, err := h.uc.JoinGame(ctx, gameID, domain.JoinGameRequest{UserID: user, CardID: 1})
	if err != nil {
		t.Fatalf("reserve first card: %v", err)
	}
	again, err := h.uc.JoinGame(ctx, gameID, domain.JoinGameRequest{UserID: user, CardID: 1})
	if err != nil {
		t.Fatalf("idempotent same-card join: %v", err)
	}
	if again.ID != first.ID {
		t.Fatalf("same-card join should return existing reservation: first=%s again=%s", first.ID, again.ID)
	}
	if h.activeCardCount(gameID, user) != 1 {
		t.Fatalf("idempotent join created duplicate active rows")
	}

	if _, err := h.uc.JoinGame(ctx, gameID, domain.JoinGameRequest{UserID: other, CardID: 1}); err == nil {
		t.Fatal("another user should not be able to take an active card")
	}

	for cardID := 2; cardID <= domain.MaxCardsPerPlayer; cardID++ {
		if _, err := h.uc.JoinGame(ctx, gameID, domain.JoinGameRequest{UserID: user, CardID: cardID}); err != nil {
			t.Fatalf("reserve card %d: %v", cardID, err)
		}
	}
	if h.balance(user) != float64(domain.MaxCardsPerPlayer)*domain.BetAmountRegular {
		t.Fatalf("reservations should not charge before start: balance=%v", h.balance(user))
	}
	if _, err := h.uc.JoinGame(ctx, gameID, domain.JoinGameRequest{UserID: user, CardID: domain.MaxCardsPerPlayer + 1}); err == nil {
		t.Fatal("joining more than the per-player card cap should fail")
	} else if !strings.Contains(err.Error(), "maximum") {
		t.Fatalf("expected max-card error, got %v", err)
	}

	g := h.gameState(gameID)
	if g.PlayerCount != 1 {
		t.Fatalf("same user holding multiple cards should count as one player, got %d", g.PlayerCount)
	}
	wantPool := float64(domain.MaxCardsPerPlayer) * domain.BetAmountRegular * (1 - domain.HouseCut)
	if g.PrizePool != wantPool {
		t.Fatalf("pool should count cards, not users: want %v got %v", wantPool, g.PrizePool)
	}
}

func TestIntegration_GameRoom_LeaveUnpaidReservationReleasesWithoutRefund(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	user := h.seedUser("UnpaidLeaver", 203)
	h.setBalance(user, 50)
	gameID := h.seedWaitingGame()

	if _, err := h.uc.JoinGame(ctx, gameID, domain.JoinGameRequest{UserID: user, CardID: 10}); err != nil {
		t.Fatalf("reserve card 10: %v", err)
	}
	if _, err := h.uc.JoinGame(ctx, gameID, domain.JoinGameRequest{UserID: user, CardID: 11}); err != nil {
		t.Fatalf("reserve card 11: %v", err)
	}

	if err := h.uc.LeaveGame(ctx, gameID, domain.LeaveGameRequest{UserID: user, CardID: 10}); err != nil {
		t.Fatalf("leave one unpaid card: %v", err)
	}
	if h.balance(user) != 50 {
		t.Fatalf("unpaid leave should not refund or charge: balance=%v", h.balance(user))
	}
	if paid, active := h.cardState(gameID, user, 10); paid || active {
		t.Fatalf("left card should be inactive and unpaid, got paid=%v active=%v", paid, active)
	}
	if paid, active := h.cardState(gameID, user, 11); paid || !active {
		t.Fatalf("remaining card should stay active and unpaid, got paid=%v active=%v", paid, active)
	}
	if refunds := h.txCountByReference(user, "GAME_REFUND"); refunds != 0 {
		t.Fatalf("unpaid leave should not create refund tx, got %d", refunds)
	}
	g := h.gameState(gameID)
	if g.PlayerCount != 1 {
		t.Fatalf("remaining card should keep player in game, got player_count=%d", g.PlayerCount)
	}
	if g.PrizePool != domain.BetAmountRegular*(1-domain.HouseCut) {
		t.Fatalf("pool after dropping one card: got %v", g.PrizePool)
	}

	if err := h.uc.LeaveGame(ctx, gameID, domain.LeaveGameRequest{UserID: user}); err != nil {
		t.Fatalf("leave remaining unpaid cards: %v", err)
	}
	if h.balance(user) != 50 {
		t.Fatalf("leaving all unpaid cards should leave balance unchanged: %v", h.balance(user))
	}
	g = h.gameState(gameID)
	if g.PlayerCount != 0 || g.PrizePool != 0 {
		t.Fatalf("empty game counters should reset, got player_count=%d pool=%v", g.PlayerCount, g.PrizePool)
	}
}

func TestIntegration_GameRoom_PaidLeaveRefundsOnlyOnce(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	user := h.seedUser("PaidLeaver", 204)
	h.setBalance(user, 40)
	gameID := h.seedWaitingGame()
	h.addPaidCard(gameID, user, 12, 0)
	if _, err := h.db.Exec(
		`UPDATE games SET player_count=1, prize_pool=$2 WHERE id=$1`,
		gameID, domain.BetAmountRegular*(1-domain.HouseCut),
	); err != nil {
		t.Fatalf("update game counters: %v", err)
	}

	if err := h.uc.LeaveGame(ctx, gameID, domain.LeaveGameRequest{UserID: user, CardID: 12}); err != nil {
		t.Fatalf("leave paid card: %v", err)
	}
	if h.balance(user) != 50 {
		t.Fatalf("paid leave should refund one stake: balance=%v", h.balance(user))
	}
	if refunds := h.txCountByReference(user, "GAME_REFUND"); refunds != 1 {
		t.Fatalf("expected one refund tx, got %d", refunds)
	}

	if err := h.uc.LeaveGame(ctx, gameID, domain.LeaveGameRequest{UserID: user, CardID: 12}); err == nil {
		t.Fatal("leaving the same paid card twice should fail")
	}
	if h.balance(user) != 50 {
		t.Fatalf("second leave must not refund again: balance=%v", h.balance(user))
	}
	if refunds := h.txCountByReference(user, "GAME_REFUND"); refunds != 1 {
		t.Fatalf("second leave created another refund tx, got %d", refunds)
	}
}

func TestIntegration_GameRoom_CancelRefundsOnlyPaidActiveCards(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	paidUser := h.seedUser("CancelPaid", 205)
	unpaidUser := h.seedUser("CancelUnpaid", 206)
	h.setBalance(paidUser, 40)
	h.setBalance(unpaidUser, 50)
	gameID := h.seedWaitingGame()
	h.addPaidCard(gameID, paidUser, 20, 0)
	h.addReservation(gameID, unpaidUser, 21, 1)
	if _, err := h.db.Exec(
		`UPDATE games SET state='DRAWING', player_count=2, prize_pool=$2, started_at=now() WHERE id=$1`,
		gameID, 2*domain.BetAmountRegular*(1-domain.HouseCut),
	); err != nil {
		t.Fatalf("force drawing game: %v", err)
	}

	result, err := h.uc.CancelGame(ctx, gameID)
	if err != nil {
		t.Fatalf("cancel game: %v", err)
	}
	if result.Game.State != domain.GameStateCancelled {
		t.Fatalf("cancelled game state = %s", result.Game.State)
	}
	if result.RefundedCount != 1 || result.RefundedAmount != domain.BetAmountRegular {
		t.Fatalf("refund summary = count %d amount %v", result.RefundedCount, result.RefundedAmount)
	}
	if h.balance(paidUser) != 50 {
		t.Fatalf("paid active card should be refunded: balance=%v", h.balance(paidUser))
	}
	if h.balance(unpaidUser) != 50 {
		t.Fatalf("unpaid reservation should not receive money: balance=%v", h.balance(unpaidUser))
	}
	if refunds := h.txCountByReference(paidUser, "GAME_REFUND"); refunds != 1 {
		t.Fatalf("paid user refund tx count = %d", refunds)
	}
	if refunds := h.txCountByReference(unpaidUser, "GAME_REFUND"); refunds != 0 {
		t.Fatalf("unpaid user refund tx count = %d", refunds)
	}
	if _, active := h.cardState(gameID, paidUser, 20); active {
		t.Fatal("paid card should be inactive after cancel")
	}
	if _, active := h.cardState(gameID, unpaidUser, 21); active {
		t.Fatal("unpaid card should be inactive after cancel")
	}

	if _, err := h.uc.CancelGame(ctx, gameID); err == nil {
		t.Fatal("cancelling an already-cancelled game should fail")
	}
	if h.balance(paidUser) != 50 || h.txCountByReference(paidUser, "GAME_REFUND") != 1 {
		t.Fatalf("second cancel must not double refund: balance=%v refunds=%d", h.balance(paidUser), h.txCountByReference(paidUser, "GAME_REFUND"))
	}
}
