//go:build !windows

package crashlog

import "context"

func read(context.Context, Query) ([]Fault, error) { return nil, ErrUnsupported }

func CrashDumpsDir() (string, error) { return "", ErrUnsupported }
