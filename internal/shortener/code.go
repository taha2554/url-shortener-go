package shortener

import (
	"crypto/rand"
	"errors"
	"math/big"
	"net"
	"net/url"
	"regexp"
	"strings"
)

const alphabet = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no 0/O/1/l/I

var aliasPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{3,32}$`)

// reserved paths that must never be used as a short code
var reserved = map[string]bool{"api": true, "static": true, "health": true, "favicon.ico": true}

func RandomCode(n int) (string, error) {
	var b strings.Builder
	max := big.NewInt(int64(len(alphabet)))
	for i := 0; i < n; i++ {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b.WriteByte(alphabet[idx.Int64()])
	}
	return b.String(), nil
}

func ValidAlias(alias string) error {
	if !aliasPattern.MatchString(alias) {
		return errors.New("alias must be 3-32 characters: letters, numbers, - or _")
	}
	if reserved[strings.ToLower(alias)] {
		return errors.New("alias is reserved")
	}
	return nil
}

// NormalizeURL accepts http(s) URLs, adds https:// when the scheme is missing
// and refuses links that point at localhost or private networks.
func NormalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("url is required")
	}
	if len(raw) > 2048 {
		return "", errors.New("url is too long")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", errors.New("url is not valid")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("only http and https links are allowed")
	}
	host := u.Hostname()
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return "", errors.New("local addresses are not allowed")
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast()) {
		return "", errors.New("private addresses are not allowed")
	}
	return u.String(), nil
}
