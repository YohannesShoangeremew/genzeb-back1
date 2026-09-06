package domain

import "time"

// AppSettings holds operator-tunable knobs edited from the admin dashboard.
// Single row (app_settings table). Extend as more settings are added.
type AppSettings struct {
	MinDeposit          float64 `json:"min_deposit" db:"min_deposit"`
	WelcomeBonusEnabled bool    `json:"welcome_bonus_enabled" db:"welcome_bonus_enabled"`
	ReferralEnabled     bool    `json:"referral_enabled" db:"referral_enabled"`
	ReferralAmount      float64 `json:"referral_amount" db:"referral_amount"`
	// DepositBonus is awarded once after each successfully completed Telebirr or
	// CBE Birr deposit. It is play-only money and never becomes withdrawable cash.
	DepositBonusEnabled    bool    `json:"deposit_bonus_enabled" db:"deposit_bonus_enabled"`
	DepositBonusPercentage float64 `json:"deposit_bonus_percentage" db:"deposit_bonus_percentage"`
	// MaintenanceMode puts the player Mini App into "we'll be right back" mode:
	// the frontend shows a maintenance screen and the API rejects player actions
	// (join/deposit/withdraw/…) with 503, while the admin dashboard stays fully
	// usable. MaintenanceMessage is an optional operator note shown to players.
	MaintenanceMode    bool   `json:"maintenance_mode" db:"maintenance_mode"`
	MaintenanceMessage string `json:"maintenance_message" db:"maintenance_message"`
	// Per-method deposit switches. When a method is off, players cannot submit a
	// deposit with it (the API rejects it and the pickers hide it), so a channel
	// whose external verification has broken can be closed instantly without
	// taking the whole app down. Withdrawals are deliberately NOT gated by these,
	// so closing a broken deposit channel never traps a player's balance.
	DepositTelebirrEnabled bool      `json:"deposit_telebirr_enabled" db:"deposit_telebirr_enabled"`
	DepositCBEBirrEnabled  bool      `json:"deposit_cbebirr_enabled" db:"deposit_cbebirr_enabled"`
	DepositMpesaEnabled    bool      `json:"deposit_mpesa_enabled" db:"deposit_mpesa_enabled"`
	UpdatedAt              time.Time `json:"updated_at" db:"updated_at"`
}

// DepositMethodEnabled reports whether players may currently deposit with method
// m. An unknown method reads as disabled (fail closed for anything unexpected).
func (s *AppSettings) DepositMethodEnabled(m PaymentMethod) bool {
	switch m {
	case PaymentMethodTelebirr:
		return s.DepositTelebirrEnabled
	case PaymentMethodCBEBirr:
		return s.DepositCBEBirrEnabled
	case PaymentMethodMpesa:
		return s.DepositMpesaEnabled
	default:
		return false
	}
}

// UpdateAppSettingsRequest is the admin payload to change settings. Pointers so a
// partial update leaves untouched fields alone.
type UpdateAppSettingsRequest struct {
	MinDeposit             *float64 `json:"min_deposit,omitempty"`
	WelcomeBonusEnabled    *bool    `json:"welcome_bonus_enabled,omitempty"`
	ReferralEnabled        *bool    `json:"referral_enabled,omitempty"`
	ReferralAmount         *float64 `json:"referral_amount,omitempty"`
	DepositBonusEnabled    *bool    `json:"deposit_bonus_enabled,omitempty"`
	DepositBonusPercentage *float64 `json:"deposit_bonus_percentage,omitempty"`
	MaintenanceMode        *bool    `json:"maintenance_mode,omitempty"`
	MaintenanceMessage     *string  `json:"maintenance_message,omitempty"`

	DepositTelebirrEnabled *bool `json:"deposit_telebirr_enabled,omitempty"`
	DepositCBEBirrEnabled  *bool `json:"deposit_cbebirr_enabled,omitempty"`
	DepositMpesaEnabled    *bool `json:"deposit_mpesa_enabled,omitempty"`
}

// WithdrawalRollbackResult reports how a rejected withdrawal was split back: the
// genuine (deposit/winnings-backed) part returned to withdrawable cash, and the
// remainder — money the player never earned by playing — returned as play-only
// bonus instead. See WalletUseCase.RejectWithdrawalToBonus.
type WithdrawalRollbackResult struct {
	Amount       float64 `json:"amount"`        // the withdrawal that was rolled back
	RealRefunded float64 `json:"real_refunded"` // returned to withdrawable balance
	BonusGranted float64 `json:"bonus_granted"` // returned as play-only bonus
}
