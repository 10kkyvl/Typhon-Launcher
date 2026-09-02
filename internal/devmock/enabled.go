//go:build !devmock

package devmock

const Enabled = false

// Banner returns the empty string here so the devmock marker literal never
// exists in a non-devmock binary for the release guard to find by accident.
func Banner() string { return "" }
