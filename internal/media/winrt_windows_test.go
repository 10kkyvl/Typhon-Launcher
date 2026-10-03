package media

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeAsync struct {
	statuses  []int32
	statusErr error
	code      uint32
	codeErr   error
	polls     int
	cancelled bool
}

func (f *fakeAsync) status() (int32, error) {
	if f.statusErr != nil {
		return 0, f.statusErr
	}
	i := f.polls
	if i >= len(f.statuses) {
		i = len(f.statuses) - 1
	}
	f.polls++
	return f.statuses[i], nil
}

func (f *fakeAsync) errorCode() (uint32, error) { return f.code, f.codeErr }

func (f *fakeAsync) cancel() { f.cancelled = true }

func TestWaitAsync(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name      string
		op        *fakeAsync
		wantErr   func(error) bool
		wantPolls int
	}{
		{
			"completed at once",
			&fakeAsync{statuses: []int32{asyncCompleted}},
			func(err error) bool { return err == nil },
			1,
		},
		{
			"completed after a few polls",
			&fakeAsync{statuses: []int32{0, 0, 0, asyncCompleted}},
			func(err error) bool { return err == nil },
			4,
		},
		{
			"failed reports the HRESULT",
			&fakeAsync{statuses: []int32{0, asyncError}, code: 0x80004005},
			func(err error) bool {
				var h *hresultError
				return errors.As(err, &h) && h.hr == 0x80004005
			},
			2,
		},
		{
			"failed and the code is unreadable",
			&fakeAsync{statuses: []int32{asyncError}, codeErr: boom},
			func(err error) bool { return errors.Is(err, boom) },
			1,
		},
		{
			"canceled by the system",
			&fakeAsync{statuses: []int32{asyncCanceled}},
			func(err error) bool { return err != nil },
			1,
		},
		{
			"status unreadable",
			&fakeAsync{statusErr: boom},
			func(err error) bool { return errors.Is(err, boom) },
			0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := waitAsync(context.Background(), tt.op, time.Microsecond)
			if !tt.wantErr(err) {
				t.Fatalf("waitAsync error = %v", err)
			}
			if tt.op.polls != tt.wantPolls {
				t.Fatalf("polled %d times, want %d", tt.op.polls, tt.wantPolls)
			}
			if tt.op.cancelled {
				t.Fatal("a finished operation was cancelled")
			}
		})
	}
}

func TestWaitAsyncStopsWithTheContext(t *testing.T) {
	op := &fakeAsync{statuses: []int32{0}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := waitAsync(ctx, op, time.Hour)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("waitAsync error = %v, want Canceled", err)
	}
	if !op.cancelled {
		t.Fatal("the pending operation was not cancelled")
	}
}

func TestWaitAsyncStopsAtTheDeadline(t *testing.T) {
	op := &fakeAsync{statuses: []int32{0}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	err := waitAsync(ctx, op, time.Microsecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waitAsync error = %v, want DeadlineExceeded", err)
	}
	if !op.cancelled {
		t.Fatal("the pending operation was not cancelled")
	}
}

func TestHresultError(t *testing.T) {
	err := error(&hresultError{op: "RequestAsync", hr: 0x80070005})
	if got, want := err.Error(), "RequestAsync: HRESULT 0x80070005"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}
