//go:build !windows

package steamlink

import (
	"context"
	"strings"
)

func Supported() bool { return false }

func Root() (string, error) { return "", ErrUnsupported }

func steamRunning(context.Context) (bool, error) { return false, ErrUnsupported }

func JoinArgs(args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = a
		if a == "" || strings.ContainsAny(a, " \t\"") {
			parts[i] = `"` + strings.ReplaceAll(a, `"`, `\"`) + `"`
		}
	}
	return strings.Join(parts, " ")
}
