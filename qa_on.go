//go:build qa && windows

package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
)

const (
	qaMarker      = "TYPHON_QA_ENABLED"
	qaPortEnv     = "TYPHON_QA_CDP_PORT"
	qaDefaultPort = 9333
	qaMinPort     = 1024
	qaTitleSuffix = " [qa]"
)

var errQAPort = errors.New("invalid " + qaPortEnv)

func qaPort(lookup func(string) (string, bool)) (int, error) {
	raw, set := lookup(qaPortEnv)
	if !set {
		return qaDefaultPort, nil
	}
	port, err := strconv.ParseUint(raw, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("%w: %q, want an integer in %d..65535 (%w)", errQAPort, raw, qaMinPort, err)
	}
	if port < qaMinPort {
		return 0, fmt.Errorf("%w: %q, want an integer in %d..65535", errQAPort, raw, qaMinPort)
	}
	return int(port), nil
}

func qaStart() ([]string, error) {
	port, err := qaPort(os.LookupEnv)
	if err != nil {
		return nil, err
	}
	// WebView2 listens on 127.0.0.1 only (Get-NetTCPConnection shows no ::1 or
	// 0.0.0.0 socket), so no --remote-debugging-address is passed or relied on.
	slog.Warn("qa build: CDP on 127.0.0.1", "port", port, "marker", qaMarker)
	return []string{fmt.Sprintf("--remote-debugging-port=%d", port)}, nil
}
