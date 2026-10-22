package account

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"runtime"
	"strings"

	"typhon/internal/app"
)

const APIPrefix = "/v1"

var apiBaseURL = "https://api.typhon-launcher.com"

const siteBaseURL = "https://typhon-launcher.com"

// StatusPageURL lives on the vendor subdomain on purpose: when typhon-launcher.com
// itself is down (DNS, certificate), a page on our own domain would be down too.
const StatusPageURL = "https://typhon.betteruptime.com"

const (
	forgotPasswordPath = "/account/forgot"
	supportPath        = "/support"
)

func sitePageURL(language, path string) string {
	if language == "en" {
		return siteBaseURL + "/en" + path
	}
	return siteBaseURL + path
}

func ForgotPasswordURL(language string) string {
	return sitePageURL(language, forgotPasswordPath)
}

func SupportURL(language string) string {
	return sitePageURL(language, supportPath)
}

var UserAgent = fmt.Sprintf("Typhon/%s (%s/%s)", app.Version, runtime.GOOS, runtime.GOARCH)

var (
	ErrEmptyBaseURL    = errors.New("api base url is empty")
	ErrInsecureBaseURL = errors.New("api base url must use https outside loopback")
)

func BaseURL() string {
	base := os.Getenv("TYPHON_API_URL")
	if base == "" {
		base = apiBaseURL
	}
	return strings.TrimSuffix(base, "/")
}

func ValidateBaseURL(raw string) (string, error) {
	trimmed := strings.TrimSuffix(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return "", ErrEmptyBaseURL
	}

	u, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("parse api base url: %w", err)
	}
	if u.Host == "" {
		return "", fmt.Errorf("%w: missing host", ErrInsecureBaseURL)
	}
	if err := checkURLScheme(u); err != nil {
		return "", err
	}
	return trimmed, nil
}

func CheckURLScheme(u *url.URL) error {
	return checkURLScheme(u)
}

func checkURLScheme(u *url.URL) error {
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		if isLoopbackHost(u.Hostname()) {
			return nil
		}
		return fmt.Errorf("%w: plain http host %q", ErrInsecureBaseURL, u.Hostname())
	default:
		return fmt.Errorf("%w: unsupported scheme %q", ErrInsecureBaseURL, u.Scheme)
	}
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
