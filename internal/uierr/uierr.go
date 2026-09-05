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
