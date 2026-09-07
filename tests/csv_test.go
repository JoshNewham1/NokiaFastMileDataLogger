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
		{"1048576", nokialogger.Mebibyte, "1.00"},
		{"1073741824", nokialogger.Gibibyte, "1.00"},
		{"0", nokialogger.Mebibyte, "0.00"},
		{"1500000", nokialogger.Mebibyte, "1.43"},
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
	if _, err := nokialogger.ConvertBytes(json.Number("not-a-number"), nokialogger.Mebibyte); err == nil {
		t.Fatal("expected an error converting a malformed json.Number")
	}
}

func TestAppendCSVWritesHeaderOnce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.csv")

	if err := nokialogger.AppendCSV(path, "2026-01-01T00:00:00Z", "1048576", "2097152", "100"); err != nil {
		t.Fatalf("first AppendCSV: %v", err)
	}
	if err := nokialogger.AppendCSV(path, "2026-01-01T00:01:00Z", "3145728", "4194304", "200"); err != nil {
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
		"upload_bytes", "upload_mib", "upload_gib",
		"download_bytes", "download_mib", "download_gib",
		"uptime_seconds",
	}
	if strings.Join(records[0], ",") != strings.Join(wantHeader, ",") {
		t.Errorf("header = %v, want %v", records[0], wantHeader)
	}
	if records[1][1] != "1048576" || records[1][2] != "1.00" {
		t.Errorf("first row bytes/MiB = %v", records[1])
	}
	if records[2][7] != "200" {
		t.Errorf("second row uptime = %q, want 200", records[2][7])
	}
}

func TestAppendCSVCreatesMissingDirectories(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "sub", "data.csv")

	if err := nokialogger.AppendCSV(path, "2026-01-01T00:00:00Z", "0", "0", "0"); err != nil {
		t.Fatalf("AppendCSV into a missing directory tree: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected the CSV file to exist: %v", err)
	}
}

func TestAppendCSVInvalidUploadBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.csv")

	err := nokialogger.AppendCSV(path, "2026-01-01T00:00:00Z", json.Number("bad"), "0", "0")
	if err == nil || !strings.Contains(err.Error(), "converting upload bytes to MB") {
		t.Fatalf("got err=%v, want an upload-conversion error", err)
	}
}

func TestAppendCSVInvalidDownloadBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.csv")

	err := nokialogger.AppendCSV(path, "2026-01-01T00:00:00Z", "0", json.Number("bad"), "0")
	if err == nil || !strings.Contains(err.Error(), "converting download bytes to MB") {
		t.Fatalf("got err=%v, want a download-conversion error", err)
	}
}
