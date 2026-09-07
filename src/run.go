package nokialogger

import (
	"fmt"
	"os"
	"time"
)

// Run performs one full cycle: load config, log in and fetch stats, append
// a CSV row. On any failure it attempts a failure email and exits non-zero.
// There is no retry; each run is a single attempt.
func Run() {
	cfg, err := LoadConfig()
	if err != nil {
		Fail(cfg, fmt.Sprintf("loading config: %v", err))
	}

	upload, download, uptime, err := FetchStats(cfg)
	if err != nil {
		Fail(cfg, fmt.Sprintf("fetching stats: %v", err))
	}

	if err := AppendCSV(cfg.OutPath, time.Now().Format(time.RFC3339), upload, download, uptime); err != nil {
		Fail(cfg, fmt.Sprintf("writing CSV: %v", err))
	}
}

// Fail attempts to notify the user by email, falls back to stderr if that
// fails or isn't configured, and exits non-zero. There is no retry.
func Fail(cfg *Config, message string) {
	if cfg != nil {
		if err := SendFailureEmail(cfg, message); err != nil {
			fmt.Fprintf(os.Stderr, "nokia_logger: %s\n(failure email also failed: %v)\n", message, err)
			os.Exit(1)
		}
	}
	fmt.Fprintf(os.Stderr, "nokia_logger: %s\n", message)
	os.Exit(1)
}
