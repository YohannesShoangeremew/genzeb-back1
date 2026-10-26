package payment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bingo/backend/config"
	"github.com/bingo/backend/internal/domain"
	"github.com/google/uuid"
)

// healthTTL is how long an availability probe result is cached.
const healthTTL = 20 * time.Second

var numberPattern = regexp.MustCompile(`[-+]?\d+(\.\d+)?`)

type Verifier struct {
	baseURL       string
	apiKey        string
	houseAccounts map[domain.PaymentMethod]string
	houseNames    map[domain.PaymentMethod]string
	debugLog      bool
	recorder      domain.VerificationRecorder
	client        *http.Client

	mu              sync.Mutex
	healthy         bool
	healthCheckedAt time.Time
}

func NewVerifier(cfg config.PaymentVerifierConfig) domain.PaymentVerifier {
	return NewVerifierWithRecorder(cfg, nil)
}

func NewVerifierWithRecorder(cfg config.PaymentVerifierConfig, recorder domain.VerificationRecorder) domain.PaymentVerifier {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil
	}

	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://verify.et"
	}

	houseAccounts := map[domain.PaymentMethod]string{}
	if acct := onlyDigits(cfg.TelebirrAccount); acct != "" {
		houseAccounts[domain.PaymentMethodTelebirr] = acct
	}
	if acct := onlyDigits(cfg.CBEBirrAccount); acct != "" {
		houseAccounts[domain.PaymentMethodCBEBirr] = acct
	}
	if acct := onlyDigits(cfg.MpesaAccount); acct != "" {
		houseAccounts[domain.PaymentMethodMpesa] = acct
	}

	houseNames := map[domain.PaymentMethod]string{}
	if name := normalizeName(cfg.TelebirrName); name != "" {
		houseNames[domain.PaymentMethodTelebirr] = name
	}
	if name := normalizeName(cfg.CBEBirrName); name != "" {
		houseNames[domain.PaymentMethodCBEBirr] = name
	}
	if name := normalizeName(cfg.MpesaName); name != "" {
		houseNames[domain.PaymentMethodMpesa] = name
	}

	return &Verifier{
		baseURL:       baseURL,
		apiKey:        cfg.APIKey,
		houseAccounts: houseAccounts,
		houseNames:    houseNames,
		debugLog:      cfg.DebugLog,
		recorder:      recorder,
		client: &http.Client{
			Timeout: 45 * time.Second, // 45s client timeout recommended by Verify.ET for OCR processing
		},
	}
}

func (v *Verifier) Available(ctx context.Context) bool {
	if v == nil {
		return false
	}

	v.mu.Lock()
	if !v.healthCheckedAt.IsZero() && time.Since(v.healthCheckedAt) < healthTTL {
		cached := v.healthy
		v.mu.Unlock()
		return cached
	}
	v.mu.Unlock()

	healthy := v.probe(ctx)

	v.mu.Lock()
	v.healthy = healthy
	v.healthCheckedAt = time.Now()
	v.mu.Unlock()
	return healthy
}

func (v *Verifier) probe(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.baseURL, nil)
	if err != nil {
		return false
	}
	req.Header.Set("x-api-key", v.apiKey)

	resp, err := v.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	return resp.StatusCode < 500
}

func (v *Verifier) Verify(ctx context.Context, req domain.PaymentVerificationRequest) (result *domain.PaymentVerificationResult, err error) {
	if v == nil {
		return nil, errors.New("payment verifier is not configured")
	}

	var rawBody string
	defer func() { v.record(ctx, req, result, rawBody, err) }()

	if !domain.IsSupportedPaymentMethod(req.Method) {
		return nil, errors.New("unsupported payment method")
	}

	reference := strings.TrimSpace(req.Reference)
	if len(reference) > 50 {
		return nil, errors.New("invalid transaction reference: please enter only your transaction ID, not the full SMS text")
	}

	bankCode := ""
	switch req.Method {
	case domain.PaymentMethodTelebirr:
		bankCode = "telebirr"
	case domain.PaymentMethodCBEBirr:
		bankCode = "cbe"
	case domain.PaymentMethodMpesa:
		bankCode = "mpesa"
	default:
		return nil, errors.New("unsupported payment method for verify.et")
	}

	// Verify.ET endpoint with 5s inline wait
	endpoint := "/api/verify?waitMs=5000"
	payload := map[string]any{
		"bank":      bankCode,
		"reference": reference,
	}

	if house := v.houseAccounts[req.Method]; house != "" {
		payload["settlementAccount"] = house
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to encode verifier request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, v.baseURL+endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to build verifier request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", v.apiKey)
	httpReq.Header.Set("Idempotency-Key", uuid.New().String())

	resp, err := v.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: verifier request failed: %v", domain.ErrVerifierUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusAccepted {
		return nil, fmt.Errorf("%w: verification queued (202), falling back to manual review", domain.ErrVerifierUnavailable)
	}

	if isTransientStatus(resp.StatusCode) {
		return nil, fmt.Errorf("%w: verifier returned status %d", domain.ErrVerifierUnavailable, resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read verifier response: %v", domain.ErrVerifierUnavailable, err)
	}
	rawBody = string(raw)

	if v.debugLog {
		log.Printf("[verify] method=%s ref=%s status=%d raw=%s", req.Method, reference, resp.StatusCode, string(raw))
	}

	var decoded map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("%w: failed to decode verifier response: %v", domain.ErrVerifierUnavailable, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("verifier returned status %d: %s", resp.StatusCode, responseMessage(decoded))
	}

	if success, ok := boolValue(decoded["success"]); ok && !success {
		return nil, fmt.Errorf("receipt was not verified: %s", responseMessage(decoded))
	}

	data := mapValue(decoded["data"])
	if len(data) == 0 {
		data = decoded
	}

	provider := normalizeProvider(stringValue(data["bank"]), req.Method)
	if provider == "" {
		provider = req.Method
	}

	status := firstString(data, "status", "transactionStatus", "processingStatus")
	if status != "" && !isCompletedStatus(status) && status != "completed" {
		if status == "pending" || status == "queued" {
			return nil, fmt.Errorf("%w: verification still pending, falling back to manual review", domain.ErrVerifierUnavailable)
		}
	}

	houseAccount := v.houseAccounts[provider]
	if houseAccount != "" {
		credited := firstString(data,
			"creditedPartyAccountNo", "creditedAccountNo", "creditedPartyAccount",
			"creditAccount", "receiverAccount", "receiverPhone", "receiver",
			"payeeAccount", "to", "settlementAccount")
		if hasAccountDigits(credited) && !accountMatches(houseAccount, credited) {
			return nil, fmt.Errorf("receipt was paid to a different account (%s), not the house account", credited)
		}
	}

	amount, err := responseAmount(decoded, data)
	if err != nil {
		return nil, err
	}

	verifiedReference := firstString(data, "reference", "receiptNo", "receiptNumber", "transactionReference", "referenceNumber")
	if verifiedReference == "" {
		verifiedReference = reference
	}

	return &domain.PaymentVerificationResult{
		Provider:  provider,
		Reference: verifiedReference,
		Amount:    amount,
		Status:    "completed",
	}, nil
}

func (v *Verifier) record(ctx context.Context, req domain.PaymentVerificationRequest, result *domain.PaymentVerificationResult, raw string, err error) {
	if v.recorder == nil {
		return
	}

	outcome := domain.VerificationVerified
	reason := "ok"
	switch {
	case err == nil:
	case errors.Is(err, domain.ErrVerifierUnavailable):
		outcome = domain.VerificationUnavailable
		reason = err.Error()
	default:
		outcome = domain.VerificationRejected
		reason = err.Error()
	}

	entry := &domain.VerificationLog{
		Method:      req.Method,
		Reference:   req.Reference,
		Outcome:     outcome,
		Reason:      reason,
		RawResponse: raw,
	}
	if req.UserID != uuid.Nil {
		uid := req.UserID
		entry.UserID = &uid
	}
	if result != nil {
		amt := result.Amount
		entry.Amount = &amt
	}
	v.recorder.Record(ctx, entry)
}

var amountKeys = []string{"settledAmount", "amount", "transactionAmount", "paidAmount", "total", "totalPaidAmount"}

func responseAmount(root, data map[string]any) (float64, error) {
	for _, key := range amountKeys {
		if amount, ok := parseAmount(data[key]); ok {
			return amount, nil
		}
	}
	for _, key := range amountKeys {
		if amount, ok := parseAmount(root[key]); ok {
			return amount, nil
		}
	}
	return 0, errors.New("payment verifier did not return amount")
}

func parseAmount(value any) (float64, bool) {
	switch v := value.(type) {
	case json.Number:
		amount, err := v.Float64()
		return amount, err == nil
	case float64:
		return v, true
	case int:
		return float64(v), true
	case string:
		cleaned := strings.ReplaceAll(v, ",", "")
		match := numberPattern.FindString(cleaned)
		if match == "" {
			return 0, false
		}
		amount, err := strconv.ParseFloat(match, 64)
		return amount, err == nil
	default:
		return 0, false
	}
}

func normalizeProvider(value string, fallback domain.PaymentMethod) domain.PaymentMethod {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), " ", ""))
	normalized = strings.ReplaceAll(normalized, "-", "")
	switch normalized {
	case "telebirr", "telebirrmobilemoney":
		return domain.PaymentMethodTelebirr
	case "cbebirr", "cbe":
		return domain.PaymentMethodCBEBirr
	case "mpesa", "safaricommpesa":
		return domain.PaymentMethodMpesa
	case "":
		return fallback
	}
	return ""
}

func isTransientStatus(code int) bool {
	switch code {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusRequestTimeout, http.StatusTooManyRequests:
		return true
	}
	return code >= 500
}

var digitRuns = regexp.MustCompile(`\d+`)

func accountMatches(houseDigits, credited string) bool {
	for _, run := range digitRuns.FindAllString(credited, -1) {
		if len(run) >= 4 && len(run) <= len(houseDigits) && strings.HasSuffix(houseDigits, run) {
			return true
		}
		if len(run) >= 9 && len(houseDigits) >= 9 && run[len(run)-9:] == houseDigits[len(houseDigits)-9:] {
			return true
		}
	}
	return false
}

var nonNameChars = regexp.MustCompile(`[^A-Za-z\s]+`)
var whitespaceRun = regexp.MustCompile(`\s+`)

func normalizeName(s string) string {
	s = nonNameChars.ReplaceAllString(s, " ")
	s = whitespaceRun.ReplaceAllString(s, " ")
	return strings.ToUpper(strings.TrimSpace(s))
}

func hasAccountDigits(s string) bool {
	for _, run := range digitRuns.FindAllString(s, -1) {
		if len(run) >= 4 {
			return true
		}
	}
	return false
}

func onlyDigits(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

func isCompletedStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "complete", "success", "successful":
		return true
	default:
		return false
	}
}

func mapValue(value any) map[string]any {
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	return map[string]any{}
}

func boolValue(value any) (bool, bool) {
	if typed, ok := value.(bool); ok {
		return typed, true
	}
	return false, false
}

func firstString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringValue(values[key]); value != "" {
			return value
		}
	}
	return ""
}

func stringValue(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case json.Number:
		return v.String()
	default:
		return ""
	}
}

func responseMessage(values map[string]any) string {
	for _, key := range []string{"error", "message", "details"} {
		if value := stringValue(values[key]); value != "" {
			return value
		}
	}
	return "unknown verifier response"
}