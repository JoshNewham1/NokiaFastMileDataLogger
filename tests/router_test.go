package nokialogger_test

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	nokialogger "nokia_logger/src"
)

func TestSha256Base64(t *testing.T) {
	input := "admin:hunter2"
	sum := sha256.Sum256([]byte(input))
	want := base64.StdEncoding.EncodeToString(sum[:])

	if got := nokialogger.Sha256Base64(input); got != want {
		t.Errorf("Sha256Base64(%q) = %q, want %q", input, got, want)
	}

	if nokialogger.Sha256Base64("a") == nokialogger.Sha256Base64("b") {
		t.Error("different inputs produced the same hash")
	}
}

func TestBase64URLEscape(t *testing.T) {
	cases := []struct{ in, want string }{
		{"abc", "abc"},
		{"a+b/c=", "a-b_c."},
		{"++//==", "--__.."},
	}
	for _, c := range cases {
		if got := nokialogger.Base64URLEscape(c.in); got != c.want {
			t.Errorf("Base64URLEscape(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRandomBase64(t *testing.T) {
	s, err := nokialogger.RandomBase64(16)
	if err != nil {
		t.Fatalf("RandomBase64: %v", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		t.Fatalf("RandomBase64 did not return valid base64: %v", err)
	}
	if len(decoded) != 16 {
		t.Errorf("decoded length = %d, want 16", len(decoded))
	}

	s2, err := nokialogger.RandomBase64(16)
	if err != nil {
		t.Fatalf("RandomBase64: %v", err)
	}
	if s == s2 {
		t.Error("two calls to RandomBase64 produced the same value")
	}
}

// fakeRouter builds an httptest.Server that plays the part of the Nokia
// router across all five endpoints this program calls, so the HTTP-facing
// functions can be tested without a real device.
type fakeRouter struct {
	nonce         string
	randomKey     string
	loginResult   int
	loginErrorMsg string
	radioStatus   int
	radioBody     string
	deviceStatus  int
	deviceBody    string
	statsStatus   int
	statsBody     string
}

func (f *fakeRouter) start(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/login_web_app.cgi", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			fmt.Fprintf(w, `{"nonce":%q,"randomKey":%q,"pubkey":"unused"}`, f.nonce, f.randomKey)
			return
		}
		if f.loginErrorMsg != "" {
			fmt.Fprintf(w, `{"result":%d,"error_msg":%q}`, f.loginResult, f.loginErrorMsg)
			return
		}
		fmt.Fprintf(w, `{"result":%d,"failurecount":1,"tip":""}`, f.loginResult)
	})
	mux.HandleFunc("/fastmile_radio_status_web_app.cgi", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(f.radioStatus)
		fmt.Fprint(w, f.radioBody)
	})
	mux.HandleFunc("/device_status_web_app.cgi", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(f.deviceStatus)
		fmt.Fprint(w, f.deviceBody)
	})
	mux.HandleFunc("/fastmile_statistics_status_web_app.cgi", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(f.statsStatus)
		fmt.Fprint(w, f.statsBody)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func defaultFakeRouter() *fakeRouter {
	return &fakeRouter{
		nonce:        "dGVzdC1ub25jZQ==",
		randomKey:    "123",
		loginResult:  0,
		radioStatus:  http.StatusOK,
		radioBody:    `{"cellular_stats":[{"BytesSent":1000000,"BytesReceived":2000000}]}`,
		deviceStatus: http.StatusOK,
		deviceBody:   `{"UpTime":12345}`,
		statsStatus:  http.StatusOK,
		statsBody:    `{"stats_cfg":[{"BytesSent":48145,"BytesReceived":2162}]}`,
	}
}

func TestFetchStatsSuccess(t *testing.T) {
	srv := defaultFakeRouter().start(t)
	cfg := &nokialogger.Config{BaseURL: srv.URL, Username: "admin", Password: "pass"}

	stats, err := nokialogger.FetchStats(cfg)
	if err != nil {
		t.Fatalf("FetchStats: %v", err)
	}
	if stats.RadioUpload.String() != "1000000" || stats.RadioDownload.String() != "2000000" {
		t.Errorf("got radio upload=%s download=%s", stats.RadioUpload, stats.RadioDownload)
	}
	if stats.StatsUpload.String() != "48145" || stats.StatsDownload.String() != "2162" {
		t.Errorf("got stats upload=%s download=%s", stats.StatsUpload, stats.StatsDownload)
	}
	if stats.Uptime.String() != "12345" {
		t.Errorf("got uptime=%s", stats.Uptime)
	}
}

func TestFetchStatsLoginRejected(t *testing.T) {
	fr := defaultFakeRouter()
	fr.loginResult = -1
	fr.loginErrorMsg = "bad credentials"
	srv := fr.start(t)
	cfg := &nokialogger.Config{BaseURL: srv.URL, Username: "admin", Password: "wrong"}

	_, err := nokialogger.FetchStats(cfg)
	if err == nil || !strings.Contains(err.Error(), "bad credentials") {
		t.Fatalf("got err=%v, want it to mention the router's error_msg", err)
	}
}

func TestFetchStatsLoginRejectedWithoutErrorMsg(t *testing.T) {
	fr := defaultFakeRouter()
	fr.loginResult = -1
	srv := fr.start(t)
	cfg := &nokialogger.Config{BaseURL: srv.URL, Username: "admin", Password: "wrong"}

	_, err := nokialogger.FetchStats(cfg)
	if err == nil || !strings.Contains(err.Error(), "failurecount") {
		t.Fatalf("got err=%v, want the composed result/failurecount/tip message", err)
	}
}

func TestFetchStatsEmptyCellularStats(t *testing.T) {
	fr := defaultFakeRouter()
	fr.radioBody = `{"cellular_stats":[]}`
	srv := fr.start(t)
	cfg := &nokialogger.Config{BaseURL: srv.URL, Username: "admin", Password: "pass"}

	_, err := nokialogger.FetchStats(cfg)
	if err == nil || !strings.Contains(err.Error(), "no cellular_stats") {
		t.Fatalf("got err=%v, want a no-cellular_stats error", err)
	}
}

func TestFetchStatsRadioEndpointError(t *testing.T) {
	fr := defaultFakeRouter()
	fr.radioStatus = http.StatusInternalServerError
	srv := fr.start(t)
	cfg := &nokialogger.Config{BaseURL: srv.URL, Username: "admin", Password: "pass"}

	_, err := nokialogger.FetchStats(cfg)
	if err == nil || !strings.Contains(err.Error(), "getting radio stats") {
		t.Fatalf("got err=%v, want a wrapped radio-stats error", err)
	}
}

func TestFetchStatsDeviceEndpointError(t *testing.T) {
	fr := defaultFakeRouter()
	fr.deviceStatus = http.StatusInternalServerError
	srv := fr.start(t)
	cfg := &nokialogger.Config{BaseURL: srv.URL, Username: "admin", Password: "pass"}

	_, err := nokialogger.FetchStats(cfg)
	if err == nil || !strings.Contains(err.Error(), "getting device status") {
		t.Fatalf("got err=%v, want a wrapped device-status error", err)
	}
}

func TestFetchStatsEmptyStatsCfg(t *testing.T) {
	fr := defaultFakeRouter()
	fr.statsBody = `{"stats_cfg":[]}`
	srv := fr.start(t)
	cfg := &nokialogger.Config{BaseURL: srv.URL, Username: "admin", Password: "pass"}

	_, err := nokialogger.FetchStats(cfg)
	if err == nil || !strings.Contains(err.Error(), "no stats_cfg") {
		t.Fatalf("got err=%v, want a no-stats_cfg error", err)
	}
}

func TestFetchStatsFastmileStatisticsEndpointError(t *testing.T) {
	fr := defaultFakeRouter()
	fr.statsStatus = http.StatusInternalServerError
	srv := fr.start(t)
	cfg := &nokialogger.Config{BaseURL: srv.URL, Username: "admin", Password: "pass"}

	_, err := nokialogger.FetchStats(cfg)
	if err == nil || !strings.Contains(err.Error(), "getting fastmile statistics") {
		t.Fatalf("got err=%v, want a wrapped fastmile-statistics error", err)
	}
}

func TestGetNonceMissingFields(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/login_web_app.cgi", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"nonce":"","randomKey":""}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	if _, err := nokialogger.GetNonce(http.DefaultClient, srv.URL); err == nil {
		t.Fatal("expected an error when nonce/randomKey are missing")
	}
}

func TestGetJSONUnreachable(t *testing.T) {
	// Port 0 on loopback with no listener: connection refused, fast and
	// dependency-free way to exercise the client.Get error branch.
	var out struct{}
	if err := nokialogger.GetJSON(http.DefaultClient, "http://127.0.0.1:1/nope", &out); err == nil {
		t.Fatal("expected a network error hitting an unroutable address")
	}
}

func TestLoginUnreachable(t *testing.T) {
	nonce := &nokialogger.NonceResponse{Nonce: "n", RandomKey: "1"}
	if err := nokialogger.Login(http.DefaultClient, "http://127.0.0.1:1", "u", "p", nonce); err == nil {
		t.Fatal("expected a network error hitting an unroutable address")
	}
}

func TestLoginMalformedResponse(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/login_web_app.cgi", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `not json`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	nonce := &nokialogger.NonceResponse{Nonce: "n", RandomKey: "1"}
	if err := nokialogger.Login(http.DefaultClient, srv.URL, "u", "p", nonce); err == nil {
		t.Fatal("expected a decode error for a non-JSON login response")
	}
}
