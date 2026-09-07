// Package nokialogger logs into a Nokia FastMile 5G router, reads the
// Cellular Packets Upload/Download totals and device uptime, and appends
// one row to a CSV file. It's meant to run once per invocation, triggered
// by Windows Task Scheduler at boot.
package nokialogger

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Config holds everything read from .env (or the real OS environment as a
// fallback) needed to log in to the router, write the CSV, and send a
// failure notification.
type Config struct {
	Username         string
	Password         string
	BaseURL          string
	OutPath          string
	MailjetAPIKey    string
	MailjetAPISecret string
	MailjetFromEmail string
	MailjetToEmail   string
}

// LoadConfig reads .env (searched next to the executable, then the current
// directory) and the real OS environment, and validates the required
// fields.
func LoadConfig() (*Config, error) {
	envValues := map[string]string{}
	for _, dir := range EnvSearchDirs() {
		path := filepath.Join(dir, ".env")
		if values, err := ParseEnvFile(path); err == nil {
			envValues = values
			break
		}
	}

	// .env wins over real OS environment variables. Windows predefines
	// USERNAME (and others) for every process to the logged-in Windows
	// account, which would otherwise silently shadow the router username.
	get := func(key string) string {
		if v, ok := envValues[key]; ok && v != "" {
			return v
		}
		return os.Getenv(key)
	}

	cfg := &Config{
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
		dir, err := ExecutableDir()
		if err != nil {
			dir = "."
		}
		cfg.OutPath = filepath.Join(dir, "data.csv")
	}

	return cfg, nil
}

// EnvSearchDirs returns, in priority order, the directories checked for a
// .env file: next to the running executable, then the current directory.
func EnvSearchDirs() []string {
	dirs := []string{}
	if dir, err := ExecutableDir(); err == nil {
		dirs = append(dirs, dir)
	}
	if cwd, err := os.Getwd(); err == nil {
		dirs = append(dirs, cwd)
	}
	return dirs
}

// ExecutableDir returns the directory containing the running executable.
func ExecutableDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}

// ParseEnvFile reads a simple KEY=VALUE file, ignoring blank lines and
// lines starting with #, and stripping surrounding quotes from values.
func ParseEnvFile(path string) (map[string]string, error) {
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
