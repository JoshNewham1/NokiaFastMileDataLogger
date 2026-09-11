package nokialogger_test

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	nokialogger "nokia_logger/src"
)

func TestConvertBytes(t *testing.T) {
	cases := []struct {
		bytes    json.Number
		unitSize float64
		want     string
	}{
		{"1073741824", nokialogger.Gibibyte, "1.00"},
		{"0", nokialogger.Gibibyte, "0.00"},
		{"1200000000", nokialogger.Gibibyte, "1.12"},
	}
	for _, c := range cases {
		got, err := nokialogger.ConvertBytes(c.bytes, c.unitSize)
		if err != nil {
			t.Errorf("ConvertBytes(%v, %v): %v", c.bytes, c.unitSize, err)
			continue
		}
		if got != c.want {
			t.Errorf("ConvertBytes(%v, %v) = %q, want %q", c.bytes, c.unitSize, got, c.want)
		}
	}
}

func TestConvertBytesInvalidNumber(t *testing.T) {
	if _, err := nokialogger.ConvertBytes(json.Number("not-a-number"), nokialogger.Gibibyte); err == nil {
		t.Fatal("expected an error converting a malformed json.Number")
	}
}

func TestAppendCSVWritesHeaderOnce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.csv")

	first := nokialogger.RouterStats{
		RadioUpload: "1073741824", RadioDownload: "2097152",
		StatsUpload: "500000", StatsDownload: "600000",
		Uptime: "100",
	}
	if err := nokialogger.AppendCSV(path, "2026-01-01T00:00:00Z", first); err != nil {
		t.Fatalf("first AppendCSV: %v", err)
	}

	second := nokialogger.RouterStats{
		RadioUpload: "3145728", RadioDownload: "4194304",
		StatsUpload: "700000", StatsDownload: "800000",
		Uptime: "200",
	}
	if err := nokialogger.AppendCSV(path, "2026-01-01T00:01:00Z", second); err != nil {
		t.Fatalf("second AppendCSV: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatalf("reading back CSV: %v", err)
	}
	if len(records) != 3 { // header + 2 rows
		t.Fatalf("got %d records, want 3 (header + 2 rows)", len(records))
	}

	wantHeader := []string{
		"timestamp",
		"radio_upload_bytes", "radio_upload_gib",
		"radio_download_bytes", "radio_download_gib",
		"stats_upload_mb", "stats_upload_gib",
		"stats_download_mb", "stats_download_gib",
		"uptime_seconds",
	}
	if strings.Join(records[0], ",") != strings.Join(wantHeader, ",") {
		t.Errorf("header = %v, want %v", records[0], wantHeader)
	}
	if records[1][1] != "1073741824" || records[1][2] != "1.00" {
		t.Errorf("first row radio upload bytes/GiB = %v", records[1])
	}
	if records[2][9] != "200" {
		t.Errorf("second row uptime = %q, want 200", records[2][9])
	}
}

func TestAppendCSVCreatesMissingDirectories(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "sub", "data.csv")

	stats := nokialogger.RouterStats{
		RadioUpload: "0", RadioDownload: "0",
		StatsUpload: "0", StatsDownload: "0",
		Uptime: "0",
	}
	if err := nokialogger.AppendCSV(path, "2026-01-01T00:00:00Z", stats); err != nil {
		t.Fatalf("AppendCSV into a missing directory tree: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected the CSV file to exist: %v", err)
	}
}

func TestAppendCSVInvalidRadioUploadBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.csv")

	stats := nokialogger.RouterStats{
		RadioUpload: json.Number("bad"), RadioDownload: "0",
		StatsUpload: "0", StatsDownload: "0",
		Uptime: "0",
	}
	err := nokialogger.AppendCSV(path, "2026-01-01T00:00:00Z", stats)
	if err == nil || !strings.Contains(err.Error(), "converting radio upload bytes to GiB") {
		t.Fatalf("got err=%v, want a radio-upload-conversion error", err)
	}
}

func TestAppendCSVInvalidRadioDownloadBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.csv")

	stats := nokialogger.RouterStats{
		RadioUpload: "0", RadioDownload: json.Number("bad"),
		StatsUpload: "0", StatsDownload: "0",
		Uptime: "0",
	}
	err := nokialogger.AppendCSV(path, "2026-01-01T00:00:00Z", stats)
	if err == nil || !strings.Contains(err.Error(), "converting radio download bytes to GiB") {
		t.Fatalf("got err=%v, want a radio-download-conversion error", err)
	}
}

func TestAppendCSVInvalidStatsUploadBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.csv")

	stats := nokialogger.RouterStats{
		RadioUpload: "0", RadioDownload: "0",
		StatsUpload: json.Number("bad"), StatsDownload: "0",
		Uptime: "0",
	}
	err := nokialogger.AppendCSV(path, "2026-01-01T00:00:00Z", stats)
	if err == nil || !strings.Contains(err.Error(), "converting stats upload bytes to GiB") {
		t.Fatalf("got err=%v, want a stats-upload-conversion error", err)
	}
}

func TestAppendCSVInvalidStatsDownloadBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.csv")

	stats := nokialogger.RouterStats{
		RadioUpload: "0", RadioDownload: "0",
		StatsUpload: "0", StatsDownload: json.Number("bad"),
		Uptime: "0",
	}
	err := nokialogger.AppendCSV(path, "2026-01-01T00:00:00Z", stats)
	if err == nil || !strings.Contains(err.Error(), "converting stats download bytes to GiB") {
		t.Fatalf("got err=%v, want a stats-download-conversion error", err)
	}
}
