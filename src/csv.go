package nokialogger

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const Megabyte = 1e6
const Gibibyte = 1_073_741_824

// AppendCSV appends one row to the CSV file at path, writing the header
// first if the file is new or empty. Each row records raw bytes and derived
// GiB for both stats sources (the FastMile Radio page and the Fastmile
// statistics page), plus the device uptime in seconds.
func AppendCSV(path, timestamp string, stats RouterStats) error {
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
			"radio_upload_bytes", "radio_upload_gib",
			"radio_download_bytes", "radio_download_gib",
			"stats_upload_mb", "stats_upload_gib",
			"stats_download_mb", "stats_download_gib",
			"uptime_seconds",
		}); err != nil {
			return err
		}
	}

	radioUploadGiB, err := ConvertBytes(stats.RadioUpload, Gibibyte)
	if err != nil {
		return fmt.Errorf("converting radio upload bytes to GiB: %w", err)
	}
	radioDownloadGiB, err := ConvertBytes(stats.RadioDownload, Gibibyte)
	if err != nil {
		return fmt.Errorf("converting radio download bytes to GiB: %w", err)
	}
	// Fastmile statistics page shows values in MB, GiB/MB == convert to bytes then GiB
	statsUploadGiB, err := ConvertBytes(stats.StatsUpload, Gibibyte/Megabyte)
	if err != nil {
		return fmt.Errorf("converting stats upload bytes to GiB: %w", err)
	}
	statsDownloadGiB, err := ConvertBytes(stats.StatsDownload, Gibibyte/Megabyte)
	if err != nil {
		return fmt.Errorf("converting stats download bytes to GiB: %w", err)
	}

	return w.Write([]string{
		timestamp,
		stats.RadioUpload.String(), radioUploadGiB,
		stats.RadioDownload.String(), radioDownloadGiB,
		stats.StatsUpload.String(), statsUploadGiB,
		stats.StatsDownload.String(), statsDownloadGiB,
		stats.Uptime.String(),
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
