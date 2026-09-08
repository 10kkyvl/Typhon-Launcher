//go:build darwin && !devmock

package procs

import (
	"context"
	"testing"
)

func TestSupportedOnDarwin(t *testing.T) {
	if !Supported() {
		t.Fatal("Supported() = false, want true on darwin")
	}
}

// List без установленного CrossOver обязан вернуть пустой список, а не
// ошибку: «игр не запущено» и «рантайма нет» для цикла детекта одно и то же,
// а ошибка каждые несколько секунд залила бы лог.
func TestListWithoutRuntimeIsEmpty(t *testing.T) {
	got, complete, err := listWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("listWith(nil): %v", err)
	}
	if !complete {
		t.Fatal("listWith(nil) complete = false, want true: no runtime is a reliable answer of zero processes")
	}
	if len(got) != 0 {
		t.Fatalf("got %+v, want none", got)
	}
}

func TestListCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := List(ctx); err == nil {
		t.Fatal("List with a cancelled context: want error")
	}
}
