//go:build qa && windows

package selfupdate

import (
	"log/slog"
	"os"

	"typhon/internal/account"
)

// manifestBaseURL also checks the key, so a bad or half-set qa source stops
// the service at construction instead of at the first check.
func manifestBaseURL() (string, error) {
	src, ok, err := parseQASource(os.LookupEnv)
	if err != nil {
		return "", err
	}
	if !ok {
		return account.BaseURL(), nil
	}
	slog.Warn("qa build: launcher updates come from a test source signed with a test key", "url", src.baseURL)
	return src.baseURL, nil
}
