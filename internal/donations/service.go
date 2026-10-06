package donations

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	MinAmountCents = 100   // 1 €
	MaxAmountCents = 50000 // 500 €
	maxNameRunes   = 24
	minNameRunes   = 2
	maxMessageLen  = 500
)

// Config is the provider setup. Every field is optional: a provider with missing fields is off.
type Config struct {
	SiteURL             string // public origin, for the post-payment redirect
	BTCPayURL           string
	BTCPayStoreID       string
	BTCPayAPIKey        string
	BTCPayWebhookSecret string
	KofiURL             string // public donation page
	KofiToken           string // webhook verification token
	GoalCents           int64  // monthly goal shown publicly (0 = none)
}

// Service is the store plus the provider clients.
type Service struct {
	Store *Store
	cfg   Config
	http  *http.Client
	now   func() time.Time
}

func NewService(store *Store, cfg Config) *Service {
	cfg.BTCPayURL = strings.TrimRight(cfg.BTCPayURL, "/")
	cfg.SiteURL = strings.TrimRight(cfg.SiteURL, "/")
	return &Service{Store: store, cfg: cfg, http: &http.Client{Timeout: 10 * time.Second}, now: time.Now}
}

// SetHTTPClient replaces the client used for BTCPay calls (tests).
func (s *Service) SetHTTPClient(c *http.Client) { s.http = c }

func (s *Service) Config() Config { return s.cfg }

func (s *Service) BTCPayEnabled() bool {
	c := s.cfg
	return c.BTCPayURL != "" && c.BTCPayStoreID != "" && c.BTCPayAPIKey != "" && c.BTCPayWebhookSecret != "" && c.SiteURL != ""
}

func (s *Service) KofiEnabled() bool { return s.cfg.KofiURL != "" }

func (s *Service) Enabled() bool { return s.BTCPayEnabled() || s.KofiEnabled() }

// KofiWebhookEnabled reports whether Ko-fi webhooks can be verified.
func (s *Service) KofiWebhookEnabled() bool { return s.cfg.KofiToken != "" }

var (
	ErrBadName   = errors.New("display name must be 2-24 letters, digits, spaces, - _ '")
	ErrBadAmount = errors.New("amount out of range")
)

// SanitizeName validates a donor-chosen public name. It is deliberately strict: the name is shown
// to every visitor, so no links, markup or handles.
func SanitizeName(raw string) (string, error) {
	name := strings.Join(strings.Fields(raw), " ")
	n := utf8.RuneCountInString(name)
	if n < minNameRunes || n > maxNameRunes {
		return "", ErrBadName
	}
	for _, r := range name {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' || r == '-' || r == '_' || r == '\'' || r == '’') {
			return "", ErrBadName
		}
	}
	low := strings.ToLower(name)
	if strings.Contains(low, "http") || strings.Contains(low, "www") {
		return "", ErrBadName
	}
	return name, nil
}

// ParseCents reads a decimal amount ("3", "3.5", "3.00", "5.00000000") as cents. More than two
// significant decimals is an error: money is never rounded silently.
func ParseCents(s string) (int64, error) {
	s = strings.TrimSpace(s)
	whole, frac, _ := strings.Cut(s, ".")
	if whole == "" && frac == "" {
		return 0, fmt.Errorf("empty amount")
	}
	var cents int64
	for _, c := range whole {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid amount %q", s)
		}
		cents = cents*10 + int64(c-'0')
		if cents > 1<<40 {
			return 0, fmt.Errorf("amount too large")
		}
	}
	cents *= 100
	for i, c := range frac {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid amount %q", s)
		}
		switch {
		case i == 0:
			cents += int64(c-'0') * 10
		case i == 1:
			cents += int64(c - '0')
		case c != '0':
			return 0, fmt.Errorf("amount %q has sub-cent digits", s)
		}
	}
	return cents, nil
}

// FormatCents renders cents as "5.00".
func FormatCents(c int64) string { return fmt.Sprintf("%d.%02d", c/100, c%100) }

// VerifyBTCPaySignature checks the BTCPay-Sig header ("sha256=<hex>") against HMAC-SHA256(secret, body).
func VerifyBTCPaySignature(body []byte, header, secret string) bool {
	if secret == "" {
		return false
	}
	hexSig, ok := strings.CutPrefix(strings.TrimSpace(header), "sha256=")
	if !ok {
		return false
	}
	got, err := hex.DecodeString(hexSig)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(got, mac.Sum(nil))
}

func (s *Service) btcpayDo(ctx context.Context, method, path string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.cfg.BTCPayURL+"/api/v1/stores/"+url.PathEscape(s.cfg.BTCPayStoreID)+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+s.cfg.BTCPayAPIKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("btcpay: HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out)
}

// CreateInvoice opens a BTCPay invoice for an EUR amount and returns its id and checkout URL.
func (s *Service) CreateInvoice(ctx context.Context, donationID string, amountCents int64) (id, checkout string, err error) {
	if !s.BTCPayEnabled() {
		return "", "", errors.New("btcpay not configured")
	}
	in := map[string]any{
		"amount":   FormatCents(amountCents),
		"currency": CurrencyEUR,
		"metadata": map[string]string{"orderId": donationID, "itemDesc": "Soutien à Gazes"},
		"checkout": map[string]any{"redirectURL": s.cfg.SiteURL + "/soutenir/merci", "redirectAutomatically": false, "expirationMinutes": 60},
	}
	var out struct {
		ID           string `json:"id"`
		CheckoutLink string `json:"checkoutLink"`
	}
	if err := s.btcpayDo(ctx, http.MethodPost, "/invoices", in, &out); err != nil {
		return "", "", err
	}
	if out.ID == "" || out.CheckoutLink == "" {
		return "", "", errors.New("btcpay: incomplete invoice answer")
	}
	return out.ID, out.CheckoutLink, nil
}

// Invoice is what BTCPay reports for an invoice.
type Invoice struct {
	Status      string
	AmountCents int64
	Currency    string
}

// GetInvoice reads an invoice back from BTCPay (the webhook payload itself is never trusted).
func (s *Service) GetInvoice(ctx context.Context, id string) (Invoice, error) {
	var out struct {
		Status   string          `json:"status"`
		Amount   json.RawMessage `json:"amount"`
		Currency string          `json:"currency"`
	}
	if err := s.btcpayDo(ctx, http.MethodGet, "/invoices/"+url.PathEscape(id), nil, &out); err != nil {
		return Invoice{}, err
	}
	cents, err := ParseCents(strings.Trim(string(out.Amount), `"`))
	if err != nil {
		return Invoice{}, err
	}
	return Invoice{Status: out.Status, AmountCents: cents, Currency: out.Currency}, nil
}

// KofiPayload is the JSON a Ko-fi webhook carries in its `data` form field (only what is used:
// the e-mail and the other fields are never read).
type KofiPayload struct {
	VerificationToken string `json:"verification_token"`
	Type              string `json:"type"`
	IsPublic          bool   `json:"is_public"`
	FromName          string `json:"from_name"`
	Message           string `json:"message"`
	Amount            string `json:"amount"`
	Currency          string `json:"currency"`
	TransactionID     string `json:"kofi_transaction_id"`
	MessageID         string `json:"message_id"`
}

var ErrKofiIgnored = errors.New("ko-fi event ignored")

// KofiDonation verifies and converts a Ko-fi webhook `data` payload. Only donations and
// subscription payments count; shop orders and commissions are ignored (ErrKofiIgnored).
func (s *Service) KofiDonation(data string) (Donation, error) {
	var p KofiPayload
	if err := json.Unmarshal([]byte(data), &p); err != nil {
		return Donation{}, err
	}
	if s.cfg.KofiToken == "" || !hmac.Equal([]byte(p.VerificationToken), []byte(s.cfg.KofiToken)) {
		return Donation{}, errors.New("bad verification token")
	}
	if p.Type != "Donation" && p.Type != "Subscription" {
		return Donation{}, ErrKofiIgnored
	}
	ref := p.TransactionID
	if ref == "" {
		ref = p.MessageID
	}
	if ref == "" {
		return Donation{}, errors.New("ko-fi payload has no transaction id")
	}
	cents, err := ParseCents(p.Amount)
	if err != nil || cents <= 0 {
		return Donation{}, fmt.Errorf("bad ko-fi amount %q", p.Amount)
	}
	cur := strings.ToUpper(strings.TrimSpace(p.Currency))
	if cur == "" {
		cur = CurrencyEUR
	}
	now := s.now()
	at := now.Unix()
	d := Donation{
		Provider: ProviderKofi, ProviderRef: ref, DonorLabel: truncate(strings.TrimSpace(p.FromName), 80),
		Visibility: VisAnonymous, AmountCents: cents, Currency: cur, Status: StatusSettled,
		Message: truncate(strings.TrimSpace(p.Message), maxMessageLen), CreatedAt: at, SettledAt: &at,
	}
	if p.IsPublic {
		if name, err := SanitizeName(p.FromName); err == nil {
			d.Visibility, d.DisplayName = VisNamed, name
		}
	}
	return d, nil
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
