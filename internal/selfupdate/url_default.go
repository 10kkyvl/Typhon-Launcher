//go:build !devmock

package selfupdate

import "typhon/internal/account"

func manifestBaseURL() (string, error) {
	return account.BaseURL(), nil
}
