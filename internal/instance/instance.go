package instance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Wails v3 beta.17 owns this protocol and exposes no way to run its single
// instance check before application.New, which is built after every service.
// A second launch would open the state files first, so the hand-off is
// repeated here byte for byte; the names and the id live in one place and
// TestProtocolMatchesWails fails when a Wails update changes them.
const (
	namePrefix   = "wails-app-"
	mutexSuffix  = "-sim"
	classSuffix  = "-sic"
	windowSuffix = "-siw"
	copyDataID   = 1542

	windowWait = 2 * time.Second
	pollEvery  = 50 * time.Millisecond
)

var ErrNoID = errors.New("single instance id is empty")

type message struct {
	Args       []string `json:"args"`
	WorkingDir string   `json:"workingDir"`
}

type api interface {
	MutexExists(name string) (bool, error)
	FindWindow(class, title string) (uintptr, error)
	Send(window uintptr, dataID uintptr, payload string) error
}

type waiter func(ctx context.Context, d time.Duration) error

func Forward(ctx context.Context, id string, args []string, workDir string) (bool, error) {
	system := systemAPI()
	if system == nil {
		return false, nil
	}
	return forward(ctx, system, sleep, id, args, workDir)
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func forward(ctx context.Context, sys api, wait waiter, id string, args []string, workDir string) (bool, error) {
	if id == "" {
		return false, ErrNoID
	}
	prefix := namePrefix + id
	held, err := sys.MutexExists(prefix + mutexSuffix)
	if err != nil {
		return false, fmt.Errorf("check for a running instance: %w", err)
	}
	if !held {
		return false, nil
	}
	payload, err := json.Marshal(message{Args: args, WorkingDir: workDir})
	if err != nil {
		return false, fmt.Errorf("encode launch data: %w", err)
	}
	for waited := time.Duration(0); ; waited += pollEvery {
		window, err := sys.FindWindow(prefix+classSuffix, prefix+windowSuffix)
		if err != nil {
			return false, fmt.Errorf("find the running instance window: %w", err)
		}
		if window != 0 {
			if err := sys.Send(window, copyDataID, string(payload)); err != nil {
				return false, fmt.Errorf("send launch data to the running instance: %w", err)
			}
			return true, nil
		}
		if waited >= windowWait {
			return false, nil
		}
		if err := wait(ctx, pollEvery); err != nil {
			return false, fmt.Errorf("wait for the running instance window: %w", err)
		}
	}
}
