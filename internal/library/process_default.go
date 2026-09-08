//go:build !devmock && !darwin

package library

func newGameStarter() gameStarter { return execStarter }
