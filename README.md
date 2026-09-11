# Nokia FastMile Data Logger

A small cross-platform tool that logs into a Nokia FastMile 5G router's web UI, reads the Cellular Packets Upload/Download totals from both the FastMile Radio page and Bytes Sent/Received from the Fastmile statistics page, and appends a timestamped row to a CSV file. It collects one data point per execution, and is intended to be triggered by Windows Task Scheduler/cron at will.

## Setup

Create a `.env` file next to the executable (see `.env.example`) with:

- `USERNAME`, `PASSWORD` - router admin login
- `STATISTICS_URL` - the router's status page URL (e.g. `http://192.168.1.254/web_whw/#/status/fastmile5gradio`); only the scheme and host are used, the rest is just where you'd view this in a browser
- `OUT_DIR` - path to the CSV file to append to (optional; defaults to `data.csv` next to the executable)
- `MAILJET_API_KEY`, `MAILJET_API_SECRET`, `MAILJET_FROM_EMAIL`, `MAILJET_TO_EMAIL` - optional. If all four are set, a failure sends an email through Mailjet. If any are missing, failures print to stderr instead

## How it works

This logs into the router's admin page and reads Upload/Download totals from two different pages plus the device's running time. Instead of a browser, it makes those same requests directly:

- **Status > FastMile Radio** - the original counters this tool has always logged.
- **Status > Fastmile statistics** - a separate counter on the router that's believed to be more consistent/accurate than the Radio page's. Logged alongside the Radio numbers, not instead of them, so the two can be compared over time.

Both are fetched using the same logged-in session, no extra login step. Each run writes one CSV row with the **timestamp, both sources' upload/download figures, and uptime**.

If anything goes wrong (can't log in, can't reach the router, can't write the file), it sends a failure email if Mailjet is configured, otherwise it just prints the error. Either way, it doesn't retry. Each run is one attempt.

## Layout

- `src/` - the `nokialogger` library package (config loading, login, stats fetch, CSV output, failure email) plus `src/cmd/nokia_logger/main.go`, a one-line entrypoint that calls it
- `tests/` - black-box tests (`package nokialogger_test`) exercising `src/`'s exported API against fake HTTP servers, no real router required

## Building

Build for Linux:
```
go build -o nokia_logger ./src/cmd/nokia_logger
```

Cross-compile for Windows (from Linux):

```
GOOS=windows GOARCH=amd64 go build -o nokia_logger.exe ./src/cmd/nokia_logger
```

## Testing

```
go test ./tests/...
```

## Scheduling on Windows

Add a Task Scheduler task with an "At startup" trigger that runs `nokia_logger.exe` with its working directory set to wherever `.env` lives. If you want more than one data point per boot, add a second trigger with a repeat interval, and set the task to skip a new instance if one's already running.

### Technical notes

- Login uses the same nonce/SHA256 challenge as the router's own JavaScript, POSTed to `/login_web_app.cgi` to get a session cookie. The nonce response also includes an RSA public key that the router's own web UI doesn't use for this step, it sends the AES key/IV as plain random bytes instead, and we copy this behaviour.
- FastMile Radio page stats come from `GET /fastmile_radio_status_web_app.cgi` (`cellular_stats[].BytesSent`/`BytesReceived`, in raw bytes).
- Fastmile statistics page stats come from `GET /fastmile_statistics_status_web_app.cgi` (`stats_cfg[0].BytesSent`/`BytesReceived`, in raw bytes).
- Device uptime comes from `GET /device_status_web_app.cgi` (`UpTime`, in seconds).
- The CSV records each source's upload/download as raw bytes plus derived GiB (divide by 1,073,741,824), with columns prefixed `radio_` and `stats_` respectively (e.g. `radio_upload_bytes`, `stats_download_gib`), plus `uptime_seconds`.


## Limitations

- Built and tested against one model, the Nokia FastMile 5G-24W-A. Other models or firmware versions may use a different login flow or different field names.
