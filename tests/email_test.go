package nokialogger_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	nokialogger "nokia_logger/src"
)

func fullMailjetConfig() *nokialogger.Config {
	return &nokialogger.Config{
		MailjetAPIKey:    "key",
		MailjetAPISecret: "secret",
		MailjetFromEmail: "from@example.com",
		MailjetToEmail:   "to@example.com",
	}
}

func withMailjetURL(t *testing.T, url string) {
	t.Helper()
	orig := nokialogger.MailjetSendURL
	nokialogger.MailjetSendURL = url
	t.Cleanup(func() { nokialogger.MailjetSendURL = orig })
}

func TestSendFailureEmailMissingCredentials(t *testing.T) {
	if err := nokialogger.SendFailureEmail(&nokialogger.Config{}, "boom"); err == nil {
		t.Fatal("expected an error when Mailjet credentials are unset")
	}
}

func TestMailjetConfigured(t *testing.T) {
	if nokialogger.MailjetConfigured(&nokialogger.Config{}) {
		t.Error("empty config should not be considered configured")
	}
	if !nokialogger.MailjetConfigured(fullMailjetConfig()) {
		t.Error("fully populated config should be considered configured")
	}

	partial := fullMailjetConfig()
	partial.MailjetToEmail = ""
	if nokialogger.MailjetConfigured(partial) {
		t.Error("config missing MailjetToEmail should not be considered configured")
	}
}

func TestSendFailureEmailSuccess(t *testing.T) {
	var gotAuthUser, gotAuthPass string
	var gotBody map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthUser, gotAuthPass, _ = r.BasicAuth()
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	withMailjetURL(t, srv.URL)

	if err := nokialogger.SendFailureEmail(fullMailjetConfig(), "something broke"); err != nil {
		t.Fatalf("SendFailureEmail: %v", err)
	}
	if gotAuthUser != "key" || gotAuthPass != "secret" {
		t.Errorf("got basic auth %q/%q, want key/secret", gotAuthUser, gotAuthPass)
	}
	messages, ok := gotBody["Messages"].([]interface{})
	if !ok || len(messages) != 1 {
		t.Fatalf("got body %+v, want one message", gotBody)
	}
}

func TestSendFailureEmailNonSuccessStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad request body"))
	}))
	t.Cleanup(srv.Close)
	withMailjetURL(t, srv.URL)

	err := nokialogger.SendFailureEmail(fullMailjetConfig(), "something broke")
	if err == nil || !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "bad request body") {
		t.Fatalf("got err=%v, want it to mention the status code and body", err)
	}
}

func TestSendFailureEmailUnreachable(t *testing.T) {
	withMailjetURL(t, "http://127.0.0.1:1/send")

	if err := nokialogger.SendFailureEmail(fullMailjetConfig(), "something broke"); err == nil {
		t.Fatal("expected a network error hitting an unroutable address")
	}
}
