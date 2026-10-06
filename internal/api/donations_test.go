package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gazes/gazes/internal/config"
	"github.com/gazes/gazes/internal/donations"
)

type donationEnv struct {
	h       http.Handler
	svc     *donations.Service
	invoice string // status the fake BTCPay reports
}

func newDonationEnv(t *testing.T, goal int64) *donationEnv {
	t.Helper()
	e := &donationEnv{invoice: "Settled"}
	btcpay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST":
			io.WriteString(w, `{"id":"inv1","checkoutLink":"https://pay.example/i/inv1"}`)
		case strings.HasSuffix(r.URL.Path, "/inv1"):
			io.WriteString(w, `{"status":"`+e.invoice+`","amount":"7.00","currency":"EUR"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(btcpay.Close)
	st, err := donations.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	e.svc = donations.NewService(st, donations.Config{SiteURL: "https://gazes.example", BTCPayURL: btcpay.URL, BTCPayStoreID: "st", BTCPayAPIKey: "k",
		BTCPayWebhookSecret: "whsec", KofiURL: "https://ko-fi.com/gazes", KofiToken: "kofitok", GoalCents: goal})
	s := NewServer(&config.Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, nil, nil, WithDonations(e.svc))
	e.h = s.Router()
	return e
}

func (e *donationEnv) do(method, path string, body io.Reader, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, body)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func sign(body, secret string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

func TestDonationFlowBTCPay(t *testing.T) {
	e := newDonationEnv(t, 5000)
	invoice := func(body string) *httptest.ResponseRecorder {
		return e.do("POST", "/api/v1/donations/btcpay/invoice", strings.NewReader(body), map[string]string{"Content-Type": "application/json"})
	}

	for _, bad := range []string{`{"amount_cents":50}`, `{"amount_cents":999999}`, `{"amount_cents":500,"visibility":"named","display_name":"<x>"}`, `{"amount_cents":500,"extra":1}`, `nope`} {
		if c := invoice(bad).Code; c != 400 {
			t.Errorf("%s: %d, want 400", bad, c)
		}
	}
	rec := invoice(`{"amount_cents":700,"visibility":"named","display_name":"Léa"}`)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "https://pay.example/i/inv1") {
		t.Fatalf("invoice: %d %s", rec.Code, rec.Body)
	}

	// Pending: nothing public yet.
	var pub publicDonations
	json.Unmarshal(e.do("GET", "/api/v1/donations", nil, nil).Body.Bytes(), &pub)
	if len(pub.Donors) != 0 || *pub.MonthCents != 0 || !pub.BTCPay || pub.KofiURL == "" {
		t.Fatalf("public before settle: %+v", pub)
	}

	hook := func(body, sig string) int {
		return e.do("POST", "/api/v1/donations/webhooks/btcpay", strings.NewReader(body), map[string]string{"BTCPay-Sig": sig}).Code
	}
	settled := `{"type":"InvoiceSettled","invoiceId":"inv1"}`
	if c := hook(settled, "sha256=00"); c != 401 {
		t.Fatalf("bad signature: %d", c)
	}
	e.invoice = "Processing" // webhook claims settled but BTCPay disagrees: nothing recorded
	if c := hook(settled, sign(settled, "whsec")); c != 200 {
		t.Fatalf("unsettled: %d", c)
	}
	if got, _ := e.svc.Store.List(context.Background(), "settled", 10, 0); len(got) != 0 {
		t.Fatal("recorded a donation BTCPay does not report as settled")
	}
	e.invoice = "Settled"
	for i := 0; i < 2; i++ { // redelivery is a no-op
		if c := hook(settled, sign(settled, "whsec")); c != 200 {
			t.Fatalf("settled: %d", c)
		}
	}
	json.Unmarshal(e.do("GET", "/api/v1/donations", nil, nil).Body.Bytes(), &pub)
	if len(pub.Donors) != 1 || pub.Donors[0] != "Léa" || *pub.MonthCents != 700 {
		t.Fatalf("public after settle: %+v", pub)
	}
	if strings.Contains(e.do("GET", "/api/v1/donations", nil, nil).Body.String(), "inv1") {
		t.Fatal("provider reference leaked publicly")
	}

	// An invoice we never created is ignored without error.
	other := `{"type":"InvoiceSettled","invoiceId":"unknown"}`
	if c := hook(other, sign(other, "whsec")); c != 502 && c != 200 {
		t.Fatalf("unknown invoice: %d", c)
	}
}

func TestDonationKofiWebhook(t *testing.T) {
	e := newDonationEnv(t, 0)
	send := func(payload string) int {
		form := url.Values{"data": {payload}}
		return e.do("POST", "/api/v1/donations/webhooks/kofi", strings.NewReader(form.Encode()), map[string]string{"Content-Type": "application/x-www-form-urlencoded"}).Code
	}
	good := `{"verification_token":"kofitok","type":"Donation","is_public":false,"from_name":"Bob","amount":"3.00","currency":"EUR","kofi_transaction_id":"t1","email":"bob@example.com"}`
	if c := send(strings.Replace(good, "kofitok", "wrong", 1)); c != 401 {
		t.Fatalf("wrong token: %d", c)
	}
	for i := 0; i < 2; i++ {
		if c := send(good); c != 200 {
			t.Fatalf("good: %d", c)
		}
	}
	list, _ := e.svc.Store.List(context.Background(), "", 10, 0)
	if len(list) != 1 || list[0].DonorLabel != "Bob" || list[0].Visibility != donations.VisAnonymous || list[0].AmountCents != 300 {
		t.Fatalf("stored %+v", list)
	}
	if strings.Contains(list[0].Message+list[0].DonorLabel, "@") {
		t.Fatal("email stored")
	}
	var pub publicDonations
	json.Unmarshal(e.do("GET", "/api/v1/donations", nil, nil).Body.Bytes(), &pub)
	if len(pub.Donors) != 0 || pub.MonthCents != nil {
		t.Fatalf("anonymous Ko-fi donor or total exposed without a goal: %+v", pub)
	}
}

func TestDonationsDisabledProviders(t *testing.T) {
	st, _ := donations.Open(t.TempDir())
	t.Cleanup(func() { st.Close() })
	s := NewServer(&config.Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, nil, nil, WithDonations(donations.NewService(st, donations.Config{})))
	for _, p := range []string{"/api/v1/donations/btcpay/invoice", "/api/v1/donations/webhooks/btcpay", "/api/v1/donations/webhooks/kofi"} {
		rec := httptest.NewRecorder()
		s.Router().ServeHTTP(rec, httptest.NewRequest("POST", p, strings.NewReader("{}")))
		if rec.Code != 503 {
			t.Errorf("%s: %d, want 503", p, rec.Code)
		}
	}
}
