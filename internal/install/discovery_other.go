//go:build !windows && !devmock

package install

import "context"

func attemptDiscovery(_ context.Context, in discoverySpec) (discoveryOutcome, error) {
	if !shouldDiscoverComponents(in) {
		return discoveryOutcome{}, nil
	}
	return discoveryOutcome{reason: "component discovery is only supported on Windows"}, nil
}
