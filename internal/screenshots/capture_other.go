//go:build !windows

package screenshots

func grabWindow(uintptr) (grabbed, error) {
	return grabbed{}, ErrUnsupported
}

func windowOwner(uintptr) (uint32, error) {
	return 0, ErrUnsupported
}
