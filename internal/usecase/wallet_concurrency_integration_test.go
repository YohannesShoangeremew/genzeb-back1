//go:build integration

package usecase

import (
	"context"
	"sync"
	"testing"

	"github.com/bingo/backend/internal/domain"
	"github.com/bingo/backend/internal/repository/postgres"
	"github.com/google/uuid"
)

// These cover the one thing the rest of the suite does not: what the money
// paths do under actual contention. ProcessWithdrawal takes a FOR UPDATE row
// lock before reading the balance, so concurrent withdrawals are supposed to
// serialize and each see the previous one's debit. Nothing pinned that, which
// meant a future refactor could drop the lock (or move the balance read before
// it) and silently reopen a drain-past-the-floor hole.
//
// The scenario modelled: a player who has just been credited a prize fires many
// withdrawals at once, hoping two of them both read the pre-debit balance and
// both pass MinBalanceAfterWithdrawal.

// withdrawalTestPhone is a valid Ethiopian mobile. seedUser's generated phone is
// not one (it is "0900"+telegram_id, far too long), so the payout destination
// must be supplied explicitly or Withdraw rejects before reaching the money path.
const withdrawalTestPhone = "0912345678"

// concurrentWithdrawals fires n simultaneous withdrawals of `each` birr and
// reports how many were accepted. All goroutines are released from a single
// barrier so they contend on the wallet row as tightly as the runtime allows.
func concurrentWithdrawals(t *testing.T, uc *WalletUseCase, userID uuid.UUID, n int, each float64) (ok int, errs []string) {
	t.Helper()
	var (
		start sync.WaitGroup
		done  sync.WaitGroup
		mu    sync.Mutex
	)
	start.Add(1)
	for i := 0; i < n; i++ {
		done.Add(1)
		go func() {
			defer done.Done()
			start.Wait() // release all at once
			_, err := uc.Withdraw(context.Background(), domain.WithdrawRequest{
				UserID:        userID,
				Amount:        each,
				AccountNumber: withdrawalTestPhone,
				AccountType:   domain.PaymentMethodTelebirr,
			})
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				ok++
			} else {
				errs = append(errs, err.Error())
			}
		}()
	}
	start.Done()
	done.Wait()
	return ok, errs
}

// A single large withdrawal repeated concurrently must not let a second one
// through on a stale read. Balance 200, floor 50: one withdrawal of 100 leaves
// 100 and is legal; a second would leave 0. Note the second is blocked by the
// FLOOR, not by insufficient balance (100 >= 100), so this exercises exactly
// the check the player would be trying to skip.
func TestIntegration_Withdraw_ConcurrentCannotBreachMinBalance(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()

	userID := h.seedUser("RaceOne", 7701)
	h.addCompletedDeposit(userID, 200, "RACE1") // withdrawals require a real deposit
	h.setBalance(userID, 200)

	ok, errs := concurrentWithdrawals(t, h.walletUC(), userID, 8, 100)

	if ok != 1 {
		t.Fatalf("expected exactly 1 of 8 concurrent withdrawals to succeed, got %d (errors: %v)", ok, errs)
	}
	if got := h.balance(userID); got != 100 {
		t.Fatalf("balance after concurrent withdrawals = %.2f, want 100", got)
	}
	if got := h.balance(userID); got < domain.MinBalanceAfterWithdrawal {
		t.Fatalf("balance %.2f fell below the %.0f floor", got, domain.MinBalanceAfterWithdrawal)
	}
}

// The subtler drain: instead of one big withdrawal, fire many smaller ones that
// each individually pass the floor check but collectively would not. With
// balance 200, a floor of 50 and a 50-birr minimum request, at most 150 is
// withdrawable, so at most 3 of ten requests may be accepted (3*50=150 leaves
// exactly the floor; a 4th would leave 0). If the lock were dropped, several
// would read 200 concurrently and all pass, draining below the floor.
func TestIntegration_Withdraw_ConcurrentSmallDrainsRespectFloor(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()

	userID := h.seedUser("RaceMany", 7702)
	h.addCompletedDeposit(userID, 200, "RACE2")
	h.setBalance(userID, 200)

	each := domain.MinWithdrawalAmount // smallest a player may request
	ok, errs := concurrentWithdrawals(t, h.walletUC(), userID, 10, each)

	maxAllowed := int((200 - domain.MinBalanceAfterWithdrawal) / each)
	if ok > maxAllowed {
		t.Fatalf("%d of 10 concurrent %.0f-birr withdrawals succeeded, at most %d may (errors: %v)", ok, each, maxAllowed, errs)
	}

	final := h.balance(userID)
	if final < domain.MinBalanceAfterWithdrawal {
		t.Fatalf("balance drained to %.2f, below the %.0f floor — concurrent withdrawals bypassed the check", final, domain.MinBalanceAfterWithdrawal)
	}
	// The debits must reconcile exactly with the accepted count: no lost update,
	// no double debit.
	if want := 200 - float64(ok)*each; final != want {
		t.Fatalf("balance = %.2f but %d accepted withdrawals of %.0f imply %.2f — debits did not reconcile", final, ok, each, want)
	}
	// And the ledger must agree with the wallet.
	var pending float64
	if err := h.db.QueryRow(
		`SELECT COALESCE(SUM(amount),0)::float8 FROM transactions
		 WHERE user_id=$1 AND type='withdraw' AND status='pending'`, userID,
	).Scan(&pending); err != nil {
		t.Fatalf("sum withdrawals: %v", err)
	}
	if pending != float64(ok)*each {
		t.Fatalf("ledger shows %.2f withdrawn but %d requests were accepted (%.2f)", pending, ok, float64(ok)*each)
	}
}

// Rejecting a withdrawal refunds it, so a double reject would double-credit —
// an admin double-clicking the button, or a client retry on a slow response.
// The guard is inside transactionRepository.UpdateStatus, whose UPDATE is
// conditional on the row still being 'pending'; the loser updates 0 rows,
// errors, and rolls back its refund with it. The pre-check in RejectWithdrawal
// happens before BeginTx and cannot be the thing protecting this, so it is
// worth pinning that the conditional UPDATE really is load-bearing.
func TestIntegration_RejectWithdrawal_ConcurrentRefundsOnlyOnce(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()

	userID := h.seedUser("RejectRace", 7704)
	h.addCompletedDeposit(userID, 200, "RACE4")
	h.setBalance(userID, 200)

	withdrawal, err := h.walletUC().Withdraw(context.Background(), domain.WithdrawRequest{
		UserID:        userID,
		Amount:        100,
		AccountNumber: withdrawalTestPhone,
		AccountType:   domain.PaymentMethodTelebirr,
	})
	if err != nil {
		t.Fatalf("seed withdrawal: %v", err)
	}
	if got := h.balance(userID); got != 100 {
		t.Fatalf("balance after withdrawal = %.2f, want 100", got)
	}

	svc := postgres.NewTransactionService(h.db, postgres.NewWalletRepository(h.db), postgres.NewTransactionRepository(h.db))

	var (
		start sync.WaitGroup
		done  sync.WaitGroup
		mu    sync.Mutex
		ok    int
	)
	start.Add(1)
	for i := 0; i < 5; i++ {
		done.Add(1)
		go func() {
			defer done.Done()
			start.Wait()
			if _, err := svc.RejectWithdrawal(context.Background(), withdrawal.ID); err == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	start.Done()
	done.Wait()

	if ok != 1 {
		t.Fatalf("%d of 5 concurrent rejects succeeded, want exactly 1 — the refund was applied more than once", ok)
	}
	if got := h.balance(userID); got != 200 {
		t.Fatalf("balance = %.2f after concurrent rejects, want 200 (refunded exactly once)", got)
	}
}

// The scenario as described: a prize lands and the player immediately tries to
// take the whole balance out. The prize itself is not withdrawable cash (only
// real deposits unlock withdrawals) but it does inflate the balance, so the
// floor is the thing standing between the player and a full drain.
func TestIntegration_Withdraw_AfterPrizeStillRespectsFloor(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()

	userID := h.seedUser("WinnerRace", 7703)
	h.addCompletedDeposit(userID, 60, "RACE3") // the real money they put in
	// Prize credited on top, exactly as finalizeWinners would.
	h.addSystemDeposit(userID, domain.TransactionCategoryWinnings, 500, "game-prize-race")
	h.setBalance(userID, 560)

	// Try to take everything, several ways at once.
	ok, errs := concurrentWithdrawals(t, h.walletUC(), userID, 6, 560)
	if ok != 0 {
		t.Fatalf("a full-balance withdrawal was accepted %d times; the floor should reject all (errors: %v)", ok, errs)
	}
	if got := h.balance(userID); got != 560 {
		t.Fatalf("balance changed to %.2f despite every withdrawal being rejected", got)
	}

	// Now the largest legal amount (560 - 50 = 510), fired concurrently.
	ok, errs = concurrentWithdrawals(t, h.walletUC(), userID, 6, 510)
	if ok != 1 {
		t.Fatalf("expected exactly 1 of 6 max-legal withdrawals to succeed, got %d (errors: %v)", ok, errs)
	}
	if got := h.balance(userID); got != domain.MinBalanceAfterWithdrawal {
		t.Fatalf("balance = %.2f, want exactly the %.0f floor", got, domain.MinBalanceAfterWithdrawal)
	}
}
