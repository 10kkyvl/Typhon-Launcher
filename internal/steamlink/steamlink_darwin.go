//go:build darwin && !devmock

package steamlink

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// steamProcess is the name of the Mac Steam client in the process table. Its
// helpers (Steam Helper, ipcserver) live and die with it.
const steamProcess = "steam_osx"

// sZomb is SZOMB from <sys/proc.h>: an exited process nobody has reaped yet.
const sZomb = 5

func Supported() bool { return true }

// Root returns the folder where the Mac Steam keeps userdata, or
// ErrNotInstalled when the client has never run for this user.
func Root() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("домашняя папка: %w", err)
	}
	return rootIn(home)
}

func rootIn(home string) (string, error) {
	root := filepath.Join(home, "Library", "Application Support", "Steam")
	info, err := os.Stat(root)
	if errors.Is(err, fs.ErrNotExist) {
		return "", ErrNotInstalled
	}
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", root, err)
	}
	if !info.IsDir() {
		return "", ErrNotInstalled
	}
	return root, nil
}

// steamRunning looks for the client in the process table. The pid Steam writes
// to registry.vdf would do as well, but it stays behind after a crash and can
// later name an unrelated process; the name of a live process cannot.
func steamRunning(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	kinfos, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return false, fmt.Errorf("таблица процессов: %w", err)
	}
	//nolint:gosec // G115: a uid is a uid_t, an unsigned 32-bit value that os.Getuid returns as int
	return steamAmong(kinfos, uint32(os.Getuid())), nil
}

// steamAmong only counts a client of this user: another account's Steam keeps
// its shortcuts in its own home and never overwrites ours.
func steamAmong(kinfos []unix.KinfoProc, uid uint32) bool {
	for _, k := range kinfos {
		if k.Proc.P_pid <= 0 || k.Proc.P_stat == sZomb || k.Eproc.Ucred.Uid != uid {
			continue
		}
		if unix.ByteSliceToString(k.Proc.P_comm[:]) == steamProcess {
			return true
		}
	}
	return false
}
