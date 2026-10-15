//go:build !windows

package screenshots

func grabWindow(uintptr) (grabbed, error) {
	return grabbed{}, ErrUnsupported
}
