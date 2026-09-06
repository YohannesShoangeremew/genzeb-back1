package utils

import (
	"regexp"
	"strings"
)

var nonDigit = regexp.MustCompile(`\D`)

// CanonicalEthiopianPhone converts any common Ethiopian phone format to the
// canonical "251XXXXXXXXX" form used when users register via the bot, so
// logins by phone match the stored value regardless of how it's typed:
//
//	0911223344  → 251911223344
//	+251911223344 / 251911223344 → 251911223344
//	911223344   → 251911223344
func CanonicalEthiopianPhone(input string) string {
	d := nonDigit.ReplaceAllString(input, "")
	switch {
	case strings.HasPrefix(d, "251"):
		// already canonical
	case strings.HasPrefix(d, "0"):
		d = "251" + d[1:]
	case len(d) == 9:
		d = "251" + d
	}
	return d
}

// IsEthiopianMobile reports whether phone is a valid Ethiopian mobile number.
// It accepts the common shapes Telegram and users provide:
//
//	+251 9XXXXXXXX / 251 9XXXXXXXX  (international, 9 or 7 prefix)
//	09XXXXXXXX     / 07XXXXXXXX     (local, with trunk 0)
//	9XXXXXXXX      / 7XXXXXXXX      (national significant number)
//
// Ethiopian mobile subscriber numbers are 9 digits beginning with 9 (legacy)
// or 7 (newer Safaricom range).
func IsEthiopianMobile(phone string) bool {
	d := nonDigit.ReplaceAllString(phone, "")

	switch {
	case strings.HasPrefix(d, "251"):
		d = d[3:] // drop country code
	case strings.HasPrefix(d, "0"):
		d = d[1:] // drop trunk zero
	}

	if len(d) != 9 {
		return false
	}
	return d[0] == '9' || d[0] == '7'
}

