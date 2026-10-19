# url-shortener-go

![CI](https://github.com/taha2554/url-shortener-go/actions/workflows/ci.yml/badge.svg)

A small URL shortener written in Go using only the standard library. It has a JSON API, a one-page web form, click statistics, optional expiry and per-IP rate limiting.

## Features

- `POST /api/links` – create a short link (random 7-character code or a custom alias)
- `GET /{code}` – redirect (302) and count the click
- `GET /api/links/{code}` – stats: target URL, clicks, last click, expiry
- Optional `expires_in` (`"1h"`, `"24h"` …) – expired links return `410 Gone`
- URL validation: http/https only, adds `https://` when missing, blocks localhost and private IP ranges
- Token-bucket rate limiter per client IP on link creation
- Links persisted to a JSON file (written atomically via temp file + rename), or kept in memory
- Graceful shutdown, request logging, server timeouts
- Dockerfile with a distroless runtime image

## API example

```bash
curl -s -X POST localhost:8080/api/links \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://go.dev/doc/effective_go","alias":"effective-go","expires_in":"168h"}'
```

```json
{
  "code": "effective-go",
  "url": "https://go.dev/doc/effective_go",
  "created_at": "2026-10-19T14:02:11Z",
  "expires_at": "2026-10-26T14:02:11Z",
  "clicks": 0,
  "short_url": "http://localhost:8080/effective-go"
}
```

Errors are returned as `{"error": "..."}` with a matching status code (400, 404, 409, 410, 429).

## Running

```bash
go run ./cmd/server                 # http://localhost:8080
go run ./cmd/server -data ""        # in-memory only
```

| Flag | Env var | Default |
|---|---|---|
| `-addr` | `ADDR` | `:8080` |
| `-base-url` | `BASE_URL` | `http://localhost:8080` |
| `-data` | `DATA_FILE` | `links.json` |

With Docker:

```bash
docker build -t url-shortener .
docker run -p 8080:8080 -v shortener-data:/data url-shortener
```

## Tests

```bash
go test -race ./...
```

The handler tests use `httptest` with a fake clock so expiry and rate limiting can be tested without sleeping.

## Layout

```
cmd/server            entry point, flags, graceful shutdown
internal/shortener    HTTP handlers, URL/alias validation, rate limiter, HTML page
internal/store        Store interface, in-memory and JSON-file implementations
```

## License

MIT
