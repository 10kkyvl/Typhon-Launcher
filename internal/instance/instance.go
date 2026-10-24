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

	earlyPrefix = "typhon-"
	earlySuffix = "-early"

	deadline  = 30 * time.Second
	pollEvery = 50 * time.Millisecond
	sendBlock = 5 * time.Second
)

var (
	ErrNoID        = errors.New("single instance id is empty")
	ErrNotTaken    = errors.New("the running instance did not take the launch in time")
	ErrSendTimeout = errors.New("the running instance did not process the message in time")
)

type message struct {
	Args       []string `json:"args"`
	WorkingDir string   `json:"workingDir"`
}

type api interface {
	// Claim reports whether this process created the named mutex first and, if so,
	// keeps it until the process exits.
	Claim(name string) (bool, error)
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

// The Wails mutex appears only in application.New, after recovery and every
// service, so the first instance claims its own earlier one: whoever creates it
// first is the first instance, everyone else waits for its window.
func forward(ctx context.Context, sys api, wait waiter, id string, args []string, workDir string) (bool, error) {
	if id == "" {
		return false, ErrNoID
	}
	prefix := namePrefix + id
	first, err := sys.Claim(earlyPrefix + id + earlySuffix)
	if err != nil {
		return false, fmt.Errorf("claim the first instance: %w", err)
	}
	if first {
		// A launcher from before this hand-off existed holds only the Wails mutex.
		running, err := sys.MutexExists(prefix + mutexSuffix)
		if err != nil {
			return false, fmt.Errorf("check for a running instance: %w", err)
		}
		if !running {
			return false, nil
		}
	}
	payload, err := json.Marshal(message{Args: args, WorkingDir: workDir})
	if err != nil {
		return false, fmt.Errorf("encode launch data: %w", err)
	}
	for waited := time.Duration(0); waited < deadline; {
		window, err := sys.FindWindow(prefix+classSuffix, prefix+windowSuffix)
		if err != nil {
			return false, fmt.Errorf("find the running instance window: %w", err)
		}
		if window != 0 {
			err := sys.Send(window, copyDataID, string(payload))
			if err == nil {
				return true, nil
			}
			// Windows drops a message the target has not picked up, and the main
			// thread of the first instance pumps nothing until its services have
			// started, so a timeout means "not delivered" and is safe to repeat.
			if !errors.Is(err, ErrSendTimeout) {
				return false, fmt.Errorf("send launch data to the running instance: %w", err)
			}
			waited += sendBlock
			continue
		}
		if err := wait(ctx, pollEvery); err != nil {
			return false, fmt.Errorf("wait for the running instance window: %w", err)
		}
		waited += pollEvery
	}
	return false, ErrNotTaken
}
