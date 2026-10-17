//go:build !windows

package shortcut

import "context"

type LinkSpot struct {
	Dir  string
	Name string
}

func LauncherLinkSpots() ([]LinkSpot, error) { return nil, nil }

func RefreshLauncherLinks(context.Context, []LinkSpot, string, string) error { return nil }

func SetLinkAppID(string, string, string, func(string) bool) error { return nil }
