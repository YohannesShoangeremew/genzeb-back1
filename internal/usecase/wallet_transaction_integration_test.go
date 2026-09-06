//go:build integration

package usecase

import (
	"context"
	"strings"
	"testing"

	"github.com/bingo/backend/internal/domain"
	"github.com/bingo/backend/internal/repository/postgres"
	"github.com/google/uuid"
)

func (h *harness) walletUC() *WalletUseCase {
	h.t.Helper()
	return NewWalletUseCase(
		postgres.NewWalletRepository(h.db),
		postgres.NewTransactionRepository(h.db),
		postgres.NewUserRepository(h.db),
		postgres.NewGameRepository(h.db),
		postgres.NewBonusRepository(h.db),
		h.db,
		nil,
		nil,
	)
}

func (h *harness) addCompletedDeposit(userID uuid.UUID, amount float64, refPrefix string) uuid.UUID {
	h.t.Helper()
	id := uuid.New()
	txID := strings.ToUpper(refPrefix) + "-" + id.String()
	method := string(domain.PaymentMethodTelebirr)
	_, err := h.db.Exec(
		`INSERT INTO transactions (id, user_id, type, category, amount, status, transaction_type, transaction_id)
		 VALUES ($1,$2,'deposit','deposit',$3,'completed',$4,$5)`,
		id, userID, amount, method, txID,
	)
	if err != nil {
		h.t.Fatalf("add completed deposit: %v", err)
	}
	return id
}

func (h *harness) addSystemDeposit(userID uuid.UUID, category domain.TransactionCategory, amount float64, reference string) uuid.UUID {
	h.t.Helper()
	id := uuid.New()
	_, err := h.db.Exec(
		`INSERT INTO transactions (id, user_id, type, category, amount, status, reference)
		 VALUES ($1,$2,'deposit',$3,$4,'completed',$5)`,
		id, userID, category, amount, reference,
	)
	if err != nil {
		h.t.Fatalf("add system deposit: %v", err)
	}
	return id
}

func TestIntegration_Transactions_TransferAtomicityAndAuditRows(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()
	walletUC := h.walletUC()

	sender := h.seedUser("TransferSender", 101)
	receiver := h.seedUser("TransferReceiver", 102)
	h.setBalance(sender, 100)
	h.setBalance(receiver, 5)

	out, in, err := walletUC.Transfer(ctx, domain.TransferRequest{
		SenderID:   sender,
		ReceiverID: receiver,
		Amount:     35,
	})
	if err != nil {
		t.Fatalf("transfer: %v", err)
	}
	if h.balance(sender) != 65 || h.balance(receiver) != 40 {
		t.Fatalf("balances after transfer: sender=%v receiver=%v", h.balance(sender), h.balance(receiver))
	}
	if out.Type != domain.TransactionTypeTransferOut || out.Category != domain.TransactionCategoryTransferOut || out.Status != domain.TransactionStatusCompleted {
		t.Fatalf("sender transaction audit fields wrong: %#v", out)
	}
	if in.Type != domain.TransactionTypeTransferIn || in.Category != domain.TransactionCategoryTransferIn || in.Status != domain.TransactionStatusCompleted {
		t.Fatalf("receiver transaction audit fields wrong: %#v", in)
	}
	if out.Reference == nil || *out.Reference != receiver.String() {
		t.Fatalf("sender reference should point to receiver, got %v", out.Reference)
	}
	if in.Reference == nil || *in.Reference != sender.String() {
		t.Fatalf("receiver reference should point to sender, got %v", in.Reference)
	}

	if _, _, err := walletUC.Transfer(ctx, domain.TransferRequest{
		SenderID:   sender,
		ReceiverID: receiver,
		Amount:     66,
	}); err == nil {
		t.Fatal("overdraft transfer should fail")
	}
	if h.balance(sender) != 65 || h.balance(receiver) != 40 {
		t.Fatalf("failed transfer must not move money: sender=%v receiver=%v", h.balance(sender), h.balance(receiver))
	}
}

func TestIntegration_Transactions_WithdrawalRequiresRealDepositAndRefundsOnReject(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()
	walletUC := h.walletUC()

	user := h.seedUser("Withdrawer", 103)
	h.setBalance(user, 100)

	_, err := walletUC.Withdraw(ctx, domain.WithdrawRequest{
		UserID:      user,
		Amount:      domain.MinWithdrawalAmount,
		AccountType: domain.PaymentMethodTelebirr,
	})
	if err == nil {
		t.Fatal("withdrawal without a real completed deposit should fail")
	}
	if h.balance(user) != 100 {
		t.Fatalf("failed withdrawal changed balance: %v", h.balance(user))
	}

	h.addSystemDeposit(user, domain.TransactionCategoryWinnings, 90, "GAME_PRIZE")
	_, err = walletUC.Withdraw(ctx, domain.WithdrawRequest{
		UserID:      user,
		Amount:      domain.MinWithdrawalAmount,
		AccountType: domain.PaymentMethodTelebirr,
	})
	if err == nil {
		t.Fatal("game winnings should not satisfy the real-deposit withdrawal gate")
	}

	h.addCompletedDeposit(user, 100, "real")
	if _, err := walletUC.Withdraw(ctx, domain.WithdrawRequest{
		UserID:      user,
		Amount:      domain.MinWithdrawalAmount - 1,
		AccountType: domain.PaymentMethodTelebirr,
	}); err == nil {
		t.Fatal("below-minimum withdrawal should fail")
	}
	if _, err := walletUC.Withdraw(ctx, domain.WithdrawRequest{
		UserID:      user,
		Amount:      51,
		AccountType: domain.PaymentMethodTelebirr,
	}); err == nil {
		t.Fatal("withdrawal that leaves less than the balance floor should fail")
	}

	pending, err := walletUC.Withdraw(ctx, domain.WithdrawRequest{
		UserID:        user,
		Amount:        50,
		AccountNumber: "0912345678",
		AccountType:   domain.PaymentMethodTelebirr,
	})
	if err != nil {
		t.Fatalf("valid withdrawal: %v", err)
	}
	if h.balance(user) != 50 {
		t.Fatalf("withdrawal should reserve funds immediately: balance=%v", h.balance(user))
	}
	if pending.Status != domain.TransactionStatusPending || pending.Category != domain.TransactionCategoryWithdrawal {
		t.Fatalf("withdrawal audit fields wrong: %#v", pending)
	}

	rejected, err := walletUC.RejectWithdrawal(ctx, pending.ID)
	if err != nil {
		t.Fatalf("reject withdrawal: %v", err)
	}
	if rejected.Status != domain.TransactionStatusFailed {
		t.Fatalf("rejected status = %s", rejected.Status)
	}
	if h.balance(user) != 100 {
		t.Fatalf("rejected withdrawal should refund reserved funds: balance=%v", h.balance(user))
	}
	if _, err := walletUC.RejectWithdrawal(ctx, pending.ID); err == nil {
		t.Fatal("rejecting an already failed withdrawal should fail")
	}
	if h.balance(user) != 100 {
		t.Fatalf("second reject must not refund again: balance=%v", h.balance(user))
	}
}

func TestIntegration_Transactions_GameReferencesExcludedFromCashHistories(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()
	walletUC := h.walletUC()

	user := h.seedUser("HistoryFilter", 105)
	realID := h.addCompletedDeposit(user, 25, "history")
	h.addSystemDeposit(user, domain.TransactionCategoryRefund, 10, "GAME_REFUND")
	h.addSystemDeposit(user, domain.TransactionCategoryWinnings, 16, "GAME_PRIZE")

	deposits, err := walletUC.GetDepositHistory(ctx, user, 10)
	if err != nil {
		t.Fatalf("deposit history: %v", err)
	}
	if len(deposits) != 1 {
		t.Fatalf("cash deposit history should exclude game refs: got %d rows", len(deposits))
	}
	if deposits[0].ID != realID || deposits[0].Reference != nil {
		t.Fatalf("expected only the real cash deposit, got %#v", deposits[0])
	}
}
