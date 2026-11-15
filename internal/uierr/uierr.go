// Package uierr attaches a stable code to the errors that reach the user
// interface. Wails transports an error to the frontend as its message text, so
// the code travels inside that text: matching russian substrings, which is what
// the frontend used to do, breaks silently the moment a message is reworded.
package uierr

import (
	"errors"
	"fmt"
)

const prefix = "typhon:"

type Error struct {
	code string
	err  error
}

func New(code, detail string) error {
	return &Error{code: code, err: errors.New(detail)}
}

func Wrap(code string, err error) error {
	return &Error{code: code, err: err}
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s%s: %s", prefix, e.code, e.err.Error())
}

func (e *Error) Code() string {
	return e.code
}

func (e *Error) Unwrap() error {
	return e.err
}

func Code(err error) string {
	var coded *Error
	if errors.As(err, &coded) {
		return coded.code
	}
	return ""
}

// Codes lists every code of the chain in the order errors.As walks it, so
// the first one is what Code returns. A caller that knows an outer code only
// asks for something it does not need reads the cause behind it.
func Codes(err error) []string {
	var out []string
	var walk func(error)
	walk = func(err error) {
		if err == nil {
			return
		}
		// Each link is looked at by itself; errors.As would jump to the first
		// coded one and lose the order of the rest.
		if coded, ok := err.(*Error); ok { //nolint:errorlint // walks the chain one link at a time
			out = append(out, coded.code)
		}
		switch u := err.(type) { //nolint:errorlint // walks the chain one link at a time
		case interface{ Unwrap() error }:
			walk(u.Unwrap())
		case interface{ Unwrap() []error }:
			for _, e := range u.Unwrap() {
				walk(e)
			}
		}
	}
	walk(err)
	return out
}
