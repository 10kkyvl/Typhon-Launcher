//go:build qa && windows

package main

import (
	"errors"
	"strings"
	"testing"
)

func TestQAPort(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    int
		wantErr bool
	}{
		{name: "unset uses the default", want: qaDefaultPort},
		{name: "valid", env: map[string]string{qaPortEnv: "9444"}, want: 9444},
		{name: "lower bound", env: map[string]string{qaPortEnv: "1024"}, want: 1024},
		{name: "upper bound", env: map[string]string{qaPortEnv: "65535"}, want: 65535},
		{name: "below range", env: map[string]string{qaPortEnv: "1023"}, wantErr: true},
		{name: "zero", env: map[string]string{qaPortEnv: "0"}, wantErr: true},
		{name: "above range", env: map[string]string{qaPortEnv: "65536"}, wantErr: true},
		{name: "negative", env: map[string]string{qaPortEnv: "-9333"}, wantErr: true},
		{name: "not a number", env: map[string]string{qaPortEnv: "abc"}, wantErr: true},
		{name: "set but empty", env: map[string]string{qaPortEnv: ""}, wantErr: true},
		{name: "padded", env: map[string]string{qaPortEnv: " 9333"}, wantErr: true},
		{name: "explicit plus", env: map[string]string{qaPortEnv: "+9333"}, wantErr: true},
		{name: "overflows uint16", env: map[string]string{qaPortEnv: "99999999999"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lookup := func(key string) (string, bool) {
				v, ok := tt.env[key]
				return v, ok
			}
			got, err := qaPort(lookup)
			if tt.wantErr {
				if !errors.Is(err, errQAPort) {
					t.Fatalf("qaPort() error = %v, want errQAPort", err)
				}
				if !strings.Contains(err.Error(), qaPortEnv) {
					t.Fatalf("qaPort() error %q does not name %s", err, qaPortEnv)
				}
				return
			}
			if err != nil {
				t.Fatalf("qaPort() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("qaPort() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestQAStartRejectsBadPort(t *testing.T) {
	t.Setenv(qaPortEnv, "80")
	args, err := qaStart()
	if !errors.Is(err, errQAPort) {
		t.Fatalf("qaStart() error = %v, want errQAPort", err)
	}
	if args != nil {
		t.Fatalf("qaStart() args = %v, want none on error", args)
	}
}

func TestQAStartPort(t *testing.T) {
	t.Setenv(qaPortEnv, "9444")
	args, err := qaStart()
	if err != nil {
		t.Fatalf("qaStart() error = %v", err)
	}
	if len(args) != 1 || args[0] != "--remote-debugging-port=9444" {
		t.Fatalf("qaStart() args = %v", args)
	}
}

func TestQAWindowTitle(t *testing.T) {
	if got := windowTitle(); got != "Typhon [qa]" {
		t.Fatalf("windowTitle() = %q", got)
	}
}
