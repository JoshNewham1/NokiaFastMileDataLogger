# Nokia FastMile Data Logger

A small cross-platform tool that logs into a Nokia FastMile 5G router's web UI, reads the Cellular Packets Upload/Download totals and device uptime, and appends a timestamped row to a CSV file. It collects one data point per execution, and is intended to be triggered by Windows Task Scheduler/cron at will.

## Setup

Create a `.env` file next to the executable (see `.env.example`) with:

- `USERNAME`, `PASSWORD` - router admin login
- `STATISTICS_URL` - the router's status page URL (e.g. `http://192.168.1.254/web_whw/#/status/fastmile5gradio`); only the scheme and host are used, the rest is just where you'd view this in a browser
- `OUT_DIR` - path to the CSV file to append to (optional; defaults to `data.csv` next to the executable)
- `MAILJET_API_KEY`, `MAILJET_API_SECRET`, `MAILJET_FROM_EMAIL`, `MAILJET_TO_EMAIL` - optional. If all four are set, a failure sends an email through Mailjet. If any are missing, failures print to stderr instead

## How it works

1. `GET /login_web_app.cgi?nonce` for a nonce, a random key, and an RSA public key. The public key goes unused here - the router's web UI sends the login's AES key/IV as plain random bytes instead of RSA-encrypting them, and this tool copies that instead of guessing at something stronger.
2. Derive the login hashes (SHA256-based, matching the router's own JavaScript) and `POST` them to `/login_web_app.cgi`, which sets a session cookie.
3. `GET /fastmile_radio_status_web_app.cgi` for `cellular_stats[].BytesSent` / `BytesReceived`, in raw bytes, and derive MiB (divide by 1,048,576) and GiB (divide by 1,073,741,824) from each.
4. `GET /device_status_web_app.cgi` for `UpTime`, in seconds.
5. Append one CSV row: timestamp, upload bytes/MiB/GiB, download bytes/MiB/GiB, uptime (seconds).
6. On any failure, send a failure email through Mailjet's HTTP API if configured, then exit. On success, exit silently.

There's no retry logic anywhere in this tool. Each run is a single attempt; the next attempt is whatever triggers the next run.

## Layout

- `src/` - the `nokialogger` library package (config loading, login, stats fetch, CSV output, failure email) plus `src/cmd/nokia_logger/main.go`, a one-line entrypoint that calls it
- `tests/` - black-box tests (`package nokialogger_test`) exercising `src/`'s exported API against fake HTTP servers, no real router required

## Building

Cross-compile from Linux or macOS:

```
GOOS=windows GOARCH=amd64 go build -o nokia_logger.exe ./src/cmd/nokia_logger
```

## Testing

```
go test ./tests/...
```

## Scheduling on Windows

Add a Task Scheduler task with an "At startup" trigger that runs `nokia_logger.exe` with its working directory set to wherever `.env` lives. If you want more than one data point per boot, add a second trigger with a repeat interval, and set the task to skip a new instance if one's already running.

## Limitations

- Built and tested against one model, the Nokia FastMile 5G-24W-A. Other models or firmware versions may use a different login flow or different field names.
- We use MiB and GiB, alongside raw bytes, as that is what is shown on the Fastmile Radio page
