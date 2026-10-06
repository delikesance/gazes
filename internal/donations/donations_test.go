package donations

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestService(t *testing.T, cfg Config) *Service {
	t.Helper()
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return NewService(st, cfg)
}

func TestParseCents(t *testing.T) {
	for in, want := range map[string]int64{"3": 300, "3.5": 350, "3.00": 300, "0.07": 7, "12.34": 1234, "5.00000000": 500} {
		got, err := ParseCents(in)
		if err != nil || got != want {
			t.Errorf("ParseCents(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"", "abc", "-3", "1.234", "1,5", "1.2.3"} {
		if _, err := ParseCents(in); err == nil {
			t.Errorf("ParseCents(%q) accepted", in)
		}
	}
}

func TestSanitizeName(t *testing.T) {
	ok := map[string]string{"  Léa  ": "Léa", "Jean  Pierre": "Jean Pierre", "o'neil": "o'neil", "夜月": "夜月"}
	for in, want := range ok {
		if got, err := SanitizeName(in); err != nil || got != want {
			t.Errorf("SanitizeName(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "a", strings.Repeat("a", 25), "<b>x</b>", "visit http://x", "www.foo", "@bob", "a\nb\x00", "x.y"} {
		if _, err := SanitizeName(in); err == nil {
			t.Errorf("SanitizeName(%q) accepted", in)
		}
	}
}

func TestVerifyBTCPaySignature(t *testing.T) {
	body := []byte(`{"type":"InvoiceSettled"}`)
	mac := hmac.New(sha256.New, []byte("s3cret"))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !VerifyBTCPaySignature(body, sig, "s3cret") {
		t.Fatal("valid signature refused")
	}
	for name, c := range map[string]struct {
		body        []byte
		sig, secret string
	}{
		"wrong secret": {body, sig, "other"},
		"tampered":     {[]byte(`{"type":"InvoiceSettled "}`), sig, "s3cret"},
		"no prefix":    {body, strings.TrimPrefix(sig, "sha256="), "s3cret"},
		"not hex":      {body, "sha256=zz", "s3cret"},
		"empty secret": {body, sig, ""},
		"empty header": {body, "", "s3cret"},
	} {
		if VerifyBTCPaySignature(c.body, c.sig, c.secret) {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestStoreLifecycle(t *testing.T) {
	s := newTestService(t, Config{}).Store
	ctx := context.Background()
	now := time.Now()
	d, created, err := s.Insert(ctx, Donation{Provider: ProviderBTCPay, ProviderRef: "inv1", Visibility: VisNamed, DisplayName: "Léa",
		AmountCents: 500, Currency: CurrencyEUR, Status: StatusPending, CreatedAt: now.Unix()})
	if err != nil || !created {
		t.Fatalf("insert: %v %v", created, err)
	}
	if names, _ := s.PublicNames(ctx, 10); len(names) != 0 {
		t.Fatalf("pending donation is public: %v", names)
	}
	if ok, err := s.Settle(ctx, ProviderBTCPay, "inv1", 700, CurrencyEUR, now); err != nil || !ok {
		t.Fatalf("settle: %v %v", ok, err)
	}
	if ok, _ := s.Settle(ctx, ProviderBTCPay, "inv1", 999, CurrencyEUR, now); ok {
		t.Fatal("replayed settle changed the row")
	}
	sum, _ := s.Summary(ctx, now.Add(-time.Hour).Unix())
	if sum.AllCents != 700 || sum.MonthCents != 700 || sum.Count != 1 || sum.Donors != 1 {
		t.Fatalf("summary %+v", sum)
	}
	if names, _ := s.PublicNames(ctx, 10); len(names) != 1 || names[0] != "Léa" {
		t.Fatalf("public names %v", names)
	}
	if err := s.SetVisibility(ctx, d.ID, VisAnonymous, "Léa"); err != nil {
		t.Fatal(err)
	}
	if names, _ := s.PublicNames(ctx, 10); len(names) != 0 {
		t.Fatalf("masked donor still public: %v", names)
	}
	uid := int64(7)
	if err := s.Link(ctx, d.ID, &uid); err != nil {
		t.Fatal(err)
	}
	list, _ := s.List(ctx, "", 10, 0)
	if len(list) != 1 || list[0].UserID == nil || *list[0].UserID != 7 || list[0].DisplayName != "" {
		t.Fatalf("list %+v", list)
	}
	if err := s.Link(ctx, "nope", nil); err != ErrNotFound {
		t.Fatalf("link unknown: %v", err)
	}
}

func TestKofiDonation(t *testing.T) {
	svc := newTestService(t, Config{KofiToken: "tok"})
	svc.now = func() time.Time { return time.Unix(1000, 0) }
	mk := func(m map[string]any) string { b, _ := json.Marshal(m); return string(b) }
	base := map[string]any{"verification_token": "tok", "type": "Donation", "is_public": true, "from_name": "Léa", "message": "gg",
		"amount": "3.00", "currency": "EUR", "kofi_transaction_id": "tx1", "email": "secret@example.com"}

	d, err := svc.KofiDonation(mk(base))
	if err != nil || d.AmountCents != 300 || d.Visibility != VisNamed || d.DisplayName != "Léa" || d.DonorLabel != "Léa" || d.Status != StatusSettled || d.ProviderRef != "tx1" {
		t.Fatalf("donation %+v err %v", d, err)
	}
	if strings.Contains(mk(map[string]any{}), "secret") || strings.Contains(d.DonorLabel+d.Message+d.DisplayName, "secret@") {
		t.Fatal("e-mail leaked")
	}

	base["is_public"] = false
	if d, _ := svc.KofiDonation(mk(base)); d.Visibility != VisAnonymous || d.DisplayName != "" || d.DonorLabel != "Léa" {
		t.Fatalf("private donation %+v", d)
	}
	base["is_public"], base["from_name"] = true, "http://spam.example"
	if d, _ := svc.KofiDonation(mk(base)); d.Visibility != VisAnonymous {
		t.Fatalf("unsafe public name accepted: %+v", d)
	}
	base["verification_token"] = "bad"
	if _, err := svc.KofiDonation(mk(base)); err == nil {
		t.Fatal("bad token accepted")
	}
	base["verification_token"], base["type"] = "tok", "Shop Order"
	if _, err := svc.KofiDonation(mk(base)); err != ErrKofiIgnored {
		t.Fatalf("shop order: %v", err)
	}
	if _, err := newTestService(t, Config{}).KofiDonation(mk(map[string]any{"verification_token": "", "type": "Donation", "amount": "1", "kofi_transaction_id": "x"})); err == nil {
		t.Fatal("empty configured token accepted an empty payload token")
	}
}

func TestBTCPayClient(t *testing.T) {
	var gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		switch {
		case r.Method == "POST" && r.URL.Path == "/api/v1/stores/st1/invoices":
			b := new(strings.Builder)
			buf := make([]byte, 1024)
			n, _ := r.Body.Read(buf)
			b.Write(buf[:n])
			gotBody = b.String()
			w.Write([]byte(`{"id":"inv9","checkoutLink":"https://pay.example/i/inv9"}`))
		case r.Method == "GET" && r.URL.Path == "/api/v1/stores/st1/invoices/inv9":
			w.Write([]byte(`{"status":"Settled","amount":"5.00000000","currency":"EUR"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	svc := newTestService(t, Config{BTCPayURL: srv.URL + "/", BTCPayStoreID: "st1", BTCPayAPIKey: "key", BTCPayWebhookSecret: "w", SiteURL: "https://gazes.example/"})
	id, link, err := svc.CreateInvoice(context.Background(), "don1", 500)
	if err != nil || id != "inv9" || link != "https://pay.example/i/inv9" {
		t.Fatalf("create: %q %q %v", id, link, err)
	}
	if gotAuth != "token key" || !strings.Contains(gotBody, `"amount":"5.00"`) || !strings.Contains(gotBody, `"orderId":"don1"`) || !strings.Contains(gotBody, "https://gazes.example/soutenir/merci") {
		t.Fatalf("request auth %q body %s", gotAuth, gotBody)
	}
	inv, err := svc.GetInvoice(context.Background(), "inv9")
	if err != nil || inv.Status != "Settled" || inv.AmountCents != 500 || inv.Currency != "EUR" {
		t.Fatalf("invoice %+v %v", inv, err)
	}
	if _, err := svc.GetInvoice(context.Background(), "missing"); err == nil {
		t.Fatal("404 not reported")
	}
}
