package nokialogger_test

import (
	"os"
	"path/filepath"
	"testing"

	nokialogger "nokia_logger/src"
)

func TestParseEnvFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "USERNAME=admin\n" +
		"PASSWORD=\"pass with quotes\"\n" +
		"# a comment\n" +
		"\n" +
		"  SPACED = trimmed  \n" +
		"SINGLE='quoted'\n" +
		"NOVALUELINE\n" +
		"OUT_DIR=\"/tmp/data.csv\"\r\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	values, err := nokialogger.ParseEnvFile(path)
	if err != nil {
		t.Fatalf("ParseEnvFile: %v", err)
	}

	want := map[string]string{
		"USERNAME": "admin",
		"PASSWORD": "pass with quotes",
		"SPACED":   "trimmed",
		"SINGLE":   "quoted",
		"OUT_DIR":  "/tmp/data.csv",
	}
	for k, v := range want {
		if got := values[k]; got != v {
			t.Errorf("values[%q] = %q, want %q", k, got, v)
		}
	}
	if _, ok := values["NOVALUELINE"]; ok {
		t.Errorf("line with no '=' should be ignored, got entry for NOVALUELINE")
	}
}

func TestParseEnvFileMissing(t *testing.T) {
	if _, err := nokialogger.ParseEnvFile(filepath.Join(t.TempDir(), "does-not-exist.env")); err == nil {
		t.Fatal("expected an error reading a missing .env file, got nil")
	}
}

func TestEnvSearchDirs(t *testing.T) {
	dirs := nokialogger.EnvSearchDirs()
	if len(dirs) == 0 {
		t.Fatal("expected at least one search directory")
	}
}

func TestExecutableDir(t *testing.T) {
	dir, err := nokialogger.ExecutableDir()
	if err != nil {
		t.Fatalf("ExecutableDir: %v", err)
	}
	if dir == "" {
		t.Fatal("ExecutableDir returned an empty string")
	}
}

// withTempCwd chdirs into a fresh temp directory for the duration of the
// test, restoring the original working directory afterward.
func withTempCwd(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(orig)
	})
	return dir
}

func writeEnvFile(t *testing.T, dir string, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadConfigSuccess(t *testing.T) {
	dir := withTempCwd(t)
	writeEnvFile(t, dir, ""+
		"USERNAME=admin\n"+
		"PASSWORD=secret\n"+
		"STATISTICS_URL=\"http://192.168.1.254/web_whw/#/status/fastmile5gradio\"\n"+
		"OUT_DIR=\"/tmp/out.csv\"\n"+
		"MAILJET_API_KEY=key\n"+
		"MAILJET_API_SECRET=secretkey\n"+
		"MAILJET_FROM_EMAIL=from@example.com\n"+
		"MAILJET_TO_EMAIL=to@example.com\n")

	cfg, err := nokialogger.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Username != "admin" || cfg.Password != "secret" {
		t.Errorf("got username=%q password=%q", cfg.Username, cfg.Password)
	}
	if cfg.BaseURL != "http://192.168.1.254" {
		t.Errorf("BaseURL = %q, want scheme+host only", cfg.BaseURL)
	}
	if cfg.OutPath != "/tmp/out.csv" {
		t.Errorf("OutPath = %q, want /tmp/out.csv", cfg.OutPath)
	}
	if cfg.MailjetAPIKey != "key" || cfg.MailjetAPISecret != "secretkey" {
		t.Errorf("Mailjet credentials not loaded correctly: %+v", cfg)
	}
}

func TestLoadConfigDefaultOutPath(t *testing.T) {
	dir := withTempCwd(t)
	writeEnvFile(t, dir, ""+
		"USERNAME=admin\n"+
		"PASSWORD=secret\n"+
		"STATISTICS_URL=http://192.168.1.254/web_whw/\n")

	cfg, err := nokialogger.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if filepath.Base(cfg.OutPath) != "data.csv" {
		t.Errorf("default OutPath = %q, want it to end in data.csv", cfg.OutPath)
	}
}

func TestLoadConfigMissingRequiredFields(t *testing.T) {
	withTempCwd(t)
	// No .env file at all, and make sure no ambient env vars leak in from
	// the test runner's own environment.
	t.Setenv("USERNAME", "")
	t.Setenv("PASSWORD", "")
	t.Setenv("STATISTICS_URL", "")

	if _, err := nokialogger.LoadConfig(); err == nil {
		t.Fatal("expected an error when USERNAME/PASSWORD/STATISTICS_URL are all missing")
	}
}

func TestLoadConfigInvalidStatisticsURL(t *testing.T) {
	dir := withTempCwd(t)
	writeEnvFile(t, dir, ""+
		"USERNAME=admin\n"+
		"PASSWORD=secret\n"+
		"STATISTICS_URL=not-a-valid-url\n")

	if _, err := nokialogger.LoadConfig(); err == nil {
		t.Fatal("expected an error for a STATISTICS_URL with no scheme or host")
	}
}

// TestLoadConfigEnvFileWinsOverOSEnv reproduces the real bug this precedence
// exists to fix: Windows predefines a USERNAME environment variable for
// every process (the logged-in account name), which must not shadow the
// router username set in .env.
func TestLoadConfigEnvFileWinsOverOSEnv(t *testing.T) {
	dir := withTempCwd(t)
	writeEnvFile(t, dir, ""+
		"USERNAME=router-admin\n"+
		"PASSWORD=secret\n"+
		"STATISTICS_URL=http://192.168.1.254/\n")

	t.Setenv("USERNAME", "joshn") // simulates Windows's built-in USERNAME var

	cfg, err := nokialogger.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Username != "router-admin" {
		t.Errorf("Username = %q, want the .env value to win over the OS env var", cfg.Username)
	}
}

func TestLoadConfigFallsBackToOSEnvWhenNotInEnvFile(t *testing.T) {
	dir := withTempCwd(t)
	writeEnvFile(t, dir, ""+
		"USERNAME=admin\n"+
		"PASSWORD=secret\n"+
		"STATISTICS_URL=http://192.168.1.254/\n")

	t.Setenv("MAILJET_API_KEY", "from-os-env")

	cfg, err := nokialogger.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.MailjetAPIKey != "from-os-env" {
		t.Errorf("MailjetAPIKey = %q, want fallback to OS env var when .env doesn't set it", cfg.MailjetAPIKey)
	}
}
