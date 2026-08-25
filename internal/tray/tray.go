package tray

import (
	"errors"
	"fmt"
	"sync"
)

type Tray interface {
	Destroy()
}

type Window interface {
	Show()
	Hide()
	Focus()
}

type Controller struct {
	window Window
	create func() (Tray, error)
	quit   func()

	mu   sync.Mutex
	tray Tray
}

func New(window Window, create func() (Tray, error), quit func()) (*Controller, error) {
	if window == nil {
		return nil, errors.New("tray: window is required")
	}
	if create == nil {
		return nil, errors.New("tray: create is required")
	}
	if quit == nil {
		return nil, errors.New("tray: quit is required")
	}
	return &Controller{window: window, create: create, quit: quit}, nil
}

func (c *Controller) Apply(enabled bool) error {
	if enabled {
		return c.enable()
	}
	c.dropTray()
	return nil
}

func (c *Controller) enable() error {
	c.mu.Lock()
	if c.tray != nil {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	t, err := c.create()
	if err != nil {
		return fmt.Errorf("create tray: %w", err)
	}
	if t == nil {
		return errors.New("tray: create returned no tray")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tray != nil {
		t.Destroy()
		return nil
	}
	c.tray = t
	return nil
}

func (c *Controller) dropTray() {
	c.mu.Lock()
	t := c.tray
	c.tray = nil
	c.mu.Unlock()

	if t != nil {
		t.Destroy()
	}
}

// Without a tray the window is left to close the way it always did: the caller
// does not cancel the event, and the toolkit quits on the last closed window.
// Quitting from here on top of that runs shutdown twice and re-emits the very
// event being handled.
func (c *Controller) CloseRequested() bool {
	c.mu.Lock()
	t := c.tray
	c.mu.Unlock()

	if t == nil {
		return false
	}
	c.window.Hide()
	return true
}

func (c *Controller) Open() {
	c.window.Show()
	c.window.Focus()
}

// The tray goes first: while it is alive CloseRequested cancels the close that
// shutdown issues, and the window would be hidden instead of closed.
func (c *Controller) Quit() {
	c.dropTray()
	c.quit()
}
