//go:build !devmock

package selfupdate

// appliedKinds lists, best first, the artifact kinds this platform's Apply
// can consume: the Windows installer runs an installer, the macOS updater
// swaps in a bundle.
func appliedKinds(goos string) []Kind {
	if goos == "darwin" {
		return []Kind{KindBundle}
	}
	return []Kind{KindInstaller}
}
