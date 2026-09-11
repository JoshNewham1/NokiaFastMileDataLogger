package nokialogger

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strings"
	"time"
)

// NonceResponse is the router's response to GET /login_web_app.cgi?nonce.
type NonceResponse struct {
	Nonce     string `json:"nonce"`
	RandomKey string `json:"randomKey"`
}

// LoginResponse is the router's response to POST /login_web_app.cgi.
type LoginResponse struct {
	Result       int    `json:"result"`
	ErrorMsg     string `json:"error_msg"`
	FailureCount int    `json:"failurecount"`
	Tip          string `json:"tip"`
}

// Router's response to GET /fastmile_radio_status_web_app.cgi.
type FastmileRadioResponse struct {
	CellularStats []struct {
		BytesSent     json.Number `json:"BytesSent"`
		BytesReceived json.Number `json:"BytesReceived"`
	} `json:"cellular_stats"`
}

// Router's response to GET /fastmile_statistics_status_web_app.cgi
type FastmileStatsResponse struct {
	StatsCfg []struct {
		// Actually in MB
		MegabytesSent     json.Number `json:"BytesSent"`
		MegabytesReceived json.Number `json:"BytesReceived"`
	} `json:"stats_cfg"`
}

// DeviceStatusResponse is the router's response to GET /device_status_web_app.cgi.
type DeviceStatusResponse struct {
	UpTime json.Number `json:"UpTime"`
}

// Stats from all endpoints combined
type RouterStats struct {
	RadioUpload   json.Number
	RadioDownload json.Number
	StatsUpload   json.Number
	StatsDownload json.Number
	Uptime        json.Number
}

func FetchStats(cfg *Config) (RouterStats, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return RouterStats{}, err
	}

	transport := &http.Transport{DisableKeepAlives: true}
	client := &http.Client{Jar: jar, Timeout: 15 * time.Second, Transport: transport}

	nonce, err := GetNonce(client, cfg.BaseURL)
	if err != nil {
		return RouterStats{}, fmt.Errorf("getting nonce: %w", err)
	}

	if err := Login(client, cfg.BaseURL, cfg.Username, cfg.Password, nonce); err != nil {
		return RouterStats{}, fmt.Errorf("logging in: %w", err)
	}

	var radio FastmileRadioResponse
	if err := GetJSON(client, cfg.BaseURL+"/fastmile_radio_status_web_app.cgi", &radio); err != nil {
		return RouterStats{}, fmt.Errorf("getting radio stats: %w", err)
	}
	if len(radio.CellularStats) == 0 {
		return RouterStats{}, fmt.Errorf("radio stats response had no cellular_stats entries")
	}

	var device DeviceStatusResponse
	if err := GetJSON(client, cfg.BaseURL+"/device_status_web_app.cgi", &device); err != nil {
		return RouterStats{}, fmt.Errorf("getting device status: %w", err)
	}

	var fastmileStats FastmileStatsResponse
	if err := GetJSON(client, cfg.BaseURL+"/fastmile_statistics_status_web_app.cgi", &fastmileStats); err != nil {
		return RouterStats{}, fmt.Errorf("getting fastmile statistics: %w", err)
	}
	if len(fastmileStats.StatsCfg) == 0 {
		return RouterStats{}, fmt.Errorf("fastmile statistics response had no stats_cfg entries")
	}

	return RouterStats{
		RadioUpload:   radio.CellularStats[0].BytesSent,
		RadioDownload: radio.CellularStats[0].BytesReceived,
		StatsUpload:   fastmileStats.StatsCfg[0].MegabytesSent,
		StatsDownload: fastmileStats.StatsCfg[0].MegabytesReceived,
		Uptime:        device.UpTime,
	}, nil
}

// GetNonce fetches the login nonce and random key.
func GetNonce(client *http.Client, baseURL string) (*NonceResponse, error) {
	var n NonceResponse
	if err := GetJSON(client, baseURL+"/login_web_app.cgi?nonce", &n); err != nil {
		return nil, err
	}
	if n.Nonce == "" || n.RandomKey == "" {
		return nil, fmt.Errorf("response missing nonce or randomKey")
	}
	return &n, nil
}

// Login derives the router's SHA256-based login hashes and posts them to
// /login_web_app.cgi, establishing a session cookie in client's jar.
func Login(client *http.Client, baseURL, username, password string, nonce *NonceResponse) error {
	r := Sha256Base64(username + ":" + password)
	if os.Getenv("NOKIA_LOGGER_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "debug: username_len=%d password_len=%d r=%s\n", len(username), len(password), r)
	}
	response := Base64URLEscape(Sha256Base64(r + ":" + nonce.Nonce))
	userhash := Base64URLEscape(Sha256Base64(username + ":" + nonce.Nonce))
	randomKeyHash := Base64URLEscape(Sha256Base64(nonce.RandomKey + ":" + nonce.Nonce))
	nonceParam := Base64URLEscape(nonce.Nonce)

	enckey, err := RandomBase64(16)
	if err != nil {
		return err
	}
	enciv, err := RandomBase64(16)
	if err != nil {
		return err
	}

	body := fmt.Sprintf(
		"userhash=%s&RandomKeyhash=%s&response=%s&nonce=%s&enckey=%s&enciv=%s",
		userhash, randomKeyHash, response, nonceParam,
		Base64URLEscape(enckey), Base64URLEscape(enciv),
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

	var lr LoginResponse
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

// GetJSON GETs url and decodes the JSON response body into out.
func GetJSON(client *http.Client, url string, out interface{}) error {
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

// Sha256Base64 matches the router's own login code: sha256(a+":"+b), base64 encoded.
func Sha256Base64(s string) string {
	sum := sha256.Sum256([]byte(s))
	return base64.StdEncoding.EncodeToString(sum[:])
}

// Base64URLEscape matches the router's base64url_escape: swap +/= for -_. .
func Base64URLEscape(s string) string {
	s = strings.ReplaceAll(s, "+", "-")
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "=", ".")
	return s
}

// RandomBase64 returns n random bytes, base64 encoded.
func RandomBase64(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}
