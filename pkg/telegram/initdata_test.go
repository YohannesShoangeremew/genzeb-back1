package telegram

import "testing"

// Canonical example from the Telegram Mini Apps documentation.
// https://docs.telegram-mini-apps.com/platform/init-data#validating
const (
	testBotToken = "5768337691:AAH5YkoiEuPk8-FZa32hStHTqXiLPtAEhx8"
	testInitData = "query_id=AAHdF6IQAAAAAN0XohDhrOrc&user=%7B%22id%22%3A279058397%2C%22first_name%22%3A%22Vladislav%22%2C%22last_name%22%3A%22Kibenko%22%2C%22username%22%3A%22vdkfrost%22%2C%22language_code%22%3A%22ru%22%2C%22is_premium%22%3Atrue%7D&auth_date=1662771648&hash=c501b71e775f74ce10e377dea85a7ea24ecd640b223ea86dfe453e0eaed2e2b2"
)

func TestValidate_GoodSignature(t *testing.T) {
	// maxAge = 0 disables the freshness check (the vector is from 2022).
	u, err := Validate(testInitData, testBotToken, 0)
	if err != nil {
		t.Fatalf("expected valid initData, got error: %v", err)
	}
	if u.ID != 279058397 {
		t.Errorf("expected user id 279058397, got %d", u.ID)
	}
	if u.Username != "vdkfrost" {
		t.Errorf("expected username vdkfrost, got %q", u.Username)
	}
}

func TestValidate_WrongBotToken(t *testing.T) {
	if _, err := Validate(testInitData, "0000000000:wrong-token", 0); err == nil {
		t.Fatal("expected signature failure with wrong bot token, got nil")
	}
}

func TestValidate_Tampered(t *testing.T) {
	// Flip the user id; the hash should no longer match.
	tampered := "query_id=AAHdF6IQAAAAAN0XohDhrOrc&user=%7B%22id%22%3A999999999%7D&auth_date=1662771648&hash=c501b71e775f74ce10e377dea85a7ea24ecd640b223ea86dfe453e0eaed2e2b2"
	if _, err := Validate(tampered, testBotToken, 0); err == nil {
		t.Fatal("expected signature failure for tampered data, got nil")
	}
}

func TestValidate_MissingHash(t *testing.T) {
	if _, err := Validate("user=%7B%22id%22%3A1%7D&auth_date=1", testBotToken, 0); err == nil {
		t.Fatal("expected error for missing hash, got nil")
	}
}

func TestValidate_NoBotToken(t *testing.T) {
	if _, err := Validate(testInitData, "", 0); err == nil {
		t.Fatal("expected error when bot token is empty, got nil")
	}
}
