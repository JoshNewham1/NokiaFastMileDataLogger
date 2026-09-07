package nokialogger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// var so tests can point it at a fake server
var MailjetSendURL = "https://api.mailjet.com/v3.1/send"

func MailjetConfigured(cfg *Config) bool {
	return cfg.MailjetAPIKey != "" && cfg.MailjetAPISecret != "" && cfg.MailjetFromEmail != "" && cfg.MailjetToEmail != ""
}

// SendFailureEmail sends a one-off failure notification through Mailjet's
// HTTP API. It returns an error if credentials aren't configured, if the
// request can't be sent, or if Mailjet returns a non-2xx status.
func SendFailureEmail(cfg *Config, message string) error {
	if !MailjetConfigured(cfg) {
		return fmt.Errorf("Mailjet credentials not configured")
	}

	payload := map[string]interface{}{
		"Messages": []map[string]interface{}{
			{
				"From":     map[string]string{"Email": cfg.MailjetFromEmail},
				"To":       []map[string]string{{"Email": cfg.MailjetToEmail}},
				"Subject":  "nokia_logger failed",
				"TextPart": fmt.Sprintf("nokia_logger failed at %s:\n\n%s", time.Now().Format(time.RFC3339), message),
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, MailjetSendURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.SetBasicAuth(cfg.MailjetAPIKey, cfg.MailjetAPISecret)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("mailjet returned status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}
