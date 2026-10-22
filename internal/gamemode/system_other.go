//go:build !windows

package gamemode

import (
	"time"

	"typhon/internal/uierr"
)

var errUnsupported = uierr.New(CodeUnsupported, "this setting can only be changed on Windows")

type unsupportedPower struct{}

func (unsupportedPower) Active() (string, error)  { return "", errUnsupported }
func (unsupportedPower) Has(string) (bool, error) { return false, errUnsupported }
func (unsupportedPower) Set(string) error         { return errUnsupported }

type unsupportedPriority struct{}

func (unsupportedPriority) Get(uint32) (uint32, error)        { return 0, errUnsupported }
func (unsupportedPriority) Set(uint32, uint32) error          { return errUnsupported }
func (unsupportedPriority) Created(uint32) (time.Time, error) { return time.Time{}, errUnsupported }

func systemPowerImpl() Power       { return unsupportedPower{} }
func systemPriorityImpl() Priority { return unsupportedPriority{} }
