//go:build !windows

package jumplist

func Supported() bool { return false }

func Apply(Config, []Game) error { return ErrUnsupported }
