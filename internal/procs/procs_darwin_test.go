//go:build darwin && !devmock

package procs

import (
	"context"
	"errors"
	"os"
	"slices"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestSupportedOnDarwin(t *testing.T) {
	if !Supported() {
		t.Fatal("Supported() = false, want true on darwin")
	}
}

// Без CrossOver список не пуст: в нём остаются нативные процессы, но ни одного
// с путём. «Рантайма нет» не должно ни давать ошибку каждые несколько секунд,
// ни прятать эмулятор, которого ждёт сессия.
func TestListWithoutRuntimeKeepsNativeProcessesWithoutPaths(t *testing.T) {
	started := time.Unix(1_700_000_000, 0)
	native := func() ([]Process, error) {
		return []Process{{PID: 42, PathUnknown: true, CreatedAt: started}}, nil
	}
	got, complete, err := listWith(context.Background(), nil, native)
	if err != nil {
		t.Fatalf("listWith(nil): %v", err)
	}
	if !complete {
		t.Fatal("listWith(nil) complete = false, want true")
	}
	want := []Process{{PID: 42, PathUnknown: true, CreatedAt: started}}
	if !slices.Equal(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestListFailsWhenTheProcessTableFails(t *testing.T) {
	boom := errors.New("boom")
	_, complete, err := listWith(context.Background(), nil, func() ([]Process, error) { return nil, boom })
	if !errors.Is(err, boom) || complete {
		t.Fatalf("listWith = complete %v, err %v; want the error and an incomplete scan", complete, err)
	}
}

// A program the launcher starts outside any bottle — an emulator or a native
// game — must stay visible to the detect loop for as long as it runs.
func TestListSeesTheCurrentNativeProcess(t *testing.T) {
	list, complete, err := List(context.Background())
	if err != nil || !complete {
		t.Fatalf("List = complete %v, err %v", complete, err)
	}
	//nolint:gosec // G115: pid в macOS укладывается в uint32
	self := uint32(os.Getpid())
	for _, p := range list {
		if p.PID != self {
			continue
		}
		if !p.PathUnknown || p.Path != "" || p.CreatedAt.IsZero() || p.CreatedAtUnknown {
			t.Fatalf("own process = %+v, want a start time and no path", p)
		}
		return
	}
	t.Fatalf("own pid %d is missing from %d processes", self, len(list))
}

func TestNativeProcessesSkipZombiesAndRoundTheStart(t *testing.T) {
	row := func(pid int32, stat int8, sec int64, usec int32) unix.KinfoProc {
		var k unix.KinfoProc
		k.Proc.P_pid, k.Proc.P_stat = pid, stat
		k.Proc.P_starttime = unix.Timeval{Sec: sec, Usec: usec}
		return k
	}
	got := nativeProcesses([]unix.KinfoProc{
		row(0, 2, 1, 0),
		row(7, sZomb, 1_700_000_000, 0),
		row(9, 2, 1_700_000_000, 999_999),
	})
	want := []Process{{PID: 9, PathUnknown: true, CreatedAt: time.Unix(1_700_000_000, 0)}}
	if !slices.Equal(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestListCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := List(ctx); err == nil {
		t.Fatal("List with a cancelled context: want error")
	}
}
