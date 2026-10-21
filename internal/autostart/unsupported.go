package autostart

import (
	"strings"

	"typhon/internal/uierr"
)

const CodeUnavailable = "autostart.unavailable"

type unsupported struct {
	reason string
}

func Unsupported(reason string) Manager {
	if strings.TrimSpace(reason) == "" {
		reason = "autostart is unavailable"
	}
	return unsupported{reason: reason}
}

func (u unsupported) Enable() error {
	return uierr.New(CodeUnavailable, u.reason)
}

func (unsupported) Disable() error {
	return nil
}

func (unsupported) IsEnabled() (bool, error) {
	return false, nil
}
