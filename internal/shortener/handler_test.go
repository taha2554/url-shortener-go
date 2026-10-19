package shortener

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/taha2554/url-shortener-go/internal/store"
)

type testServer struct {
	*Server
	now time.Time
	h   http.Handler
}

func newTestServer() *testServer {
	ts := &testServer{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	ts.Server = &Server{Store: store.NewMemory(), BaseURL: "http://sho.rt/", Now: func() time.Time { return ts.now }}
	ts.h = ts.Routes()
	return ts
}

func (ts *testServer) do(method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	ts.h.ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("bad json %q: %v", rec.Body.String(), err)
	}
	return m
}

func TestCreateAndRedirect(t *testing.T) {
	ts := newTestServer()
	rec := ts.do("POST", "/api/links", `{"url":"example.com/page?x=1"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	body := decode(t, rec)
	code := body["code"].(string)
	if len(code) != 7 {
		t.Errorf("code length = %d", len(code))
	}
	if body["short_url"] != "http://sho.rt/"+code {
		t.Errorf("short_url = %v", body["short_url"])
	}

	rec = ts.do("GET", "/"+code, "")
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "https://example.com/page?x=1" {
		t.Fatalf("redirect: %d %q", rec.Code, rec.Header().Get("Location"))
	}

	stats := decode(t, ts.do("GET", "/api/links/"+code, ""))
	if stats["clicks"].(float64) != 1 {
		t.Errorf("clicks = %v", stats["clicks"])
	}
}

func TestCustomAlias(t *testing.T) {
	ts := newTestServer()
	if rec := ts.do("POST", "/api/links", `{"url":"https://go.dev","alias":"golang"}`); rec.Code != http.StatusCreated {
		t.Fatalf("first: %d", rec.Code)
	}
	if rec := ts.do("POST", "/api/links", `{"url":"https://go.dev","alias":"golang"}`); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate alias: %d", rec.Code)
	}
	for _, alias := range []string{"ab", "has space", "api"} {
		if rec := ts.do("POST", "/api/links", `{"url":"https://go.dev","alias":"`+alias+`"}`); rec.Code != http.StatusBadRequest {
			t.Errorf("alias %q accepted: %d", alias, rec.Code)
		}
	}
}

func TestExpiry(t *testing.T) {
	ts := newTestServer()
	ts.do("POST", "/api/links", `{"url":"https://go.dev","alias":"soon","expires_in":"1h"}`)
	if rec := ts.do("GET", "/soon", ""); rec.Code != http.StatusFound {
		t.Fatalf("before expiry: %d", rec.Code)
	}
	ts.now = ts.now.Add(2 * time.Hour)
	if rec := ts.do("GET", "/soon", ""); rec.Code != http.StatusGone {
		t.Fatalf("after expiry: %d", rec.Code)
	}
	if rec := ts.do("POST", "/api/links", `{"url":"https://go.dev","expires_in":"forever"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad duration accepted: %d", rec.Code)
	}
}

func TestBadInput(t *testing.T) {
	ts := newTestServer()
	cases := []string{`not json`, `{"url":""}`, `{"url":"ftp://x.com"}`, `{"url":"http://localhost:3000"}`, `{"url":"http://192.168.1.1/admin"}`}
	for _, body := range cases {
		if rec := ts.do("POST", "/api/links", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s -> %d", body, rec.Code)
		}
	}
	if rec := ts.do("GET", "/nope", ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown code -> %d", rec.Code)
	}
}

func TestRateLimit(t *testing.T) {
	ts := newTestServer()
	ts.Limiter = NewRateLimiter(60, 2)
	ts.Limiter.now = func() time.Time { return ts.now }
	for i := 0; i < 2; i++ {
		if rec := ts.do("POST", "/api/links", `{"url":"https://go.dev"}`); rec.Code != http.StatusCreated {
			t.Fatalf("request %d: %d", i, rec.Code)
		}
	}
	if rec := ts.do("POST", "/api/links", `{"url":"https://go.dev"}`); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
	ts.now = ts.now.Add(time.Second) // one token refills per second at 60/min
	if rec := ts.do("POST", "/api/links", `{"url":"https://go.dev"}`); rec.Code != http.StatusCreated {
		t.Fatalf("after refill: %d", rec.Code)
	}
}

func TestIndexPage(t *testing.T) {
	rec := newTestServer().do("GET", "/", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Shorten a link") {
		t.Fatalf("index: %d", rec.Code)
	}
}
