//go:build windows

package selfupdate

import (
	"fmt"
	"os"
	"testing"
)

func TestHoldInstanceLockAdmitsOnlyTheFirstHolder(t *testing.T) {
	name := fmt.Sprintf("typhon-start-guard-test-%d-%s", os.Getpid(), t.Name())

	first, err := holdInstanceLock(name)
	if err != nil || !first {
		t.Fatalf("first holder = %v, %v; want it to get the lock", first, err)
	}
	second, err := holdInstanceLock(name)
	if err != nil || second {
		t.Fatalf("second holder = %v, %v; want it refused: another launcher has the lock", second, err)
	}

	other, err := holdInstanceLock(name + "-other-data-dir")
	if err != nil || !other {
		t.Fatalf("a different name = %v, %v; want it free", other, err)
	}
}

func TestHoldInstanceLockRejectsANameWithNul(t *testing.T) {
	if _, err := holdInstanceLock("typhon\x00guard"); err == nil {
		t.Fatal("holdInstanceLock() error = nil for a name with a NUL")
	}
}
