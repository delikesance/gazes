package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/gazes/gazes/internal/donations"
	"github.com/go-chi/chi/v5"
)

// WithDonations mounts the public donation routes and the provider webhooks.
func WithDonations(svc *donations.Service) Option { return func(s *Server) { s.donations = svc } }

const (
	donationBodyMax  = 4 << 10
	webhookBodyMax   = 64 << 10
	publicDonorLimit = 60
)

func (s *Server) mountDonations(api chi.Router) {
	api.Get("/donations", s.handleDonationsPublic)
	api.With(s.rateLimit("donation-invoice", 10, time.Hour)).Post("/donations/btcpay/invoice", s.handleBTCPayInvoice)
	// Webhooks are authenticated by signature / token, not by session; the limit only caps abuse.
	api.With(s.rateLimit("donation-webhook", 120, time.Minute)).Post("/donations/webhooks/btcpay", s.handleBTCPayWebhook)
	api.With(s.rateLimit("donation-webhook", 120, time.Minute)).Post("/donations/webhooks/kofi", s.handleKofiWebhook)
}

type publicDonations struct {
	BTCPay     bool     `json:"btcpay"`
	KofiURL    string   `json:"kofi_url,omitempty"`
	GoalCents  int64    `json:"goal_cents,omitempty"`
	MonthCents *int64   `json:"month_cents,omitempty"` // only when a goal is configured
	MinCents   int64    `json:"min_cents"`
	MaxCents   int64    `json:"max_cents"`
	Donors     []string `json:"donors"` // names chosen by their owners; never anything else
}

// GET /api/v1/donations: what the public page needs. No account, label, reference or amount per donor.
func (s *Server) handleDonationsPublic(w http.ResponseWriter, r *http.Request) {
	cfg := s.donations.Config()
	out := publicDonations{BTCPay: s.donations.BTCPayEnabled(), GoalCents: cfg.GoalCents,
		MinCents: donations.MinAmountCents, MaxCents: donations.MaxAmountCents, Donors: []string{}}
	if s.donations.KofiEnabled() {
		out.KofiURL = cfg.KofiURL
	}
	if names, err := s.donations.Store.PublicNames(r.Context(), publicDonorLimit); err == nil {
		out.Donors = names
	}
	if cfg.GoalCents > 0 {
		now := time.Now().UTC()
		since := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).Unix()
		if sum, err := s.donations.Store.Summary(r.Context(), since); err == nil {
			out.MonthCents = &sum.MonthCents
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSONStatus(w, http.StatusOK, out)
}

func writeJSONStatus(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func donationError(w http.ResponseWriter, status int, code string) {
	writeJSONStatus(w, status, map[string]string{"error": code})
}

// POST /api/v1/donations/btcpay/invoice {amount_cents, visibility, display_name}
func (s *Server) handleBTCPayInvoice(w http.ResponseWriter, r *http.Request) {
	if !s.donations.BTCPayEnabled() {
		donationError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	var in struct {
		AmountCents int64  `json:"amount_cents"`
		Visibility  string `json:"visibility"`
		DisplayName string `json:"display_name"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, donationBodyMax))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		donationError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if in.AmountCents < donations.MinAmountCents || in.AmountCents > donations.MaxAmountCents {
		donationError(w, http.StatusBadRequest, "invalid_amount")
		return
	}
	d := donations.Donation{ID: donations.NewID(), Provider: donations.ProviderBTCPay, Visibility: donations.VisAnonymous,
		AmountCents: in.AmountCents, Currency: donations.CurrencyEUR, Status: donations.StatusPending, CreatedAt: time.Now().Unix()}
	d.ProviderRef = "pending:" + d.ID
	if in.Visibility == donations.VisNamed {
		name, err := donations.SanitizeName(in.DisplayName)
		if err != nil {
			donationError(w, http.StatusBadRequest, "invalid_name")
			return
		}
		d.Visibility, d.DisplayName = donations.VisNamed, name
	}
	if uid, ok := s.libraryUserID(r); ok {
		d.UserID = &uid
	}
	if _, _, err := s.donations.Store.Insert(r.Context(), d); err != nil {
		donationError(w, http.StatusInternalServerError, "server_error")
		return
	}
	id, link, err := s.donations.CreateInvoice(r.Context(), d.ID, d.AmountCents)
	if err != nil {
		s.logger.Warn("donations: btcpay invoice failed", "err", err)
		_ = s.donations.Store.Expire(r.Context(), donations.ProviderBTCPay, d.ProviderRef)
		donationError(w, http.StatusBadGateway, "provider_unavailable")
		return
	}
	if err := s.donations.Store.SetProviderRef(r.Context(), d.ID, id); err != nil {
		donationError(w, http.StatusInternalServerError, "server_error")
		return
	}
	writeJSONStatus(w, http.StatusOK, map[string]string{"checkout_url": link})
}

// POST /api/v1/donations/webhooks/btcpay: signed by BTCPay (HMAC-SHA256 of the raw body). The payload
// only names the invoice; status and amount are read back from BTCPay before anything is recorded.
func (s *Server) handleBTCPayWebhook(w http.ResponseWriter, r *http.Request) {
	if !s.donations.BTCPayEnabled() {
		donationError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, webhookBodyMax))
	if err != nil {
		donationError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if !donations.VerifyBTCPaySignature(body, r.Header.Get("BTCPay-Sig"), s.donations.Config().BTCPayWebhookSecret) {
		donationError(w, http.StatusUnauthorized, "bad_signature")
		return
	}
	var ev struct {
		Type      string `json:"type"`
		InvoiceID string `json:"invoiceId"`
	}
	if json.Unmarshal(body, &ev) != nil || ev.InvoiceID == "" {
		donationError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	switch ev.Type {
	case "InvoiceSettled":
		inv, err := s.donations.GetInvoice(r.Context(), ev.InvoiceID)
		if err != nil {
			s.logger.Warn("donations: btcpay invoice lookup failed", "err", err)
			donationError(w, http.StatusBadGateway, "provider_unavailable") // BTCPay redelivers
			return
		}
		if inv.Status != "Settled" {
			break
		}
		changed, err := s.donations.Store.Settle(r.Context(), donations.ProviderBTCPay, ev.InvoiceID, inv.AmountCents, inv.Currency, time.Now())
		if err != nil {
			donationError(w, http.StatusInternalServerError, "server_error")
			return
		}
		if !changed {
			// Replayed webhook, or a settled invoice with no pending row: worth a line to spot an unattached payment.
			s.logger.Info("donations: btcpay settle changed no row", "invoice", ev.InvoiceID)
		}
	case "InvoiceExpired", "InvoiceInvalid":
		if err := s.donations.Store.Expire(r.Context(), donations.ProviderBTCPay, ev.InvoiceID); err != nil {
			donationError(w, http.StatusInternalServerError, "server_error")
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}

// POST /api/v1/donations/webhooks/kofi: form field `data` holds the JSON, checked against the shared token.
func (s *Server) handleKofiWebhook(w http.ResponseWriter, r *http.Request) {
	if !s.donations.KofiWebhookEnabled() {
		donationError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, webhookBodyMax)
	if err := r.ParseForm(); err != nil {
		donationError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	d, err := s.donations.KofiDonation(r.PostForm.Get("data"))
	switch {
	case errors.Is(err, donations.ErrKofiIgnored):
		w.WriteHeader(http.StatusOK)
		return
	case err != nil:
		s.logger.Warn("donations: ko-fi webhook refused", "err", err.Error())
		donationError(w, http.StatusUnauthorized, "refused")
		return
	}
	if _, _, err := s.donations.Store.Insert(r.Context(), d); err != nil {
		donationError(w, http.StatusInternalServerError, "server_error")
		return
	}
	w.WriteHeader(http.StatusOK)
}
