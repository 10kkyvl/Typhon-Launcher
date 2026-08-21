package account

import (
	"os"
	"strings"
)

var apiBaseURL = "http://127.0.0.1:8080"

func BaseURL() string {
	base := os.Getenv("TYPHON_API_URL")
	if base == "" {
		base = apiBaseURL
	}
	return strings.TrimSuffix(base, "/")
}
