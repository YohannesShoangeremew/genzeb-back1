package utils

import "testing"

// Ethiopia has two mobile operators and both must work everywhere: Ethio
// Telecom on the 9 range (09XXXXXXXX) and Safaricom Ethiopia on the 7 range
// (07XXXXXXXX). Registration, login and withdrawal payouts all key off these
// two functions agreeing, so the shapes are pinned here.
//
// The bug these guard against: registration used to store a differently
// normalized form (leading zeros stripped, no country code), so a number
// registered as 0911... was stored as 911... and never matched the 251911...
// a login looked up. Every accepted input shape must collapse to one value.

func TestCanonicalEthiopianPhone(t *testing.T) {
	cases := []struct{ in, want string }{
		// Ethio Telecom (9 range)
		{"0911223344", "251911223344"},
		{"251911223344", "251911223344"},
		{"+251911223344", "251911223344"},
		{"+251 91 122 3344", "251911223344"},
		{"911223344", "251911223344"},
		// Safaricom Ethiopia (7 range)
		{"0711223344", "251711223344"},
		{"251711223344", "251711223344"},
		{"+251711223344", "251711223344"},
		{"+251-71-122-3344", "251711223344"},
		{"711223344", "251711223344"},
	}
	for _, tc := range cases {
		if got := CanonicalEthiopianPhone(tc.in); got != tc.want {
			t.Errorf("CanonicalEthiopianPhone(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// Every shape of the same subscriber number must canonicalize identically —
// this is the property that actually keeps login working.
func TestCanonicalEthiopianPhoneIsStableAcrossShapes(t *testing.T) {
	for _, group := range [][]string{
		{"0911223344", "251911223344", "+251911223344", "911223344", "+251 911 223 344"},
		{"0711223344", "251711223344", "+251711223344", "711223344", "+251 711 223 344"},
	} {
		first := CanonicalEthiopianPhone(group[0])
		for _, shape := range group[1:] {
			if got := CanonicalEthiopianPhone(shape); got != first {
				t.Errorf("%q canonicalized to %q but %q gave %q — shapes must agree", shape, got, group[0], first)
			}
		}
	}
	// Applying it twice must not change the result.
	for _, in := range []string{"0911223344", "0711223344", "+251911223344"} {
		once := CanonicalEthiopianPhone(in)
		if twice := CanonicalEthiopianPhone(once); twice != once {
			t.Errorf("CanonicalEthiopianPhone not idempotent for %q: %q then %q", in, once, twice)
		}
	}
}

func TestIsEthiopianMobile(t *testing.T) {
	valid := []string{
		"0911223344", "251911223344", "+251911223344", "911223344", // Ethio Telecom
		"0711223344", "251711223344", "+251711223344", "711223344", // Safaricom Ethiopia
		"+251 91 122 3344", "+251-71-122-3344",
	}
	for _, v := range valid {
		if !IsEthiopianMobile(v) {
			t.Errorf("IsEthiopianMobile(%q) = false, want true", v)
		}
	}

	invalid := []string{
		"",
		"0811223344",    // 8 is not a mobile range
		"0611223344",    // 6 is not a mobile range
		"091122334",     // too short
		"09112233445",   // too long
		"251811223344",  // valid country code, wrong operator range
		"1234567890",    // not Ethiopian
		"BOT-00000001",  // synthetic bot identifier, must never pass
		"tg_900999",     // legacy placeholder
		"+1 555 123456", // wrong country
	}
	for _, v := range invalid {
		if IsEthiopianMobile(v) {
			t.Errorf("IsEthiopianMobile(%q) = true, want false", v)
		}
	}
}
