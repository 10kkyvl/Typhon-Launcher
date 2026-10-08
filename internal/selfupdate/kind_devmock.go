//go:build devmock && !windows

package selfupdate

// The mock Apply copies whatever artifact it is given over the launcher and
// never looks at its kind, and the release tooling publishes "installer" for
// every OS, macOS included.
func appliedKinds(string) []Kind {
	return []Kind{KindInstaller, KindBundle}
}
