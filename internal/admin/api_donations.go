package admin

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gazes/gazes/internal/donations"
	"github.com/go-chi/chi/v5"
)

// SetDonations enables the donations pages of the panel. Not safe once requests are being served.
func (s *Service) SetDonations(st *donations.Store) { s.donations = st }

func (s *Service) mountDonations(r chi.Router) {
	// Who gave what is personal data: admin session only, never a Bearer token (so never the MCP).
	r.With(s.sessionOnlyRead).Get("/donations", s.handleDonationsList)
	r.With(s.sessionOnly).Post("/donations", s.handleDonationAdd)
	r.With(s.sessionOnly).Post("/donations/{id}/link", s.handleDonationLink)
	r.With(s.sessionOnly).Post("/donations/{id}/visibility", s.handleDonationVisibility)
}

type donationRow struct {
	ID          string  `json:"id"`
	Provider    string  `json:"provider"`
	ProviderRef string  `json:"provider_ref"`
	UserID      *int64  `json:"user_id"`
	Pseudo      *string `json:"pseudo"`
	DonorLabel  string  `json:"donor_label"`
	Visibility  string  `json:"visibility"`
	DisplayName string  `json:"display_name"`
	AmountCents int64   `json:"amount_cents"`
	Currency    string  `json:"currency"`
	Status      string  `json:"status"`
	Message     string  `json:"message"`
	CreatedAt   int64   `json:"created_at"`
	SettledAt   *int64  `json:"settled_at"`
}

func (s *Service) donationsReady(w http.ResponseWriter) bool {
	if s.donations == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "unavailable", "donations are not available")
		return false
	}
	return true
}

func monthStart(now time.Time) int64 {
	n := now.UTC()
	return time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, time.UTC).Unix()
}

// pseudos resolves account ids to pseudos from the read-only accounts database.
func (s *Service) pseudos(ctx context.Context, ids []int64) map[int64]string {
	out := map[int64]string{}
	if len(ids) == 0 || s.accountsDB() == nil {
		return out
	}
	q := `SELECT id, pseudo FROM users WHERE id IN (?` + strings.Repeat(",?", len(ids)-1) + `)`
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.accountsDB().QueryContext(ctx, q, args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var p string
		if rows.Scan(&id, &p) == nil {
			out[id] = p
		}
	}
	return out
}

func (s *Service) donationView(ctx context.Context, ds []donations.Donation) []donationRow {
	var ids []int64
	for _, d := range ds {
		if d.UserID != nil {
			ids = append(ids, *d.UserID)
		}
	}
	names := s.pseudos(ctx, ids)
	out := make([]donationRow, 0, len(ds))
	for _, d := range ds {
		row := donationRow{ID: d.ID, Provider: d.Provider, ProviderRef: d.ProviderRef, UserID: d.UserID, DonorLabel: d.DonorLabel,
			Visibility: d.Visibility, DisplayName: d.DisplayName, AmountCents: d.AmountCents, Currency: d.Currency,
			Status: d.Status, Message: d.Message, CreatedAt: d.CreatedAt, SettledAt: d.SettledAt}
		if d.UserID != nil {
			if p, ok := names[*d.UserID]; ok {
				row.Pseudo = &p
			}
		}
		out = append(out, row)
	}
	return out
}

// GET /donations?status=&limit=&offset= → {summary, donations}.
func (s *Service) handleDonationsList(w http.ResponseWriter, r *http.Request) {
	if !s.donationsReady(w) {
		return
	}
	status := r.URL.Query().Get("status")
	switch status {
	case "", donations.StatusPending, donations.StatusSettled, donations.StatusExpired:
	default:
		writeAPIError(w, http.StatusBadRequest, "bad_status", "status must be pending, settled or expired")
		return
	}
	page, err := parsePagination(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad_pagination", err.Error())
		return
	}
	list, err := s.donations.List(r.Context(), status, page.Limit, page.Offset)
	if err != nil {
		pbServerError(w, err)
		return
	}
	sum, err := s.donations.Summary(r.Context(), monthStart(s.now()))
	if err != nil {
		pbServerError(w, err)
		return
	}
	writeDataNoPeriod(w, map[string]any{"summary": sum, "donations": s.donationView(r.Context(), list), "limit": page.Limit, "offset": page.Offset})
}

type donationAddIn struct {
	AmountCents int64  `json:"amount_cents"`
	Currency    string `json:"currency"`
	DonorLabel  string `json:"donor_label"`
	UserID      *int64 `json:"user_id"`
	Visibility  string `json:"visibility"`
	DisplayName string `json:"display_name"`
	Message     string `json:"message"`
}

// POST /donations records a donation received outside the providers (transfer, Liberapay, cash).
func (s *Service) handleDonationAdd(w http.ResponseWriter, r *http.Request) {
	if !s.donationsReady(w) {
		return
	}
	var in donationAddIn
	if !issueDecode(w, r, &in) {
		return
	}
	cur := strings.ToUpper(strings.TrimSpace(in.Currency))
	if cur == "" {
		cur = donations.CurrencyEUR
	}
	if in.AmountCents <= 0 || in.AmountCents > 100_000_000 || len(cur) != 3 {
		writeAPIError(w, http.StatusBadRequest, "bad_amount", "amount_cents must be positive and currency a 3-letter code")
		return
	}
	if len(in.DonorLabel) > 80 || len(in.Message) > 500 {
		writeAPIError(w, http.StatusBadRequest, "too_long", "donor_label max 80, message max 500 characters")
		return
	}
	d := donations.Donation{Provider: donations.ProviderManual, DonorLabel: strings.TrimSpace(in.DonorLabel), UserID: in.UserID,
		Visibility: donations.VisAnonymous, AmountCents: in.AmountCents, Currency: cur, Status: donations.StatusSettled,
		Message: strings.TrimSpace(in.Message), CreatedAt: s.now().Unix()}
	d.SettledAt = &d.CreatedAt
	if in.Visibility == donations.VisNamed {
		name, err := donations.SanitizeName(in.DisplayName)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "bad_name", err.Error())
			return
		}
		d.Visibility, d.DisplayName = donations.VisNamed, name
	}
	d.ID = donations.NewID()
	d.ProviderRef = d.ID
	saved, _, err := s.donations.Insert(r.Context(), d)
	if err != nil {
		pbServerError(w, err)
		return
	}
	actor, _ := s.actorOf(r)
	s.opsAudit(r.Context(), 0, "donation:add", "by="+actor+" id="+saved.ID, "ok", 0)
	writeDataNoPeriod(w, s.donationView(r.Context(), []donations.Donation{saved})[0])
}

// POST /donations/{id}/link {user_id:int|null} attaches (or detaches) an account.
func (s *Service) handleDonationLink(w http.ResponseWriter, r *http.Request) {
	if !s.donationsReady(w) {
		return
	}
	var in struct {
		UserID *int64 `json:"user_id"`
	}
	if !issueDecode(w, r, &in) {
		return
	}
	if in.UserID != nil {
		var one int
		err := s.accountsDB().QueryRowContext(r.Context(), `SELECT 1 FROM users WHERE id = ?`, *in.UserID).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			writeAPIError(w, http.StatusNotFound, "no_such_user", "no such account")
			return
		} else if err != nil {
			pbServerError(w, err)
			return
		}
	}
	id := chi.URLParam(r, "id")
	if err := s.donations.Link(r.Context(), id, in.UserID); err != nil {
		s.donationErr(w, err)
		return
	}
	actor, _ := s.actorOf(r)
	detail := "by=" + actor + " id=" + id + " user="
	if in.UserID != nil {
		detail += strconv.FormatInt(*in.UserID, 10)
	} else {
		detail += "none"
	}
	s.opsAudit(r.Context(), 0, "donation:link", detail, "ok", 0)
	writeDataNoPeriod(w, map[string]any{"id": id, "user_id": in.UserID})
}

// POST /donations/{id}/visibility {visibility, display_name} (moderation: anonymous hides the name).
func (s *Service) handleDonationVisibility(w http.ResponseWriter, r *http.Request) {
	if !s.donationsReady(w) {
		return
	}
	var in struct {
		Visibility  string `json:"visibility"`
		DisplayName string `json:"display_name"`
	}
	if !issueDecode(w, r, &in) {
		return
	}
	name := ""
	switch in.Visibility {
	case donations.VisAnonymous:
	case donations.VisNamed:
		var err error
		if name, err = donations.SanitizeName(in.DisplayName); err != nil {
			writeAPIError(w, http.StatusBadRequest, "bad_name", err.Error())
			return
		}
	default:
		writeAPIError(w, http.StatusBadRequest, "bad_visibility", "visibility must be anonymous or named")
		return
	}
	id := chi.URLParam(r, "id")
	if err := s.donations.SetVisibility(r.Context(), id, in.Visibility, name); err != nil {
		s.donationErr(w, err)
		return
	}
	actor, _ := s.actorOf(r)
	s.opsAudit(r.Context(), 0, "donation:visibility", "by="+actor+" id="+id+" to="+in.Visibility, "ok", 0)
	writeDataNoPeriod(w, map[string]any{"id": id, "visibility": in.Visibility, "display_name": name})
}

func (s *Service) donationErr(w http.ResponseWriter, err error) {
	if errors.Is(err, donations.ErrNotFound) {
		writeAPIError(w, http.StatusNotFound, "not_found", "no such donation")
		return
	}
	pbServerError(w, err)
}
