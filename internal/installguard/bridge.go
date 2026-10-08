package installguard

import (
	"errors"
	"fmt"
	"strings"
)

const (
	bridgeQuiet  = "quiet"
	bridgeMusic  = "music"
	bridgeRepack = "repack-"
	bridgeVerify = "verify-"
)

var errBridgeMode = errors.New("unknown bridge mode")

type Bridge struct {
	Options                Options
	Limit32BitAddressSpace bool
}

func (b Bridge) Mode() string {
	mode := bridgeMusic
	if b.Options.HideProgress {
		mode = bridgeQuiet
	}
	if b.Options.VerifyRepack {
		mode = bridgeVerify + mode
	}
	if b.Limit32BitAddressSpace {
		mode = bridgeRepack + mode
	}
	return mode
}

func ParseBridgeMode(token string) (Bridge, error) {
	var b Bridge
	rest := token
	if after, ok := strings.CutPrefix(rest, bridgeRepack); ok {
		b.Limit32BitAddressSpace = true
		rest = after
	}
	if after, ok := strings.CutPrefix(rest, bridgeVerify); ok {
		b.Options.VerifyRepack = true
		rest = after
	}
	switch rest {
	case bridgeQuiet:
		b.Options.HideProgress = true
	case bridgeMusic:
	default:
		return Bridge{}, fmt.Errorf("%w: %q", errBridgeMode, token)
	}
	return b, nil
}
