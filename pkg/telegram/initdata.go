// Package telegram verifies Telegram Web App (Mini App) initData.
//
// When a Telegram Mini App opens the website, Telegram injects a signed
// `initData` query string. This package validates that signature using the
// bot token (per https://core.telegram.org/bots/webapps#validating-data-received-via-the-mini-app)
// so the backend can trust the user identity it contains.
package telegram

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// User is the Telegram user object embedded in initData.
type User struct {
	ID           int64  `json:"id"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	Username     string `json:"username"`
	LanguageCode string `json:"language_code"`
	PhotoURL     string `json:"photo_url"`
	IsPremium    bool   `json:"is_premium"`
}

// Validate verifies the initData signature with the bot token and returns the
// embedded Telegram user. If maxAge > 0, initData older than maxAge (per its
// auth_date field) is rejected to prevent replay of stale payloads.
func Validate(initData, botToken string, maxAge time.Duration) (*User, error) {
	if botToken == "" {
		return nil, fmt.Errorf("telegram bot token is not configured")
	}

	values, err := url.ParseQuery(initData)
	if err != nil {
		return nil, fmt.Errorf("invalid initData: %w", err)
	}

	hash := values.Get("hash")
	if hash == "" {
		return nil, fmt.Errorf("initData is missing the hash field")
	}

	// data_check_string = every field except "hash", sorted by key,
	// formatted as "key=value" and joined with newlines (decoded values).
	keys := make([]string, 0, len(values))
	for k := range values {
		if k == "hash" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, k+"="+values.Get(k))
	}
	dataCheckString := strings.Join(pairs, "\n")

	// secret_key = HMAC_SHA256(key="WebAppData", data=botToken)
	secretKey := hmacSHA256([]byte("WebAppData"), []byte(botToken))
	// expected hash = HMAC_SHA256(key=secret_key, data=dataCheckString)
	expected := hex.EncodeToString(hmacSHA256(secretKey, []byte(dataCheckString)))

	if !hmac.Equal([]byte(expected), []byte(hash)) {
		return nil, fmt.Errorf("initData signature verification failed")
	}

	// Optional freshness check.
	if maxAge > 0 {
		if authDate := values.Get("auth_date"); authDate != "" {
			if sec, err := strconv.ParseInt(authDate, 10, 64); err == nil {
				if time.Since(time.Unix(sec, 0)) > maxAge {
					return nil, fmt.Errorf("initData has expired")
				}
			}
		}
	}

	userJSON := values.Get("user")
	if userJSON == "" {
		return nil, fmt.Errorf("initData is missing the user field")
	}

	var u User
	if err := json.Unmarshal([]byte(userJSON), &u); err != nil {
		return nil, fmt.Errorf("failed to parse telegram user: %w", err)
	}
	if u.ID == 0 {
		return nil, fmt.Errorf("initData user has no id")
	}

	return &u, nil
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}
