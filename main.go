// Command nokia_logger logs into a Nokia FastMile 5G router, reads the
// Cellular Packets Upload/Download totals and device uptime, and appends
// one row to a CSV file. It runs once per invocation and is meant to be
// triggered by Windows Task Scheduler at boot.
package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type config struct {
	Username         string
	Password         string
	BaseURL          string
	OutPath          string
	MailjetAPIKey    string
	MailjetAPISecret string
	MailjetFromEmail string
	MailjetToEmail   string
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		fail(cfg, fmt.Sprintf("loading config: %v", err))
	}

	upload, download, uptime, err := fetchStats(cfg)
	if err != nil {
		fail(cfg, fmt.Sprintf("fetching stats: %v", err))
	}

	if err := appendCSV(cfg.OutPath, time.Now().Format(time.RFC3339), upload, download, uptime); err != nil {
		fail(cfg, fmt.Sprintf("writing CSV: %v", err))
	}
}

// fail attempts to notify the user by email, falls back to stderr if that
// fails or isn't configured, and exits non-zero. There is no retry.
func fail(cfg *config, message string) {
	if cfg != nil {
		if err := sendFailureEmail(cfg, message); err != nil {
			fmt.Fprintf(os.Stderr, "nokia_logger: %s\n(failure email also failed: %v)\n", message, err)
			os.Exit(1)
		}
	}
	fmt.Fprintf(os.Stderr, "nokia_logger: %s\n", message)
	os.Exit(1)
}

// --- config loading ---

func loadConfig() (*config, error) {
	envValues := map[string]string{}
	for _, dir := range envSearchDirs() {
		path := filepath.Join(dir, ".env")
		if values, err := parseEnvFile(path); err == nil {
			envValues = values
			break
		}
	}

	// .env wins over any OS environment variables
	get := func(key string) string {
		if v, ok := envValues[key]; ok && v != "" {
			return v
		}
		return os.Getenv(key)
	}

	cfg := &config{
		Username:         get("USERNAME"),
		Password:         get("PASSWORD"),
		OutPath:          get("OUT_DIR"),
		MailjetAPIKey:    get("MAILJET_API_KEY"),
		MailjetAPISecret: get("MAILJET_API_SECRET"),
		MailjetFromEmail: get("MAILJET_FROM_EMAIL"),
		MailjetToEmail:   get("MAILJET_TO_EMAIL"),
	}

	statsURL := get("STATISTICS_URL")
	if cfg.Username == "" || cfg.Password == "" || statsURL == "" {
		return cfg, fmt.Errorf("USERNAME, PASSWORD, and STATISTICS_URL are required")
	}

	parsed, err := url.Parse(statsURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return cfg, fmt.Errorf("STATISTICS_URL %q is not a valid URL", statsURL)
	}
	cfg.BaseURL = parsed.Scheme + "://" + parsed.Host

	if cfg.OutPath == "" {
		dir, err := executableDir()
		if err != nil {
			dir = "."
		}
		cfg.OutPath = filepath.Join(dir, "data.csv")
	}

	return cfg, nil
}

func envSearchDirs() []string {
	dirs := []string{}
	if dir, err := executableDir(); err == nil {
		dirs = append(dirs, dir)
	}
	if cwd, err := os.Getwd(); err == nil {
		dirs = append(dirs, cwd)
	}
	return dirs
}

func executableDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}

func parseEnvFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		values[key] = value
	}
	return values, nil
}

// --- router login and stats ---

type nonceResponse struct {
	Nonce     string `json:"nonce"`
	RandomKey string `json:"randomKey"`
}

type loginResponse struct {
	Result       int    `json:"result"`
	ErrorMsg     string `json:"error_msg"`
	FailureCount int    `json:"failurecount"`
	Tip          string `json:"tip"`
}

type statsResponse struct {
	CellularStats []struct {
		BytesSent     json.Number `json:"BytesSent"`
		BytesReceived json.Number `json:"BytesReceived"`
	} `json:"cellular_stats"`
}

type deviceStatusResponse struct {
	UpTime json.Number `json:"UpTime"`
}

func fetchStats(cfg *config) (upload, download, uptime json.Number, err error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return "", "", "", err
	}
	// The router's HTTP server is HTTP/1.0 and closes the connection after
	// every response without signaling it cleanly
	transport := &http.Transport{DisableKeepAlives: true}
	client := &http.Client{Jar: jar, Timeout: 15 * time.Second, Transport: transport}

	nonce, err := getNonce(client, cfg.BaseURL)
	if err != nil {
		return "", "", "", fmt.Errorf("getting nonce: %w", err)
	}

	if err := login(client, cfg.BaseURL, cfg.Username, cfg.Password, nonce); err != nil {
		return "", "", "", fmt.Errorf("logging in: %w", err)
	}

	var stats statsResponse
	if err := getJSON(client, cfg.BaseURL+"/fastmile_radio_status_web_app.cgi", &stats); err != nil {
		return "", "", "", fmt.Errorf("getting radio stats: %w", err)
	}
	if len(stats.CellularStats) == 0 {
		return "", "", "", fmt.Errorf("radio stats response had no cellular_stats entries")
	}

	var device deviceStatusResponse
	if err := getJSON(client, cfg.BaseURL+"/device_status_web_app.cgi", &device); err != nil {
		return "", "", "", fmt.Errorf("getting device status: %w", err)
	}

	return stats.CellularStats[0].BytesSent, stats.CellularStats[0].BytesReceived, device.UpTime, nil
}

func getNonce(client *http.Client, baseURL string) (*nonceResponse, error) {
	var n nonceResponse
	if err := getJSON(client, baseURL+"/login_web_app.cgi?nonce", &n); err != nil {
		return nil, err
	}
	if n.Nonce == "" || n.RandomKey == "" {
		return nil, fmt.Errorf("response missing nonce or randomKey")
	}
	return &n, nil
}

func login(client *http.Client, baseURL, username, password string, nonce *nonceResponse) error {
	r := sha256Base64(username + ":" + password)
	if os.Getenv("NOKIA_LOGGER_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "debug: username_len=%d password_len=%d r=%s\n", len(username), len(password), r)
	}
	response := base64URLEscape(sha256Base64(r + ":" + nonce.Nonce))
	userhash := base64URLEscape(sha256Base64(username + ":" + nonce.Nonce))
	randomKeyHash := base64URLEscape(sha256Base64(nonce.RandomKey + ":" + nonce.Nonce))
	nonceParam := base64URLEscape(nonce.Nonce)

	enckey, err := randomBase64(16)
	if err != nil {
		return err
	}
	enciv, err := randomBase64(16)
	if err != nil {
		return err
	}

	body := fmt.Sprintf(
		"userhash=%s&RandomKeyhash=%s&response=%s&nonce=%s&enckey=%s&enciv=%s",
		userhash, randomKeyHash, response, nonceParam,
		base64URLEscape(enckey), base64URLEscape(enciv),
	)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/login_web_app.cgi", strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var lr loginResponse
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		return fmt.Errorf("decoding login response: %w", err)
	}
	if lr.Result != 0 {
		msg := lr.ErrorMsg
		if msg == "" {
			msg = fmt.Sprintf("result=%d failurecount=%d tip=%s", lr.Result, lr.FailureCount, lr.Tip)
		}
		return fmt.Errorf("router rejected login: %s", msg)
	}
	return nil
}

func getJSON(client *http.Client, url string, out interface{}) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d from %s", resp.StatusCode, url)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// sha256Base64 matches the router's own login code: sha256(a+":"+b), base64 encoded.
func sha256Base64(s string) string {
	sum := sha256.Sum256([]byte(s))
	return base64.StdEncoding.EncodeToString(sum[:])
}

// base64URLEscape matches the router's base64url_escape: swap +/= for -_. .
func base64URLEscape(s string) string {
	s = strings.ReplaceAll(s, "+", "-")
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "=", ".")
	return s
}

func randomBase64(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}

// --- CSV output ---

func appendCSV(path, timestamp string, upload, download, uptime json.Number) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	writeHeader := false
	if info, err := os.Stat(path); err != nil || info.Size() == 0 {
		writeHeader = true
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	if writeHeader {
		if err := w.Write([]string{
			"timestamp",
			"upload_bytes", "upload_mib", "upload_gib",
			"download_bytes", "download_mib", "download_gib",
			"uptime_seconds",
		}); err != nil {
			return err
		}
	}

	uploadMB, err := convertBytes(upload, mebibyte)
	if err != nil {
		return fmt.Errorf("converting upload bytes to MB: %w", err)
	}
	uploadGiB, err := convertBytes(upload, gibibyte)
	if err != nil {
		return fmt.Errorf("converting upload bytes to GiB: %w", err)
	}
	downloadMB, err := convertBytes(download, mebibyte)
	if err != nil {
		return fmt.Errorf("converting download bytes to MB: %w", err)
	}
	downloadGiB, err := convertBytes(download, gibibyte)
	if err != nil {
		return fmt.Errorf("converting download bytes to GiB: %w", err)
	}

	return w.Write([]string{
		timestamp,
		upload.String(), uploadMB, uploadGiB,
		download.String(), downloadMB, downloadGiB,
		uptime.String(),
	})
}

const (
	mebibyte = 1_048_576
	gibibyte = 1_073_741_824
)

// convertBytes converts a raw byte count (as reported by the router) to the
// given unit size, formatted to two decimal places.
func convertBytes(bytes json.Number, unitSize float64) (string, error) {
	value, err := bytes.Float64()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%.2f", value/unitSize), nil
}

// --- failure notification ---

func sendFailureEmail(cfg *config, message string) error {
	if cfg.MailjetAPIKey == "" || cfg.MailjetAPISecret == "" || cfg.MailjetFromEmail == "" || cfg.MailjetToEmail == "" {
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

	req, err := http.NewRequest(http.MethodPost, "https://api.mailjet.com/v3.1/send", bytes.NewReader(body))
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
