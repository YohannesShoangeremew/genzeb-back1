package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/bingo/backend/internal/domain"
	"github.com/google/uuid"
)

type TransactionService struct {
	db               *sql.DB
	walletRepo       domain.WalletRepository
	transactionRepo  domain.TransactionRepository
	bonusRepo        domain.BonusRepository
	referralNotifier domain.BroadcastSender
}

// NewTransactionService creates a new transaction service
func NewTransactionService(
	db *sql.DB,
	walletRepo domain.WalletRepository,
	transactionRepo domain.TransactionRepository,
	bonusRepos ...domain.BonusRepository,
) *TransactionService {
	var bonusRepo domain.BonusRepository
	if len(bonusRepos) > 0 {
		bonusRepo = bonusRepos[0]
	}
	return &TransactionService{
		db:              db,
		walletRepo:      walletRepo,
		transactionRepo: transactionRepo,
		bonusRepo:       bonusRepo,
	}
}

// SetReferralNotifier wires the best-effort Telegram notice sent after a
// referral reward has committed.
func (s *TransactionService) SetReferralNotifier(notifier domain.BroadcastSender) {
	s.referralNotifier = notifier
}

// AdjustBalance credits (amount > 0) or debits (amount < 0) a user's wallet as
// a manual admin action, recording a completed transaction for the audit trail.
// Debits are rejected if they would overdraw the wallet.
func (s *TransactionService) AdjustBalance(ctx context.Context, userID uuid.UUID, amount float64, reason string) (*domain.Transaction, error) {
	if amount == 0 {
		return nil, fmt.Errorf("amount must be non-zero")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	wallet, err := s.walletRepo.LockForUpdate(ctx, tx, userID)
	if err != nil {
		return nil, fmt.Errorf("wallet not found: %w", err)
	}

	if amount < 0 && wallet.Balance+amount < 0 {
		return nil, fmt.Errorf("insufficient balance: cannot debit %.2f from %.2f", -amount, wallet.Balance)
	}

	if err := s.walletRepo.UpdateBalance(ctx, tx, userID, amount); err != nil {
		return nil, fmt.Errorf("failed to update balance: %w", err)
	}

	txType := domain.TransactionTypeDeposit
	category := domain.TransactionCategoryAdminCredit
	if amount < 0 {
		txType = domain.TransactionTypeWithdraw
		category = domain.TransactionCategoryAdminDebit
	}
	note := reason
	if note == "" {
		note = "admin balance adjustment"
	}
	record := &domain.Transaction{
		UserID:    userID,
		Type:      txType,
		Category:  category,
		Amount:    absFloat(amount),
		Status:    domain.TransactionStatusCompleted,
		Reference: &note,
	}
	if err := s.transactionRepo.Create(ctx, tx, record); err != nil {
		return nil, fmt.Errorf("failed to record adjustment: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return record, nil
}

func absFloat(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

// ApproveDeposit approves a deposit transaction and updates wallet balance
func (s *TransactionService) ApproveDeposit(ctx context.Context, transactionID uuid.UUID) (*domain.Transaction, error) {
	// Get transaction
	transaction, err := s.transactionRepo.FindByID(ctx, transactionID)
	if err != nil {
		return nil, fmt.Errorf("transaction not found: %w", err)
	}

	// Validate transaction type and status
	if transaction.Type != domain.TransactionTypeDeposit {
		return nil, fmt.Errorf("transaction is not a deposit")
	}

	if transaction.Status != domain.TransactionStatusPending {
		return nil, fmt.Errorf("transaction is not pending (current status: %s)", transaction.Status)
	}

	// Start database transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Lock wallet for update
	_, err = s.walletRepo.LockForUpdate(ctx, tx, transaction.UserID)
	if err != nil {
		return nil, fmt.Errorf("wallet not found: %w", err)
	}

	// Update wallet balance
	if err := s.walletRepo.UpdateBalance(ctx, tx, transaction.UserID, transaction.Amount); err != nil {
		return nil, fmt.Errorf("failed to update balance: %w", err)
	}

	// Update transaction status to completed
	if err := s.transactionRepo.UpdateStatus(ctx, tx, transactionID, domain.TransactionStatusCompleted); err != nil {
		return nil, fmt.Errorf("failed to update transaction status: %w", err)
	}

	// Reward only genuine Telebirr/CBE Birr deposits, after the guarded status
	// transition succeeds. Keeping it in this transaction makes the cash credit,
	// completed status, and bonus award atomic.
	eligible := transaction.TransactionType != nil &&
		(*transaction.TransactionType == domain.PaymentMethodTelebirr || *transaction.TransactionType == domain.PaymentMethodCBEBirr)
	if eligible && s.bonusRepo != nil {
		var enabled bool
		var percentage float64
		err := tx.QueryRowContext(ctx, `
			SELECT deposit_bonus_enabled, deposit_bonus_percentage::float8
			FROM app_settings WHERE id = 1
		`).Scan(&enabled, &percentage)
		if err != nil && err != sql.ErrNoRows {
			return nil, fmt.Errorf("failed to read deposit bonus settings: %w", err)
		}
		if enabled && percentage > 0 {
			rewardAmount := math.Round((transaction.Amount*percentage/100)*100) / 100
			if rewardAmount > 0 {
				if _, err := s.bonusRepo.GrantDepositOnce(ctx, tx, transactionID, transaction.UserID, rewardAmount); err != nil {
					return nil, fmt.Errorf("failed to award deposit bonus: %w", err)
				}
			}
		}
	}

	// A referral qualifies on the invited player's first successful REAL deposit,
	// regardless of payment provider. Admin credits never carry a provider method
	// through this service, so they cannot trigger a referral award.
	var rewardedReferrerID uuid.UUID
	var rewardedReferrerTelegramID int64
	var referralRewardAmount float64
	eligibleReferralDeposit := transaction.TransactionType != nil &&
		(*transaction.TransactionType == domain.PaymentMethodTelebirr ||
			*transaction.TransactionType == domain.PaymentMethodCBEBirr ||
			*transaction.TransactionType == domain.PaymentMethodMpesa)
	if eligibleReferralDeposit {
		var referrerID uuid.UUID
		var referrerTelegramID int64
		err := tx.QueryRowContext(ctx, `
			SELECT r.id, r.telegram_id
			FROM users invited
			JOIN users r ON r.id = invited.referred_by
			WHERE invited.id = $1 AND invited.referral_rewarded = false
		`, transaction.UserID).Scan(&referrerID, &referrerTelegramID)
		if err != nil && err != sql.ErrNoRows {
			return nil, fmt.Errorf("failed to inspect referral eligibility: %w", err)
		}
		if err == nil {
			referralEnabled := true
			rewardAmount := domain.ReferralRewardAmount
			err = tx.QueryRowContext(ctx, `
				SELECT referral_enabled, referral_amount::float8
				FROM app_settings WHERE id = 1
			`).Scan(&referralEnabled, &rewardAmount)
			if err != nil && err != sql.ErrNoRows {
				return nil, fmt.Errorf("failed to read referral settings: %w", err)
			}

			if referralEnabled && rewardAmount > 0 {
				if s.bonusRepo == nil {
					return nil, fmt.Errorf("referral bonus repository is unavailable")
				}
				awarded, grantErr := s.bonusRepo.GrantReferralOnce(ctx, tx, transaction.UserID, referrerID, rewardAmount)
				if grantErr != nil {
					return nil, fmt.Errorf("failed to award referral bonus: %w", grantErr)
				}
				if awarded {
					rewardedReferrerID = referrerID
					rewardedReferrerTelegramID = referrerTelegramID
					referralRewardAmount = rewardAmount
				}
			} else {
				// Turning the switch off does not postpone this first-deposit
				// trigger until a later deposit.
				if _, err := tx.ExecContext(ctx, `
					UPDATE users SET referral_rewarded = true
					WHERE id = $1 AND referred_by = $2 AND referral_rewarded = false
				`, transaction.UserID, referrerID); err != nil {
					return nil, fmt.Errorf("failed to close disabled referral eligibility: %w", err)
				}
			}
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	if rewardedReferrerTelegramID > 0 && s.referralNotifier != nil {
		message := fmt.Sprintf(
			"🎉 %0.f ብር የመጫወቻ ቦነስ አግኝተዋል!\nየጋበዙት ሰው የመጀመሪያ ገቢውን አድርጓል።\n\nYou earned a %0.f birr PLAY bonus — someone you invited made their first deposit! 🎮",
			referralRewardAmount, referralRewardAmount,
		)
		if notifyErr := s.referralNotifier.SendMessage(rewardedReferrerTelegramID, message); notifyErr != nil {
			log.Printf("[referral] rewarded %s but Telegram notice failed: %v", rewardedReferrerID, notifyErr)
		}
	}

	// Fetch updated transaction
	transaction, err = s.transactionRepo.FindByID(ctx, transactionID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch updated transaction: %w", err)
	}

	return transaction, nil
}

// RejectDeposit rejects a deposit transaction (no balance change)
func (s *TransactionService) RejectDeposit(ctx context.Context, transactionID uuid.UUID) (*domain.Transaction, error) {
	// Get transaction
	transaction, err := s.transactionRepo.FindByID(ctx, transactionID)
	if err != nil {
		return nil, fmt.Errorf("transaction not found: %w", err)
	}

	// Validate transaction type and status
	if transaction.Type != domain.TransactionTypeDeposit {
		return nil, fmt.Errorf("transaction is not a deposit")
	}

	if transaction.Status != domain.TransactionStatusPending {
		return nil, fmt.Errorf("transaction is not pending (current status: %s)", transaction.Status)
	}

	// Start database transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Update transaction status to failed (no balance change for rejected deposits)
	if err := s.transactionRepo.UpdateStatus(ctx, tx, transactionID, domain.TransactionStatusFailed); err != nil {
		return nil, fmt.Errorf("failed to update transaction status: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Fetch updated transaction
	transaction, err = s.transactionRepo.FindByID(ctx, transactionID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch updated transaction: %w", err)
	}

	return transaction, nil
}

// ApproveWithdrawal approves a withdrawal transaction (balance already subtracted)
func (s *TransactionService) ApproveWithdrawal(ctx context.Context, transactionID uuid.UUID) (*domain.Transaction, error) {
	// Get transaction
	transaction, err := s.transactionRepo.FindByID(ctx, transactionID)
	if err != nil {
		return nil, fmt.Errorf("transaction not found: %w", err)
	}

	// Validate transaction type and status
	if transaction.Type != domain.TransactionTypeWithdraw {
		return nil, fmt.Errorf("transaction is not a withdrawal")
	}

	if transaction.Status != domain.TransactionStatusPending {
		return nil, fmt.Errorf("transaction is not pending (current status: %s)", transaction.Status)
	}

	// Start database transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Update transaction status to completed (balance already subtracted when withdrawal was created)
	if err := s.transactionRepo.UpdateStatus(ctx, tx, transactionID, domain.TransactionStatusCompleted); err != nil {
		return nil, fmt.Errorf("failed to update transaction status: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Fetch updated transaction
	transaction, err = s.transactionRepo.FindByID(ctx, transactionID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch updated transaction: %w", err)
	}

	return transaction, nil
}

// RejectWithdrawal rejects a withdrawal transaction and refunds the balance
func (s *TransactionService) RejectWithdrawal(ctx context.Context, transactionID uuid.UUID) (*domain.Transaction, error) {
	// Get transaction
	transaction, err := s.transactionRepo.FindByID(ctx, transactionID)
	if err != nil {
		return nil, fmt.Errorf("transaction not found: %w", err)
	}

	// Validate transaction type and status
	if transaction.Type != domain.TransactionTypeWithdraw {
		return nil, fmt.Errorf("transaction is not a withdrawal")
	}

	if transaction.Status != domain.TransactionStatusPending {
		return nil, fmt.Errorf("transaction is not pending (current status: %s)", transaction.Status)
	}

	// Start database transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Lock wallet for update
	_, err = s.walletRepo.LockForUpdate(ctx, tx, transaction.UserID)
	if err != nil {
		return nil, fmt.Errorf("wallet not found: %w", err)
	}

	// Refund balance (add it back)
	if err := s.walletRepo.UpdateBalance(ctx, tx, transaction.UserID, transaction.Amount); err != nil {
		return nil, fmt.Errorf("failed to refund balance: %w", err)
	}

	// Update transaction status to failed
	if err := s.transactionRepo.UpdateStatus(ctx, tx, transactionID, domain.TransactionStatusFailed); err != nil {
		return nil, fmt.Errorf("failed to update transaction status: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Fetch updated transaction
	transaction, err = s.transactionRepo.FindByID(ctx, transactionID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch updated transaction: %w", err)
	}

	return transaction, nil
}

// CancelTransaction cancels a pending transaction
func (s *TransactionService) CancelTransaction(ctx context.Context, transactionID uuid.UUID) (*domain.Transaction, error) {
	// Get transaction
	transaction, err := s.transactionRepo.FindByID(ctx, transactionID)
	if err != nil {
		return nil, fmt.Errorf("transaction not found: %w", err)
	}

	// Validate status
	if transaction.Status != domain.TransactionStatusPending {
		return nil, fmt.Errorf("transaction is not pending (current status: %s)", transaction.Status)
	}

	// Start database transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// If it's a withdrawal, refund the balance
	if transaction.Type == domain.TransactionTypeWithdraw {
		// Lock wallet for update
		_, err = s.walletRepo.LockForUpdate(ctx, tx, transaction.UserID)
		if err != nil {
			return nil, fmt.Errorf("wallet not found: %w", err)
		}

		// Refund balance
		if err := s.walletRepo.UpdateBalance(ctx, tx, transaction.UserID, transaction.Amount); err != nil {
			return nil, fmt.Errorf("failed to refund balance: %w", err)
		}
	}

	// Update transaction status to cancelled
	if err := s.transactionRepo.UpdateStatus(ctx, tx, transactionID, domain.TransactionStatusCancelled); err != nil {
		return nil, fmt.Errorf("failed to update transaction status: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Fetch updated transaction
	transaction, err = s.transactionRepo.FindByID(ctx, transactionID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch updated transaction: %w", err)
	}

	return transaction, nil
}

// ProcessWithdrawal processes a withdrawal request (subtracts balance and creates transaction)
func (s *TransactionService) ProcessWithdrawal(ctx context.Context, userID uuid.UUID, amount float64, accountNumber string, accountType domain.PaymentMethod) (*domain.Transaction, error) {
	// Start database transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Lock wallet for update
	wallet, err := s.walletRepo.LockForUpdate(ctx, tx, userID)
	if err != nil {
		return nil, fmt.Errorf("wallet not found: %w", err)
	}

	// Check if user has at least one completed *real* cash-in deposit. Genuine
	// deposits have a NULL reference; system credits that share the deposit type
	// (game prizes "GAME_PRIZE", refunds "GAME_REFUND", admin balance adjustments)
	// all carry a reference, so `reference IS NULL` excludes them — a player who
	// only ever won/was-refunded/was-credited still can't withdraw without
	// having put real money in.
	var depositCount int
	checkDepositQuery := `
		SELECT COUNT(*)
		FROM transactions
		WHERE user_id = $1
		  AND type = $2
		  AND status = $3
		  AND reference IS NULL
	`
	err = tx.QueryRowContext(ctx, checkDepositQuery, userID, domain.TransactionTypeDeposit, domain.TransactionStatusCompleted).Scan(&depositCount)
	if err != nil {
		return nil, fmt.Errorf("failed to check deposit history: %w", err)
	}

	if depositCount == 0 {
		return nil, fmt.Errorf("withdrawal not allowed: user must have at least one completed deposit")
	}

	// Check balance
	if wallet.Balance < amount {
		return nil, fmt.Errorf("insufficient balance")
	}

	// Remaining balance floor: the player must keep at least this much.
	remainingBalance := wallet.Balance - amount
	if remainingBalance < domain.MinBalanceAfterWithdrawal {
		return nil, fmt.Errorf("withdrawal not allowed: remaining balance must be at least %.0f", domain.MinBalanceAfterWithdrawal)
	}

	// Subtract balance immediately
	if err := s.walletRepo.UpdateBalance(ctx, tx, userID, -amount); err != nil {
		return nil, fmt.Errorf("failed to update balance: %w", err)
	}

	// Create transaction with pending status
	transaction := &domain.Transaction{
		UserID:          userID,
		Type:            domain.TransactionTypeWithdraw,
		Category:        domain.TransactionCategoryWithdrawal,
		Amount:          amount,
		Status:          domain.TransactionStatusPending,
		TransactionType: &accountType,
		TransactionID:   &accountNumber,
	}

	if err := s.transactionRepo.Create(ctx, tx, transaction); err != nil {
		return nil, fmt.Errorf("failed to create withdrawal transaction: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return transaction, nil
}

// ProcessTransfer processes a transfer between two users
func (s *TransactionService) ProcessTransfer(ctx context.Context, senderID, receiverID uuid.UUID, amount float64) (*domain.Transaction, *domain.Transaction, error) {
	// Start database transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Lock both wallets in a consistent order (by UUID) to avoid deadlocks when
	// two reciprocal transfers (A->B and B->A) run concurrently.
	firstID, secondID := senderID, receiverID
	if receiverID.String() < senderID.String() {
		firstID, secondID = receiverID, senderID
	}
	firstWallet, err := s.walletRepo.LockForUpdate(ctx, tx, firstID)
	if err != nil {
		return nil, nil, fmt.Errorf("wallet not found: %w", err)
	}
	secondWallet, err := s.walletRepo.LockForUpdate(ctx, tx, secondID)
	if err != nil {
		return nil, nil, fmt.Errorf("wallet not found: %w", err)
	}

	// Identify the sender's wallet for the balance check.
	senderWallet := firstWallet
	if firstID != senderID {
		senderWallet = secondWallet
	}

	// Check sender balance
	if senderWallet.Balance < amount {
		return nil, nil, fmt.Errorf("insufficient balance")
	}

	// Subtract from sender
	if err := s.walletRepo.UpdateBalance(ctx, tx, senderID, -amount); err != nil {
		return nil, nil, fmt.Errorf("failed to update sender balance: %w", err)
	}

	// Add to receiver
	if err := s.walletRepo.UpdateBalance(ctx, tx, receiverID, amount); err != nil {
		return nil, nil, fmt.Errorf("failed to update receiver balance: %w", err)
	}

	// Create transfer_out transaction for sender
	receiverIDStr := receiverID.String()
	senderTransaction := &domain.Transaction{
		UserID:    senderID,
		Type:      domain.TransactionTypeTransferOut,
		Category:  domain.TransactionCategoryTransferOut,
		Amount:    amount,
		Status:    domain.TransactionStatusCompleted,
		Reference: &receiverIDStr,
	}

	if err := s.transactionRepo.Create(ctx, tx, senderTransaction); err != nil {
		return nil, nil, fmt.Errorf("failed to create sender transaction: %w", err)
	}

	// Create transfer_in transaction for receiver
	senderIDStr := senderID.String()
	receiverTransaction := &domain.Transaction{
		UserID:    receiverID,
		Type:      domain.TransactionTypeTransferIn,
		Category:  domain.TransactionCategoryTransferIn,
		Amount:    amount,
		Status:    domain.TransactionStatusCompleted,
		Reference: &senderIDStr,
	}

	if err := s.transactionRepo.Create(ctx, tx, receiverTransaction); err != nil {
		return nil, nil, fmt.Errorf("failed to create receiver transaction: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return senderTransaction, receiverTransaction, nil
}

// ethiopianDayStart returns midnight of the current Ethiopian calendar day
// (UTC+3), expressed in the SAME wall-clock frame that timestamps are written
// in — which is why it takes `local` rather than assuming one.
//
// transactions.created_at is `timestamp without time zone` and is written by
// the application (transactionRepository.Create uses time.Now()), so the driver
// discards the offset and the column stores the app's LOCAL wall clock. A
// day-boundary computed in a different frame than the stored values silently
// slides the window.
//
// This previously ended in .UTC(), which is correct only while the app process
// happens to run in UTC — true in production today purely because the runtime
// image is bare alpine with no tzdata and no TZ set, so Go falls back to UTC.
// Setting TZ, or adding tzdata for some unrelated reason, would have shifted
// the daily withdrawal window by three hours with nothing to indicate it: the
// window would have started at 21:00 the previous evening, so a player's late
// withdrawals from yesterday would count against today's cap and block them
// early. Converting into `local` instead is correct whatever the process
// timezone is, and makes a development machine behave like production.
func ethiopianDayStart(now time.Time, local *time.Location) time.Time {
	eat := time.FixedZone("EAT", 3*60*60)
	nowEAT := now.In(eat)
	midnightEAT := time.Date(nowEAT.Year(), nowEAT.Month(), nowEAT.Day(), 0, 0, 0, 0, eat)
	return midnightEAT.In(local)
}
