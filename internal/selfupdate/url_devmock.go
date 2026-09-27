//go:build devmock && !windows

package selfupdate

import (
	"fmt"
	"os"
	"strings"

	"typhon/internal/account"
)

const manifestURLEnv = "TYPHON_DEVMOCK_MANIFEST_URL"

func manifestBaseURL() (string, error) {
	raw := strings.TrimSpace(os.Getenv(manifestURLEnv))
	if raw == "" {
		return account.BaseURL(), nil
	}
	base, err := account.ValidateBaseURL(raw)
	if err != nil {
		return "", fmt.Errorf("%s: %w", manifestURLEnv, err)
	}
	return base, nil
}
