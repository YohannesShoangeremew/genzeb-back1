package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bingo/backend/internal/domain"
	"github.com/bingo/backend/pkg/auth"
	"github.com/bingo/backend/pkg/jwt"
	"github.com/bingo/backend/pkg/referral"
	"github.com/bingo/backend/pkg/telegram"
	"github.com/bingo/backend/pkg/utils"
)

// telegramInitDataMaxAge is how long a Telegram initData payload stays valid.
const telegramInitDataMaxAge = 24 * time.Hour

type AuthUseCase struct {
	userRepo        domain.UserRepository
	jwtService      *jwt.Service
	adminSecretCode string
	botToken        string
}

// NewAuthUseCase creates a new auth use case
func NewAuthUseCase(userRepo domain.UserRepository, jwtService *jwt.Service, adminSecretCode, botToken string) *AuthUseCase {
	return &AuthUseCase{
		userRepo:        userRepo,
		jwtService:      jwtService,
		adminSecretCode: adminSecretCode,
		botToken:        botToken,
	}
}

// TelegramLogin authenticates a Telegram Mini App user from signed initData and
// returns a JWT. The user must already be registered (via the bot) — this does
// not create new users, since the phone number is only captured by the bot.
func (uc *AuthUseCase) TelegramLogin(ctx context.Context, initData string) (*domain.LoginResponse, error) {
	tgUser, err := telegram.Validate(initData, uc.botToken, telegramInitDataMaxAge)
	if err != nil {
		return nil, fmt.Errorf("telegram authentication failed: %w", err)
	}

	user, err := uc.userRepo.FindByTelegramID(ctx, tgUser.ID)
	if err != nil {
		return nil, errors.New("user not registered: please start the Telegram bot first")
	}
	if user.Banned {
		return nil, errors.New("your account has been suspended")
	}

	token, err := uc.jwtService.GenerateToken(user.ID, user.Role)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	user.Password = nil
	return &domain.LoginResponse{
		Token: token,
		User:  user,
	}, nil
}

// Login authenticates an admin user and returns a JWT token. The user is
// resolved by phone (preferred) or, for backward compatibility, telegram_id.
func (uc *AuthUseCase) Login(ctx context.Context, req domain.LoginRequest) (*domain.LoginResponse, error) {
	var user *domain.User
	var err error
	switch {
	case req.Phone != "":
		user, err = uc.userRepo.FindByPhone(ctx, utils.CanonicalEthiopianPhone(req.Phone))
	case req.TelegramID != 0:
		user, err = uc.userRepo.FindByTelegramID(ctx, req.TelegramID)
	default:
		return nil, errors.New("phone or telegram_id is required")
	}
	if err != nil {
		return nil, errors.New("invalid credentials")
	}

	if user.Banned {
		return nil, errors.New("your account has been suspended")
	}

	// Check if user has admin role
	if user.Role != "admin" {
		return nil, errors.New("admin access required")
	}

	// Check if user has a password set
	if user.Password == nil || *user.Password == "" {
		return nil, errors.New("password not set for this user")
	}

	// Verify password
	if !auth.CheckPasswordHash(req.Password, *user.Password) {
		return nil, errors.New("invalid credentials")
	}

	// Generate JWT token
	token, err := uc.jwtService.GenerateToken(user.ID, user.Role)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	// Clear password from response
	user.Password = nil

	return &domain.LoginResponse{
		Token: token,
		User:  user,
	}, nil
}

// CreateAdmin promotes an existing user to admin and sets admin password.
func (uc *AuthUseCase) CreateAdmin(ctx context.Context, req domain.CreateAdminRequest) (*domain.User, error) {
	if uc.adminSecretCode == "" {
		return nil, errors.New("secret code not configured")
	}
	if req.SecretCode != uc.adminSecretCode {
		return nil, errors.New("invalid secret code")
	}

	hashedPassword, err := auth.HashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	_, err = uc.userRepo.FindByTelegramID(ctx, req.TelegramID)
	if err != nil {
		if err.Error() != "user not found" {
			return nil, err
		}

		referralCode, err := uc.generateUniqueReferralCode(ctx)
		if err != nil {
			return nil, err
		}

		newUser := &domain.User{
			TelegramID:  req.TelegramID,
			FirstName:   "Admin",
			LastName:    nil,
			PhoneNumber: fmt.Sprintf("tg_%d", req.TelegramID),
			ReferalCode: referralCode,
			Role:        "admin",
			Password:    &hashedPassword,
		}

		if err := uc.userRepo.Create(ctx, nil, newUser); err != nil {
			return nil, err
		}

		newUser.Password = nil
		return newUser, nil
	}

	if err := uc.userRepo.SetAdminCredentialsByTelegramID(ctx, req.TelegramID, hashedPassword); err != nil {
		return nil, err
	}

	updatedUser, err := uc.userRepo.FindByTelegramID(ctx, req.TelegramID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch updated user: %w", err)
	}

	updatedUser.Password = nil
	return updatedUser, nil
}

func (uc *AuthUseCase) generateUniqueReferralCode(ctx context.Context) (string, error) {
	var referralCode string
	maxAttempts := domain.MaxReferralCodeGenerationAttempts
	for i := 0; i < maxAttempts; i++ {
		code, err := referral.GenerateReferralCode()
		if err != nil {
			return "", fmt.Errorf("failed to generate referral code: %w", err)
		}

		_, err = uc.userRepo.FindByReferralCode(ctx, code)
		if err != nil {
			referralCode = code
			break
		}

		if i == maxAttempts-1 {
			return "", errors.New("failed to generate unique referral code after multiple attempts")
		}
	}

	return referralCode, nil
}
