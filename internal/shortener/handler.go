package shortener

import (
	"embed"
	"encoding/json"
	"errors"
	"html/template"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/taha2554/url-shortener-go/internal/store"
)

//go:embed templates/index.html
var templates embed.FS

var indexTmpl = template.Must(template.ParseFS(templates, "templates/index.html"))

type Server struct {
	Store   store.Store
	BaseURL string
	Limiter *RateLimiter
	Now     func() time.Time
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("POST /api/links", s.createLink)
	mux.HandleFunc("GET /api/links/{code}", s.linkStats)
	mux.HandleFunc("GET /{code}", s.redirect)
	return logRequests(mux)
}

type createRequest struct {
	URL       string `json:"url"`
	Alias     string `json:"alias,omitempty"`
	ExpiresIn string `json:"expires_in,omitempty"` // Go duration, e.g. "24h"
}

type linkResponse struct {
	store.Link
	ShortURL string `json:"short_url"`
}

func (s *Server) createLink(w http.ResponseWriter, r *http.Request) {
	if s.Limiter != nil && !s.Limiter.Allow(clientIP(r)) {
		writeError(w, http.StatusTooManyRequests, "too many requests, slow down")
		return
	}

	var req createRequest
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	target, err := NormalizeURL(req.URL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	now := s.now()
	link := store.Link{URL: target, CreatedAt: now}
	if req.ExpiresIn != "" {
		d, err := time.ParseDuration(req.ExpiresIn)
		if err != nil || d <= 0 || d > 365*24*time.Hour {
			writeError(w, http.StatusBadRequest, "expires_in must be a duration between 1s and 8760h")
			return
		}
		exp := now.Add(d)
		link.ExpiresAt = &exp
	}

	if req.Alias != "" {
		if err := ValidAlias(req.Alias); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		link.Code = req.Alias
		if err := s.Store.Create(link); errors.Is(err, store.ErrExists) {
			writeError(w, http.StatusConflict, "that alias is already taken")
			return
		} else if err != nil {
			internalError(w, err)
			return
		}
	} else {
		// random codes collide very rarely, retry a few times just in case
		for attempt := 0; ; attempt++ {
			code, err := RandomCode(7)
			if err != nil {
				internalError(w, err)
				return
			}
			link.Code = code
			err = s.Store.Create(link)
			if err == nil {
				break
			}
			if !errors.Is(err, store.ErrExists) || attempt == 4 {
				internalError(w, err)
				return
			}
		}
	}

	writeJSON(w, http.StatusCreated, s.response(link))
}

func (s *Server) linkStats(w http.ResponseWriter, r *http.Request) {
	link, err := s.Store.Get(r.PathValue("code"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "link not found")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.response(link))
}

func (s *Server) redirect(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	link, err := s.Store.Get(code)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	now := s.now()
	if link.Expired(now) {
		http.Error(w, "this link has expired", http.StatusGone)
		return
	}
	if err := s.Store.RecordClick(code, now); err != nil {
		log.Printf("record click %s: %v", code, err)
	}
	http.Redirect(w, r, link.URL, http.StatusFound)
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	indexTmpl.Execute(w, nil)
}

func (s *Server) response(link store.Link) linkResponse {
	return linkResponse{Link: link, ShortURL: strings.TrimRight(s.BaseURL, "/") + "/" + link.Code}
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func internalError(w http.ResponseWriter, err error) {
	log.Printf("internal error: %v", err)
	writeError(w, http.StatusInternalServerError, "something went wrong")
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Microsecond))
	})
}
