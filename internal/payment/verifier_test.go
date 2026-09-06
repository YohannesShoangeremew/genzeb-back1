package payment

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bingo/backend/config"
	"github.com/bingo/backend/internal/domain"
)

// telebirrReq is a small helper for the common Telebirr verification request.
func telebirrReq(reference string) domain.PaymentVerificationRequest {
	return domain.PaymentVerificationRequest{Method: domain.PaymentMethodTelebirr, Reference: reference}
}

func TestVerifierTelebirr(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			t.Fatalf("missing api key header")
		}

		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["reference"] != "CE626EJRNS" {
			t.Fatalf("reference = %q", payload["reference"])
		}
		if _, ok := payload["suffix"]; ok {
			t.Fatalf("telebirr request should not include suffix")
		}
		if _, ok := payload["phoneNumber"]; ok {
			t.Fatalf("telebirr request should not include phoneNumber")
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"success": true,
			"provider": "telebirr",
			"data": {
				"transactionStatus": "Completed",
				"receiptNo": "CE626EJRNS",
				"creditedPartyAccountNo": "2519****9691",
				"settledAmount": "N/A",
				"totalPaidAmount": "101.00 Birr"
			}
		}`))
	}))
	defer server.Close()

	verifier := NewVerifier(config.PaymentVerifierConfig{BaseURL: server.URL, APIKey: "test-key", TelebirrAccount: "0997709691"})
	result, err := verifier.Verify(context.Background(), telebirrReq("CE626EJRNS"))
	if err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
	if result.Provider != domain.PaymentMethodTelebirr {
		t.Fatalf("provider = %q", result.Provider)
	}
	if result.Amount != 101 {
		t.Fatalf("amount = %v", result.Amount)
	}
}

// A real Telebirr receipt carries both settledAmount (net, what the house got)
// and totalPaidAmount (settled + service fee). We must use settledAmount so the
// player isn't charged for the fee.
func TestVerifierTelebirrUsesSettledAmountNotTotalPaid(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"success": true,
			"provider": "telebirr",
			"data": {
				"transactionStatus": "Completed",
				"receiptNo": "DFU3F35PH3",
				"creditedPartyAccountNo": "2519****9691",
				"settledAmount": "20 Birr",
				"serviceFee": "0.87 Birr",
				"serviceFeeVAT": "0.13 Birr",
				"totalPaidAmount": "21 Birr"
			}
		}`))
	}))
	defer server.Close()

	verifier := NewVerifier(config.PaymentVerifierConfig{BaseURL: server.URL, APIKey: "test-key", TelebirrAccount: "0997709691"})
	result, err := verifier.Verify(context.Background(), telebirrReq("DFU3F35PH3"))
	if err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
	if result.Amount != 20 {
		t.Fatalf("amount = %v, want 20 (settledAmount, not totalPaidAmount 21)", result.Amount)
	}
}

// CBE Birr verification goes to the dedicated /verify-cbebirr endpoint with
// {receiptNumber, phoneNumber} where phoneNumber is the HOUSE CBE Birr number
// (receipts are looked up by receiver) in canonical 251 form. The endpoint
// returns the receipt fields flat (no {success, provider, data} wrapper), and
// the credited account comes as "251XXXXXXXXX - HOLDER NAME".
func TestVerifierCBEBirr(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/verify-cbebirr" {
			t.Fatalf("path = %q, want /verify-cbebirr", r.URL.Path)
		}
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["receiptNumber"] != "TEST123ABC" {
			t.Fatalf("receiptNumber = %q, want TEST123ABC", payload["receiptNumber"])
		}
		if payload["phoneNumber"] != "251912345678" {
			t.Fatalf("phoneNumber = %q, want canonical house number 251912345678", payload["phoneNumber"])
		}
		if _, ok := payload["reference"]; ok {
			t.Fatalf("cbebirr request should use receiptNumber, not reference")
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"customerName": "TEST USER NAME",
			"debitAccount": "",
			"creditAccount": "251912345678 - TEST USER NAME",
			"receiverName": "251912345678 - TEST USER NAME",
			"orderId": "FT25211JYPQX",
			"transactionStatus": "Completed",
			"reference": "FT25211JYPQX",
			"receiptNumber": "TEST123ABC2025",
			"transactionDate": "2025-07-30 17:57",
			"amount": "73000.00",
			"paidAmount": "73000.00",
			"serviceCharge": "0.00",
			"vat": "0.00",
			"totalPaidAmount": "73000.00",
			"paymentReason": "TransferFromBankToMM by Customer to Customer",
			"paymentChannel": "USSD"
		}`))
	}))
	defer server.Close()

	verifier := NewVerifier(config.PaymentVerifierConfig{
		BaseURL:        server.URL,
		APIKey:         "test-key",
		CBEBirrAccount: "0912345678", // local form; canonicalized to 251912345678 for the lookup
	})
	result, err := verifier.Verify(context.Background(), domain.PaymentVerificationRequest{
		Method:    domain.PaymentMethodCBEBirr,
		Reference: "TEST123ABC",
	})
	if err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
	if result.Provider != domain.PaymentMethodCBEBirr {
		t.Fatalf("provider = %q, want CBEBirr", result.Provider)
	}
	if result.Amount != 73000 {
		t.Fatalf("amount = %v, want 73000", result.Amount)
	}
}

// A CBE Birr receipt credited to someone other than the house is rejected even
// though the receipt itself is genuine.
func TestVerifierCBEBirrRejectsWrongReceiver(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"creditAccount": "251911999999 - SOMEONE ELSE",
			"transactionStatus": "Completed",
			"receiptNumber": "TEST123ABC2025",
			"amount": "100.00"
		}`))
	}))
	defer server.Close()

	verifier := NewVerifier(config.PaymentVerifierConfig{
		BaseURL:        server.URL,
		APIKey:         "test-key",
		CBEBirrAccount: "0912345678",
	})
	_, err := verifier.Verify(context.Background(), domain.PaymentVerificationRequest{
		Method:    domain.PaymentMethodCBEBirr,
		Reference: "TEST123ABC",
	})
	if err == nil {
		t.Fatal("receipt credited to a different number should be rejected")
	}
	if errors.Is(err, domain.ErrVerifierUnavailable) {
		t.Fatalf("wrong receiver is a definitive rejection, not manual fallback: %v", err)
	}
}

// M-Pesa verification goes to the dedicated /verify-mpesa endpoint (the
// universal /verify does not auto-detect M-Pesa) with {receiptNumber,
// phoneNumber} where phoneNumber is the HOUSE M-Pesa number in canonical 251
// form (the house is the receiver of a deposit, so nothing is needed from the
// player beyond the receipt number). The documented response is {success,
// data: {receiptNumber, transactionDate, amount, sender, receiver, status}}
// with no provider field, so the provider falls back to the requested method.
func TestVerifierMpesa(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/verify-mpesa" {
			t.Fatalf("path = %q, want /verify-mpesa", r.URL.Path)
		}
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["receiptNumber"] != "SGH12ABCD3" {
			t.Fatalf("receiptNumber = %q, want SGH12ABCD3", payload["receiptNumber"])
		}
		if payload["phoneNumber"] != "251712341122" {
			t.Fatalf("phoneNumber = %q, want canonical house number 251712341122", payload["phoneNumber"])
		}
		if _, ok := payload["reference"]; ok {
			t.Fatalf("mpesa request should use receiptNumber, not reference")
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"success": true,
			"data": {
				"receiptNumber": "SGH12ABCD3",
				"transactionDate": "2026-07-12 14:30:00",
				"amount": "75.00",
				"sender": "251712345678 - PLAYER NAME",
				"receiver": "2517****1122 - HOUSE NAME",
				"status": "Success"
			}
		}`))
	}))
	defer server.Close()

	// House number configured in local form; canonicalized for the lookup.
	verifier := NewVerifier(config.PaymentVerifierConfig{BaseURL: server.URL, APIKey: "test-key", MpesaAccount: "0712341122"})
	result, err := verifier.Verify(context.Background(), domain.PaymentVerificationRequest{
		Method:    domain.PaymentMethodMpesa,
		Reference: "SGH12ABCD3",
	})
	if err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
	if result.Provider != domain.PaymentMethodMpesa {
		t.Fatalf("provider = %q, want Mpesa", result.Provider)
	}
	if result.Amount != 75 {
		t.Fatalf("amount = %v, want 75", result.Amount)
	}
	if result.Reference != "SGH12ABCD3" {
		t.Fatalf("reference = %q, want SGH12ABCD3", result.Reference)
	}
}

// An M-Pesa receipt whose receiver is not the house account is rejected even
// though the receipt itself is genuine — the payer's own digits in "sender"
// must never satisfy the binding.
func TestVerifierMpesaRejectsWrongReceiver(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"success": true,
			"data": {
				"receiptNumber": "SGH12ABCD3",
				"amount": "75.00",
				"sender": "251712341122 - PLAYER NAME",
				"receiver": "2517****9999 - SOMEONE ELSE",
				"status": "Success"
			}
		}`))
	}))
	defer server.Close()

	// Note the sender's digits equal the house number — binding must still fail
	// because only the receiver side counts.
	verifier := NewVerifier(config.PaymentVerifierConfig{BaseURL: server.URL, APIKey: "test-key", MpesaAccount: "0712341122"})
	_, err := verifier.Verify(context.Background(), domain.PaymentVerificationRequest{
		Method:    domain.PaymentMethodMpesa,
		Reference: "SGH12ABCD3",
	})
	if err == nil {
		t.Fatal("receipt credited to a different number should be rejected")
	}
	if errors.Is(err, domain.ErrVerifierUnavailable) {
		t.Fatalf("wrong receiver is a definitive rejection, not manual fallback: %v", err)
	}
}

// When the M-Pesa response reveals the receiver only as a holder name (no
// digits), we cannot prove the money reached the house — fall back to manual
// admin review, never auto-credit.
func TestVerifierMpesaNameOnlyReceiverFallsBackToManual(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"success": true,
			"data": {
				"receiptNumber": "SGH12ABCD3",
				"amount": "75.00",
				"sender": "John Doe",
				"receiver": "Jane Doe",
				"status": "Success"
			}
		}`))
	}))
	defer server.Close()

	verifier := NewVerifier(config.PaymentVerifierConfig{BaseURL: server.URL, APIKey: "test-key", MpesaAccount: "0712341122"})
	_, err := verifier.Verify(context.Background(), domain.PaymentVerificationRequest{
		Method:    domain.PaymentMethodMpesa,
		Reference: "SGH12ABCD3",
	})
	if !errors.Is(err, domain.ErrVerifierUnavailable) {
		t.Fatalf("expected manual-review fallback (ErrVerifierUnavailable), got %v", err)
	}
}

// Like CBE Birr, an M-Pesa receipt cannot even be looked up without the house
// number — fall back to manual admin review, never auto-credit.
func TestVerifierMpesaNoHouseAccountFallsBackToManual(t *testing.T) {
	verifier := NewVerifier(config.PaymentVerifierConfig{BaseURL: "http://unused.invalid", APIKey: "test-key"})
	_, err := verifier.Verify(context.Background(), domain.PaymentVerificationRequest{
		Method:    domain.PaymentMethodMpesa,
		Reference: "SGH12ABCD3",
	})
	if !errors.Is(err, domain.ErrVerifierUnavailable) {
		t.Fatalf("expected manual-review fallback (ErrVerifierUnavailable), got %v", err)
	}
}

// The dedicated M-Pesa endpoint is flaky: a real receipt sometimes comes back as
// a raw non-2xx. That must NOT bounce the player — it falls back to manual admin
// review so a genuine deposit can still be approved.
func TestVerifierMpesaNon2xxFallsBackToManual(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"success": false, "message": "receipt not found"}`))
	}))
	defer server.Close()

	verifier := NewVerifier(config.PaymentVerifierConfig{BaseURL: server.URL, APIKey: "test-key", MpesaAccount: "0712341122"})
	_, err := verifier.Verify(context.Background(), domain.PaymentVerificationRequest{
		Method:    domain.PaymentMethodMpesa,
		Reference: "SGH12ABCD3",
	})
	if !errors.Is(err, domain.ErrVerifierUnavailable) {
		t.Fatalf("expected manual-review fallback (ErrVerifierUnavailable) for a dedicated-endpoint non-2xx, got %v", err)
	}
}

// The mature universal /verify (Telebirr) is trusted, so a non-2xx there stays a
// definitive rejection — the dedicated-endpoint leniency must not leak to it.
func TestVerifierTelebirrNon2xxIsDefinitive(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"success": false, "message": "receipt not found"}`))
	}))
	defer server.Close()

	verifier := NewVerifier(config.PaymentVerifierConfig{BaseURL: server.URL, APIKey: "test-key", TelebirrAccount: "0997709691"})
	_, err := verifier.Verify(context.Background(), telebirrReq("CE626EJRNS"))
	if err == nil {
		t.Fatal("a 404 from the universal /verify should be a definitive rejection")
	}
	if errors.Is(err, domain.ErrVerifierUnavailable) {
		t.Fatalf("Telebirr non-2xx must stay definitive, not manual fallback: %v", err)
	}
}

func TestVerifierAccountBinding(t *testing.T) {
	body := `{
		"success": true,
		"provider": "telebirr",
		"data": {
			"transactionStatus": "Completed",
			"receiptNo": "DFU3F35PH3",
			"creditedPartyAccountNo": "2519****9691",
			"settledAmount": "20 Birr"
		}
	}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	// Matching house account → accepted.
	ok := NewVerifier(config.PaymentVerifierConfig{BaseURL: server.URL, APIKey: "k", TelebirrAccount: "0997709691"})
	if _, err := ok.Verify(context.Background(), telebirrReq("DFU3F35PH3")); err != nil {
		t.Fatalf("matching account should verify, got %v", err)
	}

	// Receipt credited to a different account → rejected.
	bad := NewVerifier(config.PaymentVerifierConfig{BaseURL: server.URL, APIKey: "k", TelebirrAccount: "0911112222"})
	if _, err := bad.Verify(context.Background(), telebirrReq("DFU3F35PH3")); err == nil {
		t.Fatal("receipt paid to a different account should be rejected")
	}

	// A CBE Birr house number must not gate a Telebirr receipt: binding is looked
	// up per resolved provider, so an unrelated method's (wrong) account is
	// ignored as long as the receipt's own method matches its house account.
	crossed := NewVerifier(config.PaymentVerifierConfig{
		BaseURL: server.URL, APIKey: "k",
		TelebirrAccount: "0997709691", CBEBirrAccount: "0911112222",
	})
	if _, err := crossed.Verify(context.Background(), telebirrReq("DFU3F35PH3")); err != nil {
		t.Fatalf("CBE house account must not gate a Telebirr receipt, got %v", err)
	}
}

// Binding is mandatory for auto-approval: a method with no configured house
// account cannot prove the money reached us, so the receipt is treated as
// unverifiable (manual admin review) — never auto-credited, never hard-rejected.
func TestVerifierNoHouseAccountFallsBackToManual(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"success": true,
			"provider": "telebirr",
			"data": {
				"transactionStatus": "Completed",
				"receiptNo": "DFU3F35PH3",
				"creditedPartyAccountNo": "2519****9691",
				"settledAmount": "20 Birr"
			}
		}`))
	}))
	defer server.Close()

	verifier := NewVerifier(config.PaymentVerifierConfig{BaseURL: server.URL, APIKey: "k"})
	_, err := verifier.Verify(context.Background(), telebirrReq("DFU3F35PH3"))
	if !errors.Is(err, domain.ErrVerifierUnavailable) {
		t.Fatalf("expected manual-review fallback (ErrVerifierUnavailable), got %v", err)
	}
}

// Same fail-safe when the house account IS configured but the response does not
// reveal who was credited: we cannot prove the money reached the house, so fall
// back to manual review rather than auto-crediting or wrongly rejecting.
func TestVerifierMissingCreditedAccountFallsBackToManual(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"success": true,
			"provider": "telebirr",
			"data": {
				"transactionStatus": "Completed",
				"receiptNo": "DFU3F35PH3",
				"settledAmount": "20 Birr"
			}
		}`))
	}))
	defer server.Close()

	verifier := NewVerifier(config.PaymentVerifierConfig{BaseURL: server.URL, APIKey: "k", TelebirrAccount: "0997709691"})
	_, err := verifier.Verify(context.Background(), telebirrReq("DFU3F35PH3"))
	if !errors.Is(err, domain.ErrVerifierUnavailable) {
		t.Fatalf("expected manual-review fallback (ErrVerifierUnavailable), got %v", err)
	}
}

func TestVerifierUnavailableOnServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("upstream down"))
	}))
	defer server.Close()

	verifier := NewVerifier(config.PaymentVerifierConfig{BaseURL: server.URL, APIKey: "test-key"})
	_, err := verifier.Verify(context.Background(), telebirrReq("CE626EJRNS"))
	if !errors.Is(err, domain.ErrVerifierUnavailable) {
		t.Fatalf("expected ErrVerifierUnavailable, got %v", err)
	}
}

func TestVerifierRejectsUnsupportedProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"success": true,
			"provider": "dashen",
			"data": {"amount": 100}
		}`))
	}))
	defer server.Close()

	verifier := NewVerifier(config.PaymentVerifierConfig{BaseURL: server.URL, APIKey: "test-key"})
	if _, err := verifier.Verify(context.Background(), telebirrReq("CE626EJRNS")); err == nil {
		t.Fatal("expected unsupported provider error")
	}
}

// cbeServer returns a test server answering /verify-cbebirr with a genuine,
// house-credited receipt whose holder name is "TEST USER NAME".
func cbeServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"creditAccount": "251912345678 - TEST USER NAME",
			"receiverName": "251912345678 - TEST USER NAME",
			"transactionStatus": "Completed",
			"receiptNumber": "TEST123ABC2025",
			"reference": "FT25211JYPQX",
			"amount": "500.00"
		}`))
	}))
}

// #2: when the configured house holder name matches the receipt's credited-party
// name, the deposit verifies as normal (name check passes on top of the number).
func TestVerifierReceiverNameMatches(t *testing.T) {
	server := cbeServer(t)
	defer server.Close()

	verifier := NewVerifier(config.PaymentVerifierConfig{
		BaseURL:        server.URL,
		APIKey:         "k",
		CBEBirrAccount: "0912345678",
		CBEBirrName:    "Test User Name",
	})
	result, err := verifier.Verify(context.Background(), domain.PaymentVerificationRequest{
		Method:    domain.PaymentMethodCBEBirr,
		Reference: "TEST123ABC",
	})
	if err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
	if result.Amount != 500 {
		t.Fatalf("amount = %v, want 500", result.Amount)
	}
}

// #2: a genuine, house-credited receipt whose holder name does NOT match the
// configured house name is rejected — defence against a look-alike account
// number that happens to match on trailing digits.
func TestVerifierReceiverNameMismatchRejected(t *testing.T) {
	server := cbeServer(t)
	defer server.Close()

	verifier := NewVerifier(config.PaymentVerifierConfig{
		BaseURL:        server.URL,
		APIKey:         "k",
		CBEBirrAccount: "0912345678",
		CBEBirrName:    "Different Company PLC",
	})
	_, err := verifier.Verify(context.Background(), domain.PaymentVerificationRequest{
		Method:    domain.PaymentMethodCBEBirr,
		Reference: "TEST123ABC",
	})
	if err == nil {
		t.Fatal("expected rejection on holder-name mismatch, got nil")
	}
	if errors.Is(err, domain.ErrVerifierUnavailable) {
		t.Fatalf("name mismatch should be a hard reject, not manual fallback: %v", err)
	}
}

// fakeRecorder captures verification-log entries for assertions.
type fakeRecorder struct{ entries []*domain.VerificationLog }

func (f *fakeRecorder) Record(_ context.Context, e *domain.VerificationLog) {
	f.entries = append(f.entries, e)
}

// The audit recorder is invoked with the verdict and raw body for every lookup —
// a verified receipt records outcome=verified with the raw response captured.
func TestVerifierRecordsAuditLog(t *testing.T) {
	server := cbeServer(t)
	defer server.Close()

	rec := &fakeRecorder{}
	verifier := NewVerifierWithRecorder(config.PaymentVerifierConfig{
		BaseURL:        server.URL,
		APIKey:         "k",
		CBEBirrAccount: "0912345678",
	}, rec)

	if _, err := verifier.Verify(context.Background(), domain.PaymentVerificationRequest{
		Method:    domain.PaymentMethodCBEBirr,
		Reference: "TEST123ABC",
	}); err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}

	if len(rec.entries) != 1 {
		t.Fatalf("recorded %d entries, want 1", len(rec.entries))
	}
	got := rec.entries[0]
	if got.Outcome != domain.VerificationVerified {
		t.Fatalf("outcome = %q, want verified", got.Outcome)
	}
	if got.RawResponse == "" {
		t.Fatal("raw response was not captured in the audit log")
	}
	if got.Amount == nil || *got.Amount != 500 {
		t.Fatalf("amount = %v, want 500", got.Amount)
	}
}

// A definitive rejection is recorded as outcome=rejected (not unavailable).
func TestVerifierRecordsRejection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success": false, "message": "not found"}`))
	}))
	defer server.Close()

	rec := &fakeRecorder{}
	verifier := NewVerifierWithRecorder(config.PaymentVerifierConfig{
		BaseURL: server.URL, APIKey: "k", TelebirrAccount: "0997709691",
	}, rec)

	if _, err := verifier.Verify(context.Background(), telebirrReq("CE626EJRNS")); err == nil {
		t.Fatal("expected rejection")
	}
	if len(rec.entries) != 1 || rec.entries[0].Outcome != domain.VerificationRejected {
		t.Fatalf("want one rejected entry, got %+v", rec.entries)
	}
}

// #1: Available() reflects host reachability so deposit entry can fail closed.
func TestVerifierAvailable(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer up.Close()

	reachable := NewVerifier(config.PaymentVerifierConfig{BaseURL: up.URL, APIKey: "k"})
	if !reachable.Available(context.Background()) {
		t.Fatal("expected Available()=true for a reachable verifier host")
	}

	// A host that never answers (closed server) is unavailable → deposits fail closed.
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	down.Close()
	unreachable := NewVerifier(config.PaymentVerifierConfig{BaseURL: down.URL, APIKey: "k"})
	if unreachable.Available(context.Background()) {
		t.Fatal("expected Available()=false for an unreachable verifier host")
	}
}
