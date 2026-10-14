//go:build windows && !devmock

package library

import (
	"context"
	"errors"
	"testing"
)

func TestElevatingStarterSkipsCreateProcessWhenElevatedRequested(t *testing.T) {
	want := &handleProcess{id: 7}
	var gotReq launch
	start := elevatingStarter(
		func(context.Context, launch) (gameProcess, error) {
			t.Fatal("CreateProcess path used although elevation was requested")
			return nil, nil
		},
		func(req launch) (gameProcess, error) {
			gotReq = req
			return want, nil
		},
	)
	req := launch{executable: `C:\Games\g.exe`, args: []string{"-a"}, workDir: `C:\Games`, elevated: true}

	proc, err := start(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	if proc != want {
		t.Fatalf("proc = %v, want the elevated one", proc)
	}
	if gotReq.executable != req.executable || len(gotReq.args) != 1 {
		t.Fatalf("elevate got %+v", gotReq)
	}
}

func TestElevatingStarterElevatedPropagatesElevateError(t *testing.T) {
	boom := errors.New("declined")
	start := elevatingStarter(
		func(context.Context, launch) (gameProcess, error) {
			t.Fatal("CreateProcess path used although elevation was requested")
			return nil, nil
		},
		func(launch) (gameProcess, error) { return nil, boom },
	)
	if _, err := start(t.Context(), launch{elevated: true}); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the elevate error", err)
	}
}

func TestElevatingStarterNotElevatedUsesPlainStart(t *testing.T) {
	want := &handleProcess{id: 9}
	start := elevatingStarter(
		func(context.Context, launch) (gameProcess, error) { return want, nil },
		func(launch) (gameProcess, error) {
			t.Fatal("elevate used without request")
			return nil, nil
		},
	)
	proc, err := start(t.Context(), launch{})
	if err != nil || proc != want {
		t.Fatalf("proc, err = %v, %v", proc, err)
	}
}
