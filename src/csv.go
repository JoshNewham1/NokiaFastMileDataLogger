package nokialogger

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Unit sizes used to convert the router's raw byte counts. These are
// mebibytes/gibibytes (1024-based), not decimal megabytes/gigabytes,
// matching how Three defines MB/GB for data allowances.
const (
	Mebibyte = 1_048_576
	Gibibyte = 1_073_741_824
)

// AppendCSV appends one row (timestamp, upload/download in bytes and
// derived MiB/GiB, uptime in seconds) to the CSV file at path, writing the
// header first if the file is new or empty.
func AppendCSV(path, timestamp string, upload, download, uptime json.Number) error {
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

	uploadMB, err := ConvertBytes(upload, Mebibyte)
	if err != nil {
		return fmt.Errorf("converting upload bytes to MB: %w", err)
	}
	uploadGiB, err := ConvertBytes(upload, Gibibyte)
	if err != nil {
		return fmt.Errorf("converting upload bytes to GiB: %w", err)
	}
	downloadMB, err := ConvertBytes(download, Mebibyte)
	if err != nil {
		return fmt.Errorf("converting download bytes to MB: %w", err)
	}
	downloadGiB, err := ConvertBytes(download, Gibibyte)
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

// ConvertBytes converts a raw byte count (as reported by the router) to the
// given unit size, formatted to two decimal places.
func ConvertBytes(bytes json.Number, unitSize float64) (string, error) {
	value, err := bytes.Float64()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%.2f", value/unitSize), nil
}
