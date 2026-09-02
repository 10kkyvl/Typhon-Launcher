//go:build !devmock

package library

func newGameStarter() gameStarter { return execStarter }
