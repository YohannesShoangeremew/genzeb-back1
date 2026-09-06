package referral

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// GenerateReferralCode generates a unique referral code
// Returns an 8-character alphanumeric code
func GenerateReferralCode() (string, error) {
	// Generate 6 random bytes
	bytes := make([]byte, 6)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Encode to base64 and take first 8 characters
	code := base64.URLEncoding.EncodeToString(bytes)
	code = code[:8]

	// Convert to uppercase for consistency
	return code, nil
}

