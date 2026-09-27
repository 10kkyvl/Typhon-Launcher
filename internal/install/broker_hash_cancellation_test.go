package install

import (
	"context"
	"errors"
	"os"
	"testing"
)

type cancelHashReader struct {
	cancel context.CancelFunc
	reads  int
}

func (r *cancelHashReader) Read(p []byte) (int, error) {
	r.reads++
	r.cancel()
	return len(p), nil
}

func TestBrokerHashStopsBetweenReads(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	src := &cancelHashReader{cancel: cancel}
	digest, err := hashBrokerInstaller(ctx, src)
	if !errors.Is(err, context.Canceled) || digest != "" || src.reads != 1 {
		t.Fatalf("hash = %q, err = %v, reads = %d", digest, err, src.reads)
	}
}

func TestCanceledBrokerHashDoesNotPublishSpec(t *testing.T) {
	dir, pin := brokerDirs(t)
	ws := goodSpec(pin)
	if err := os.WriteFile(ws.InstallerPath, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, _, err := handOffToWorker(ctx, runSpec{
		Broker: &brokerHandoff{Key: brokerTestPrivate, Dir: dir, Gone: make(chan struct{})},
	}, ws, "")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("handoff = %v, want canceled", err)
	}
	if _, err := os.Stat(brokerSpecPath(dir)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canceled hash published broker spec: %v", err)
	}
}
