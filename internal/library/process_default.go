//go:build !devmock && !darwin && !windows

package library

func newGameStarter() gameStarter { return execStarter }
