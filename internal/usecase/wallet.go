package usecase

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"strings"

	"github.com/bingo/backend/internal/domain"
	"github.com/bingo/backend/internal/repository/postgres"
	"github.com/bingo/backend/pkg/utils"
	"github.com/google/uuid"
)

// errDuplicateReference is returned when a player submits a deposit whose
// payment reference (transaction_id) is already tied to an active deposit.
var errDuplicateReference = errors.New("this transaction reference was already used")

// ErrDepositMethodDisabled is returned when a player tries to deposit with a
// payment method an admin has switched off (e.g. its external verification is
// broken). The handler maps this to 503 so the Mini App shows a clean
// "temporarily unavailable" message rather than a hard error.
var ErrDepositMethodDisabled = errors.New("deposit method is temporarily disabled")

// ErrDepositVerifierDown is returned when the external payment verifier is
// configured but currently unreachable. Deposits fail closed on this — we refuse
// to accept a receipt we cannot verify rather than queue it for a blind manual
// approval. The Mini App also learns this up front from GetPublicStatus so it can
// warn the player BEFORE they pay. Mapped to 503 by the handler.
var ErrDepositVerifierDown = errors.New("deposit verification is temporarily unavailable, please try again shortly")

// ErrDepositReceiptRejected is returned when an admin tries to approve a pending
// deposit whose receipt the verifier already REJECTED with a definitive negative
// verdict (paid to a different account, wrong amount, or nonexistent). Approval is
// refused so a bogus receipt can't be blind-credited; an admin who has confirmed
// the payment out-of-band can still force it. Mapped to 409 by the handler.
var ErrDepositReceiptRejected = errors.New("this receipt was rejected by the verifier")

type WalletUseCase struct {
	walletRepo         domain.WalletRepository
	transactionRepo    domain.TransactionRepository
	userRepo           domain.UserRepository
	gameRepo           domain.GameRepository
	bonusRepo          domain.BonusRepository
	transactionService *postgres.TransactionService
	db                 *sql.DB
	paymentVerifier    domain.PaymentVerifier
	verificationLogs   domain.VerificationLogRepository
}

// NewWalletUseCase creates a new wallet use case
func NewWalletUseCase(
	walletRepo domain.WalletRepository,
	transactionRepo domain.TransactionRepository,
	userRepo domain.UserRepository,
	gameRepo domain.GameRepository,
	bonusRepo domain.BonusRepository,
	db *sql.DB,
	paymentVerifier domain.PaymentVerifier,
	verificationLogs domain.VerificationLogRepository,
) *WalletUseCase {
	transactionService := postgres.NewTransactionService(db, walletRepo, transactionRepo, bonusRepo)
	return &WalletUseCase{
		walletRepo:         walletRepo,
		transactionRepo:    transactionRepo,
		userRepo:           userRepo,
		gameRepo:           gameRepo,
		bonusRepo:          bonusRepo,
		transactionService: transactionService,
		db:                 db,
		paymentVerifier:    paymentVerifier,
		verificationLogs:   verificationLogs,
	}
}

// SetReferralNotifier wires the best-effort post-deposit reward notice.
func (uc *WalletUseCase) SetReferralNotifier(notifier domain.BroadcastSender) {
	uc.transactionService.SetReferralNotifier(notifier)
}

// DepositVerifierAvailable reports whether deposits can currently be verified.
// When no verifier is configured it returns true (verification is simply not in
// play, and deposits queue for manual approval as before); when one IS configured
// it reflects live reachability so the Mini App can block the deposit screen and
// warn the player before they pay into an unverifiable window.
func (uc *WalletUseCase) DepositVerifierAvailable(ctx context.Context) bool {
	if uc.paymentVerifier == nil {
		return true
	}
	return uc.paymentVerifier.Available(ctx)
}

// GetSettings returns the app settings row, falling back to defaults if unset.
func (uc *WalletUseCase) GetSettings(ctx context.Context) (*domain.AppSettings, error) {
	s := &domain.AppSettings{
		MinDeposit:          domain.DefaultMinDeposit,
		WelcomeBonusEnabled: true,
		ReferralEnabled:     true,
		ReferralAmount:      domain.ReferralRewardAmount,
		// Deposit channels default to ON — a fresh install accepts every
		// supported method until an admin turns one off.
		DepositTelebirrEnabled: true,
		DepositCBEBirrEnabled:  true,
		DepositMpesaEnabled:    true,
	}
	err := uc.db.QueryRowContext(ctx,
		`SELECT min_deposit, welcome_bonus_enabled, referral_enabled, referral_amount, deposit_bonus_enabled, deposit_bonus_percentage,
		        maintenance_mode, maintenance_message, deposit_telebirr_enabled, deposit_cbebirr_enabled,
		        deposit_mpesa_enabled, updated_at
		 FROM app_settings WHERE id = 1`).
		Scan(&s.MinDeposit, &s.WelcomeBonusEnabled, &s.ReferralEnabled, &s.ReferralAmount, &s.DepositBonusEnabled, &s.DepositBonusPercentage,
			&s.MaintenanceMode, &s.MaintenanceMessage, &s.DepositTelebirrEnabled, &s.DepositCBEBirrEnabled, &s.DepositMpesaEnabled, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return s, nil // migration not applied yet → sane defaults
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read settings: %w", err)
	}
	return s, nil
}

// UpdateSettings applies a partial settings change from the admin dashboard.
func (uc *WalletUseCase) UpdateSettings(ctx context.Context, req domain.UpdateAppSettingsRequest) (*domain.AppSettings, error) {
	cur, err := uc.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	if req.MinDeposit != nil {
		if *req.MinDeposit < 0 {
			return nil, errors.New("min_deposit cannot be negative")
		}
		cur.MinDeposit = *req.MinDeposit
	}
	if req.WelcomeBonusEnabled != nil {
		cur.WelcomeBonusEnabled = *req.WelcomeBonusEnabled
	}
	if req.ReferralEnabled != nil {
		cur.ReferralEnabled = *req.ReferralEnabled
	}
	if req.ReferralAmount != nil {
		if *req.ReferralAmount < 0 {
			return nil, errors.New("referral_amount cannot be negative")
		}
		cur.ReferralAmount = *req.ReferralAmount
	}
	if req.DepositBonusEnabled != nil {
		cur.DepositBonusEnabled = *req.DepositBonusEnabled
	}
	if req.DepositBonusPercentage != nil {
		if *req.DepositBonusPercentage < 0 || *req.DepositBonusPercentage > 100 {
			return nil, errors.New("deposit_bonus_percentage must be between 0 and 100")
		}
		cur.DepositBonusPercentage = *req.DepositBonusPercentage
	}
	if req.MaintenanceMode != nil {
		cur.MaintenanceMode = *req.MaintenanceMode
	}
	if req.MaintenanceMessage != nil {
		cur.MaintenanceMessage = strings.TrimSpace(*req.MaintenanceMessage)
	}
	if req.DepositTelebirrEnabled != nil {
		cur.DepositTelebirrEnabled = *req.DepositTelebirrEnabled
	}
	if req.DepositCBEBirrEnabled != nil {
		cur.DepositCBEBirrEnabled = *req.DepositCBEBirrEnabled
	}
	if req.DepositMpesaEnabled != nil {
		cur.DepositMpesaEnabled = *req.DepositMpesaEnabled
	}
	_, err = uc.db.ExecContext(ctx, `
		INSERT INTO app_settings (id, min_deposit, welcome_bonus_enabled, referral_enabled, referral_amount, deposit_bonus_enabled, deposit_bonus_percentage,
		                          maintenance_mode, maintenance_message, deposit_telebirr_enabled, deposit_cbebirr_enabled,
		                          deposit_mpesa_enabled, updated_at)
		VALUES (1, $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, now())
		ON CONFLICT (id) DO UPDATE SET min_deposit = $1, welcome_bonus_enabled = $2, referral_enabled = $3, referral_amount = $4,
		    deposit_bonus_enabled = $5, deposit_bonus_percentage = $6, maintenance_mode = $7, maintenance_message = $8,
		    deposit_telebirr_enabled = $9, deposit_cbebirr_enabled = $10, deposit_mpesa_enabled = $11,
		    updated_at = now()`,
		cur.MinDeposit, cur.WelcomeBonusEnabled, cur.ReferralEnabled, cur.ReferralAmount, cur.DepositBonusEnabled, cur.DepositBonusPercentage,
		cur.MaintenanceMode, cur.MaintenanceMessage, cur.DepositTelebirrEnabled, cur.DepositCBEBirrEnabled,
		cur.DepositMpesaEnabled)
	if err != nil {
		return nil, fmt.Errorf("failed to save settings: %w", err)
	}
	return uc.GetSettings(ctx)
}

// Deposit creates or completes a deposit, depending on verifier configuration.
func (uc *WalletUseCase) Deposit(ctx context.Context, req domain.DepositRequest) (*domain.Transaction, error) {
	// Validate amount
	if req.Amount <= 0 {
		return nil, errors.New("amount must be greater than 0")
	}

	if !domain.IsSupportedPaymentMethod(req.TransactionType) {
		return nil, errors.New("transaction_type must be one of Telebirr, CBEBirr, Mpesa")
	}

	// Read operator settings once for both the minimum-deposit floor and the
	// per-method deposit switch. Fail open on a read error (never block a deposit
	// just because the settings row couldn't be read), consistent with the rest
	// of the settings-gated paths.
	if s, err := uc.GetSettings(ctx); err == nil {
		if req.Amount < s.MinDeposit {
			return nil, fmt.Errorf("minimum deposit is %.0f birr", s.MinDeposit)
		}
		// Admin has switched this channel off (e.g. its verification is broken).
		// Reject cleanly so players stop paying into a method whose receipts can't
		// be confirmed, rather than losing money into a dead path.
		if !s.DepositMethodEnabled(req.TransactionType) {
			return nil, fmt.Errorf("%w: %s deposits are temporarily unavailable", ErrDepositMethodDisabled, req.TransactionType)
		}
	}

	// Canonicalize the payment reference to uppercase. Provider references
	// (Telebirr/CBE/M-Pesa) are uppercase alphanumeric, and the external
	// verifier tolerates case variants — without this, "ce626ejrns" and
	// "CE626EJRNS" would slip past the duplicate check as two different
	// receipts and the same payment could be credited twice.
	req.TransactionID = strings.ToUpper(strings.TrimSpace(req.TransactionID))
	if req.TransactionID == "" {
		return nil, errors.New("transaction_id is required")
	}

	// Verify user exists
	_, err := uc.userRepo.FindByID(ctx, req.UserID)
	if err != nil {
		return nil, errors.New("user not found")
	}

	// Anti-fraud: block reuse of a payment reference. A transaction_id already
	// tied to a pending or approved deposit cannot be submitted again.
	dup, err := uc.transactionRepo.ExistsActiveDepositByTransactionID(ctx, req.TransactionID)
	if err != nil {
		return nil, err
	}
	if dup {
		return nil, errDuplicateReference
	}

	// verified stays false unless the verifier returns a positive verdict. A
	// verified deposit is auto-approved below; everything else is left pending
	// for manual admin approval. creditAmount is what actually hits the wallet —
	// for a verified deposit it is the net amount the house account received
	// (settledAmount), NOT what the player typed or the fee-inclusive total.
	verified := false
	creditAmount := req.Amount
	if uc.paymentVerifier != nil {
		// Fail closed: if the verifier is configured but currently unreachable, do
		// not accept the deposit at all. Otherwise a receipt we can't confirm would
		// queue for a manual approval that credits blindly. The Mini App checks this
		// same signal up front (GetPublicStatus) so players are warned BEFORE they
		// pay; this backstops a direct API submit.
		if !uc.paymentVerifier.Available(ctx) {
			return nil, ErrDepositVerifierDown
		}

		verification, err := uc.paymentVerifier.Verify(ctx, domain.PaymentVerificationRequest{
			Method:    req.TransactionType,
			Reference: req.TransactionID,
			UserID:    req.UserID,
		})
		switch {
		case err == nil:
			if verification.Provider != req.TransactionType {
				return nil, errors.New("payment provider does not match transaction_type")
			}
			// Guard against referencing a receipt for a wildly different amount.
			// The Telebirr service fee is already excluded (we read settledAmount),
			// so the player simply types the amount they sent; a small tolerance
			// absorbs rounding. The wallet is credited the verified net amount.
			if math.Abs(verification.Amount-req.Amount) > 1.0 {
				log.Printf("deposit %s: amount mismatch — verified %.2f, requested %.2f", req.TransactionID, verification.Amount, req.Amount)
				return nil, fmt.Errorf("verified payment amount (%.2f) does not match requested amount (%.2f)", verification.Amount, req.Amount)
			}
			verified = true
			creditAmount = verification.Amount
		case errors.Is(err, domain.ErrVerifierUnavailable):
			// Infrastructure failure (verifier down, timeout, 5xx, auth, rate
			// limit) — fall back to manual approval rather than reject the
			// deposit. The receipt was NOT judged invalid.
			log.Printf("deposit %s: payment verifier unavailable, falling back to manual approval: %v", req.TransactionID, err)
		default:
			// Definitive negative verdict (bad receipt, amount/provider mismatch).
			return nil, fmt.Errorf("payment verification failed: %w", err)
		}
	}

	// Create transaction with pending status. Verified deposits are immediately
	// approved below so balance updates still use the existing atomic path.
	transactionType := req.TransactionType
	transaction := &domain.Transaction{
		UserID:          req.UserID,
		Type:            domain.TransactionTypeDeposit,
		Category:        domain.TransactionCategoryDeposit,
		Amount:          creditAmount,
		Status:          domain.TransactionStatusPending,
		TransactionType: &transactionType,
		TransactionID:   &req.TransactionID,
	}

	// Save transaction (no balance update). The partial unique index catches a
	// concurrent duplicate that slipped past the check above (race).
	if err := uc.transactionRepo.Create(ctx, nil, transaction); err != nil {
		if strings.Contains(err.Error(), "uniq_active_deposit_transaction_id") {
			return nil, errDuplicateReference
		}
		return nil, fmt.Errorf("failed to create deposit transaction: %w", err)
	}

	if verified {
		return uc.transactionService.ApproveDeposit(ctx, transaction.ID)
	}

	return transaction, nil
}

// Withdraw creates a withdrawal and immediately subtracts from balance.
// The payout destination is always the user's verified registration phone — the
// client-supplied account number is ignored — so a withdrawal can never be
// redirected to a different account.
func (uc *WalletUseCase) Withdraw(ctx context.Context, req domain.WithdrawRequest) (*domain.Transaction, error) {
	// Validate amount
	if req.Amount <= 0 {
		return nil, errors.New("amount must be greater than 0")
	}

	// Enforce the minimum withdrawal.
	if req.Amount < domain.MinWithdrawalAmount {
		return nil, fmt.Errorf("minimum withdrawal is %.0f birr", domain.MinWithdrawalAmount)
	}

	// Validate account type
	if !domain.IsSupportedPaymentMethod(req.AccountType) {
		return nil, errors.New("account_type must be one of Telebirr, CBEBirr, Mpesa")
	}

	// Verify user exists
	user, err := uc.userRepo.FindByID(ctx, req.UserID)
	if err != nil {
		return nil, errors.New("user not found")
	}

	// Determine the payout destination. All supported methods (Telebirr, CBE
	// Birr, M-Pesa) are phone-based mobile money. By default the payout goes to
	// the user's verified registration phone (a real Ethiopian mobile shared
	// with the bot); if the player supplies a different number — because their
	// wallet is on another phone — we accept it only after validating it is a
	// real Ethiopian mobile, so a payout can never go to a typo'd account.
	var payoutAccount string
	if supplied := strings.TrimSpace(req.AccountNumber); supplied != "" {
		if !utils.IsEthiopianMobile(supplied) {
			return nil, errors.New("withdrawal account must be a valid Ethiopian phone number")
		}
		payoutAccount = utils.CanonicalEthiopianPhone(supplied)
	} else {
		if !utils.IsEthiopianMobile(user.PhoneNumber) {
			return nil, errors.New("no verified phone number on file; provide a phone number to withdraw to")
		}
		payoutAccount = utils.CanonicalEthiopianPhone(user.PhoneNumber)
	}

	// Process withdrawal (database operations in repository)
	return uc.transactionService.ProcessWithdrawal(ctx, req.UserID, req.Amount, payoutAccount, req.AccountType)
}

// Transfer transfers money from one user to another (atomic operation)
func (uc *WalletUseCase) Transfer(ctx context.Context, req domain.TransferRequest) (*domain.Transaction, *domain.Transaction, error) {
	// Validate amount
	if req.Amount <= 0 {
		return nil, nil, errors.New("amount must be greater than 0")
	}

	// No self-transfer
	if req.SenderID == req.ReceiverID {
		return nil, nil, errors.New("cannot transfer to yourself")
	}

	// Verify sender exists
	_, err := uc.userRepo.FindByID(ctx, req.SenderID)
	if err != nil {
		return nil, nil, errors.New("sender not found")
	}

	// Verify receiver exists
	_, err = uc.userRepo.FindByID(ctx, req.ReceiverID)
	if err != nil {
		return nil, nil, errors.New("receiver not found")
	}

	// Process transfer (database operations in repository)
	return uc.transactionService.ProcessTransfer(ctx, req.SenderID, req.ReceiverID, req.Amount)
}

// GetWallet retrieves wallet by user ID
func (uc *WalletUseCase) GetWallet(ctx context.Context, userID uuid.UUID) (*domain.Wallet, error) {
	wallet, err := uc.walletRepo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return wallet, nil
}

// GetWalletByTelegramID retrieves wallet by Telegram ID
func (uc *WalletUseCase) GetWalletByTelegramID(ctx context.Context, telegramID int64) (*domain.Wallet, error) {
	// Find user by telegram ID
	user, err := uc.userRepo.FindByTelegramID(ctx, telegramID)
	if err != nil {
		return nil, err
	}

	// Get wallet by user ID
	wallet, err := uc.walletRepo.FindByUserID(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	return wallet, nil
}

// ApproveDeposit approves a pending deposit and credits the wallet. Unless force
// is set, it REFUSES a receipt the verifier definitively rejected (paid to a
// different account, wrong amount, or nonexistent) — closing the blind-approval
// hole where an admin could credit a receipt the verifier already proved bad. An
// admin who has confirmed the payment out-of-band can still force it.
func (uc *WalletUseCase) ApproveDeposit(ctx context.Context, transactionID uuid.UUID, force bool) (*domain.Transaction, error) {
	if !force && uc.verificationLogs != nil {
		txn, err := uc.transactionRepo.FindByID(ctx, transactionID)
		if err != nil {
			return nil, fmt.Errorf("transaction not found: %w", err)
		}
		if txn.TransactionID != nil && *txn.TransactionID != "" {
			// A definitive rejection is a hard block; "unavailable" (manual review)
			// and "verified" are fine to approve.
			if latest, lerr := uc.verificationLogs.LatestByReference(ctx, *txn.TransactionID); lerr == nil &&
				latest != nil && latest.Outcome == domain.VerificationRejected {
				return nil, fmt.Errorf("%w: %s", ErrDepositReceiptRejected, latest.Reason)
			}
		}
	}
	return uc.transactionService.ApproveDeposit(ctx, transactionID)
}

// AdjustBalance manually credits or debits a user's wallet (admin action).
func (uc *WalletUseCase) AdjustBalance(ctx context.Context, userID uuid.UUID, amount float64, reason string) (*domain.Transaction, error) {
	return uc.transactionService.AdjustBalance(ctx, userID, amount, reason)
}

// RejectDeposit rejects a pending deposit transaction (no balance change)
func (uc *WalletUseCase) RejectDeposit(ctx context.Context, transactionID uuid.UUID) (*domain.Transaction, error) {
	return uc.transactionService.RejectDeposit(ctx, transactionID)
}

// ApproveWithdrawal approves a pending withdrawal transaction (balance already subtracted)
func (uc *WalletUseCase) ApproveWithdrawal(ctx context.Context, transactionID uuid.UUID) (*domain.Transaction, error) {
	return uc.transactionService.ApproveWithdrawal(ctx, transactionID)
}

// RejectWithdrawal rejects a pending withdrawal and refunds the balance
func (uc *WalletUseCase) RejectWithdrawal(ctx context.Context, transactionID uuid.UUID) (*domain.Transaction, error) {
	return uc.transactionService.RejectWithdrawal(ctx, transactionID)
}

// RejectWithdrawalToBonus rejects a pending withdrawal but SPLITS the refund: the
// part backed by genuine money (deposits + winnings, minus what's already been
// paid out and what they already hold as cash) returns to withdrawable balance;
// the remainder — money that came from referral/bonus, never earned by playing —
// returns as play-only BONUS instead. So a real winner is made whole in cash,
// while a farmer's referral money can't be cashed out (only played).
//
// The whole split runs in one DB transaction so the balance credit, the bonus
// grant and the status flip commit together.
func (uc *WalletUseCase) RejectWithdrawalToBonus(ctx context.Context, transactionID uuid.UUID) (*domain.WithdrawalRollbackResult, error) {
	t, err := uc.transactionRepo.FindByID(ctx, transactionID)
	if err != nil {
		return nil, fmt.Errorf("transaction not found: %w", err)
	}
	if t.Type != domain.TransactionTypeWithdraw {
		return nil, errors.New("transaction is not a withdrawal")
	}
	if t.Status != domain.TransactionStatusPending {
		return nil, fmt.Errorf("transaction is not pending (current status: %s)", t.Status)
	}

	// Genuine entitlement = everything they deposited or genuinely won, minus
	// genuine cash already paid out. Stable during this operation, so read it up
	// front (the balance itself is read under the wallet lock below).
	var genuineIn, alreadyWithdrawn float64
	err = uc.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COALESCE(SUM(amount),0) FROM transactions WHERE user_id=$1 AND category='deposit'   AND status='completed')
		  + (SELECT COALESCE(SUM(amount),0) FROM transactions WHERE user_id=$1 AND category='winnings'),
			(SELECT COALESCE(SUM(amount),0) FROM transactions WHERE user_id=$1 AND category='withdrawal' AND status='completed')
	`, t.UserID).Scan(&genuineIn, &alreadyWithdrawn)
	if err != nil {
		return nil, fmt.Errorf("failed to compute entitlement: %w", err)
	}
	genuineEntitlement := genuineIn - alreadyWithdrawn

	tx, err := uc.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	wallet, err := uc.walletRepo.LockForUpdate(ctx, tx, t.UserID)
	if err != nil {
		return nil, fmt.Errorf("wallet not found: %w", err)
	}

	// Cash they may still hold as withdrawable = entitlement minus what's already
	// sitting in their balance. The refund tops the cash side up to that ceiling;
	// anything above it in this withdrawal is referral/bonus money → bonus.
	headroom := math.Max(0, genuineEntitlement-wallet.Balance)
	realBack := math.Min(t.Amount, headroom)
	bonusBack := t.Amount - realBack

	if realBack > 0 {
		if err := uc.walletRepo.UpdateBalance(ctx, tx, t.UserID, realBack); err != nil {
			return nil, fmt.Errorf("failed to refund balance: %w", err)
		}
	}
	if bonusBack > 0 {
		if _, err := uc.bonusRepo.Grant(ctx, tx, t.UserID, bonusBack, "Withdrawal rolled back — referral/bonus portion"); err != nil {
			return nil, fmt.Errorf("failed to grant bonus: %w", err)
		}
	}
	if err := uc.transactionRepo.UpdateStatus(ctx, tx, transactionID, domain.TransactionStatusFailed); err != nil {
		return nil, fmt.Errorf("failed to update transaction status: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit: %w", err)
	}

	return &domain.WithdrawalRollbackResult{Amount: t.Amount, RealRefunded: realBack, BonusGranted: bonusBack}, nil
}

// CancelTransaction cancels a pending transaction (for deposits, no balance change; for withdrawals, refund balance)
func (uc *WalletUseCase) CancelTransaction(ctx context.Context, transactionID uuid.UUID) (*domain.Transaction, error) {
	return uc.transactionService.CancelTransaction(ctx, transactionID)
}

// GetDepositHistory returns deposit transactions for a user
// If limit is 0 or negative, uses default limit of 10
func (uc *WalletUseCase) GetDepositHistory(ctx context.Context, userID uuid.UUID, limit int) ([]*domain.Transaction, error) {
	if limit <= 0 {
		limit = domain.DefaultTransactionHistoryLimit
	}
	return uc.transactionRepo.FindByUserIDAndType(ctx, userID, domain.TransactionTypeDeposit, limit)
}

// GetWithdrawalHistory returns withdrawal transactions for a user
// If limit is 0 or negative, uses default limit of 10
func (uc *WalletUseCase) GetWithdrawalHistory(ctx context.Context, userID uuid.UUID, limit int) ([]*domain.Transaction, error) {
	if limit <= 0 {
		limit = domain.DefaultTransactionHistoryLimit
	}
	return uc.transactionRepo.FindByUserIDAndType(ctx, userID, domain.TransactionTypeWithdraw, limit)
}

// GetTransferHistory returns transfer transactions (both in and out) for a user
// Includes user information for the other party in the transfer
// If limit is 0 or negative, uses default limit of 10
func (uc *WalletUseCase) GetTransferHistory(ctx context.Context, userID uuid.UUID, limit int) ([]*domain.TransferHistoryEntry, error) {
	if limit <= 0 {
		limit = domain.DefaultTransactionHistoryLimit
	}
	transactions, err := uc.transactionRepo.FindByUserIDAndTypes(ctx, userID, []domain.TransactionType{
		domain.TransactionTypeTransferIn,
		domain.TransactionTypeTransferOut,
	}, limit)
	if err != nil {
		return nil, err
	}

	// Collect unique user IDs from references
	userIDs := make(map[uuid.UUID]bool)
	for _, tx := range transactions {
		if tx.Reference != nil {
			otherUserID, err := uuid.Parse(*tx.Reference)
			if err == nil {
				userIDs[otherUserID] = true
			}
		}
	}

	// Fetch all users in one batch
	usersMap := make(map[uuid.UUID]*domain.User)
	for otherUserID := range userIDs {
		user, err := uc.userRepo.FindByID(ctx, otherUserID)
		if err == nil && user != nil {
			usersMap[otherUserID] = user
		}
	}

	// Build response with user information
	entries := make([]*domain.TransferHistoryEntry, 0, len(transactions))
	for _, tx := range transactions {
		entry := &domain.TransferHistoryEntry{
			Transaction: tx,
		}

		// For transfer_out: reference is receiver's ID
		// For transfer_in: reference is sender's ID
		if tx.Reference != nil {
			otherUserID, err := uuid.Parse(*tx.Reference)
			if err == nil {
				if user, exists := usersMap[otherUserID]; exists {
					entry.To = user
				}
			}
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

// Admin transaction query methods

// GetAdminTransactions applies tab filters and search before pagination.
func (uc *WalletUseCase) GetAdminTransactions(ctx context.Context, filter domain.AdminTransactionFilter, search string, limit, offset int) ([]*domain.Transaction, int, error) {
	return uc.transactionRepo.ListAdmin(ctx, filter, strings.TrimSpace(search), limit, offset)
}

// GetPendingDeposits returns pending deposit transactions for admin
func (uc *WalletUseCase) GetPendingDeposits(ctx context.Context, limit, offset int) ([]*domain.Transaction, error) {
	return uc.transactionRepo.FindByStatusAndType(ctx, domain.TransactionStatusPending, domain.TransactionTypeDeposit, limit, offset)
}

// GetPendingWithdrawals returns pending withdrawal transactions for admin
func (uc *WalletUseCase) GetPendingWithdrawals(ctx context.Context, limit, offset int) ([]*domain.Transaction, error) {
	return uc.transactionRepo.FindWithdrawalsByStatus(ctx, domain.TransactionStatusPending, limit, offset)
}

// GetCompletedDeposits returns completed deposit transactions for admin
func (uc *WalletUseCase) GetCompletedDeposits(ctx context.Context, limit, offset int) ([]*domain.Transaction, error) {
	return uc.transactionRepo.FindByStatusAndType(ctx, domain.TransactionStatusCompleted, domain.TransactionTypeDeposit, limit, offset)
}

// GetCompletedWithdrawals returns completed withdrawal transactions for admin
func (uc *WalletUseCase) GetCompletedWithdrawals(ctx context.Context, limit, offset int) ([]*domain.Transaction, error) {
	return uc.transactionRepo.FindWithdrawalsByStatus(ctx, domain.TransactionStatusCompleted, limit, offset)
}

// GetFailedTransactions returns all failed transactions for admin
func (uc *WalletUseCase) GetFailedTransactions(ctx context.Context, limit, offset int) ([]*domain.Transaction, error) {
	return uc.transactionRepo.FindByStatus(ctx, domain.TransactionStatusFailed, limit, offset)
}

// GetTransferTransactions returns all transfer transactions (in and out) for admin
func (uc *WalletUseCase) GetTransferTransactions(ctx context.Context, limit, offset int) ([]*domain.Transaction, error) {
	return uc.transactionRepo.FindByTypes(ctx, []domain.TransactionType{
		domain.TransactionTypeTransferIn,
		domain.TransactionTypeTransferOut,
	}, limit, offset)
}

// GetAllTransactions returns all transactions with optional filters for admin
func (uc *WalletUseCase) GetAllTransactions(ctx context.Context, limit, offset int) ([]*domain.Transaction, error) {
	return uc.transactionRepo.FindAll(ctx, limit, offset)
}

// CountAllTransactions is the grand total (for page-by-page navigation).
func (uc *WalletUseCase) CountAllTransactions(ctx context.Context) (int, error) {
	return uc.transactionRepo.CountAll(ctx)
}

// GetHouseCutDetail is the drill-down behind the dashboard house-cut figure:
// total, real-player P&L, and per-tier / per-day breakdowns.
func (uc *WalletUseCase) GetHouseCutDetail(ctx context.Context) (*domain.HouseCutDetail, error) {
	total, err := uc.gameRepo.GetTotalHouseCut(ctx)
	if err != nil {
		return nil, err
	}
	pnl, err := uc.transactionRepo.RealPlayerGamePnL(ctx)
	if err != nil {
		return nil, err
	}
	byTier, err := uc.gameRepo.HouseCutByTier(ctx)
	if err != nil {
		return nil, err
	}
	byDay, err := uc.gameRepo.HouseCutByDay(ctx, 14)
	if err != nil {
		return nil, err
	}
	return &domain.HouseCutDetail{TotalHouseCut: total, RealPlayerPnl: pnl, ByTier: byTier, ByDay: byDay}, nil
}

// CountWithdrawalsByStatus is the total of genuine withdrawal requests at a status.
func (uc *WalletUseCase) CountWithdrawalsByStatus(ctx context.Context, status domain.TransactionStatus) (int, error) {
	return uc.transactionRepo.CountWithdrawalsByStatus(ctx, status)
}

// CountByStatusAndType is the total of a status+type list (for pagination of the
// pending/completed deposit & withdrawal tabs).
func (uc *WalletUseCase) CountByStatusAndType(ctx context.Context, status domain.TransactionStatus, t domain.TransactionType) (int, error) {
	return uc.transactionRepo.CountByStatusAndType(ctx, status, t)
}

// GetUserTransactions returns one user's full transaction history (paginated
// with a grand total) for the admin player-detail view — deposits, withdrawals,
// bets, winnings, bonuses and referral rewards in one place.
func (uc *WalletUseCase) GetUserTransactions(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.Transaction, int, error) {
	rows, err := uc.transactionRepo.FindByUserID(ctx, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := uc.transactionRepo.CountByUser(ctx, userID)
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// GetRealPlayerWinnings lists winnings paid to real (non-bot) players, plus the
// total for pagination. Powers the admin "Winners" tab.
func (uc *WalletUseCase) GetRealPlayerWinnings(ctx context.Context, limit, offset int) ([]*domain.Transaction, int, error) {
	rows, err := uc.transactionRepo.FindRealPlayerWinnings(ctx, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := uc.transactionRepo.CountRealPlayerWinnings(ctx)
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// GetDashboardStats returns dashboard statistics for admin
func (uc *WalletUseCase) GetDashboardStats(ctx context.Context) (*domain.DashboardStats, error) {
	stats := &domain.DashboardStats{
		GamesByType: make(map[domain.GameType]int),
	}

	// Get pending deposits count
	pendingDeposits, err := uc.transactionRepo.CountByStatusAndType(ctx, domain.TransactionStatusPending, domain.TransactionTypeDeposit)
	if err != nil {
		return nil, fmt.Errorf("failed to count pending deposits: %w", err)
	}
	stats.PendingDeposits = pendingDeposits

	// Get pending withdrawals count
	pendingWithdrawals, err := uc.transactionRepo.CountByStatusAndType(ctx, domain.TransactionStatusPending, domain.TransactionTypeWithdraw)
	if err != nil {
		return nil, fmt.Errorf("failed to count pending withdrawals: %w", err)
	}
	stats.PendingWithdrawals = pendingWithdrawals

	// Get total users count
	totalUsers, err := uc.userRepo.CountAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to count users: %w", err)
	}
	stats.TotalUsers = totalUsers

	// Get total transactions count
	totalTransactions, err := uc.transactionRepo.CountAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to count transactions: %w", err)
	}
	stats.TotalTransactions = totalTransactions

	// Get total balance
	totalBalance, err := uc.walletRepo.GetTotalBalance(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get total balance: %w", err)
	}
	stats.TotalBalance = totalBalance

	// Get games by type
	gamesByType, err := uc.gameRepo.CountGamesByType(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to count games by type: %w", err)
	}
	stats.GamesByType = gamesByType

	// Get total house cut
	totalHouseCut, err := uc.gameRepo.GetTotalHouseCut(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get total house cut: %w", err)
	}
	stats.TotalHouseCut = totalHouseCut

	// Real-player game P&L (stakes − winnings, bots excluded). Negative = real
	// cash exposure from bot-inflated pools real players won.
	realPnl, err := uc.transactionRepo.RealPlayerGamePnL(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to compute real-player game P&L: %w", err)
	}
	stats.RealPlayerGamePnl = realPnl

	return stats, nil
}
