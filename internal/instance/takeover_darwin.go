//go:build darwin

package instance

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework Foundation
#include <stdlib.h>
#include "takeover_darwin.h"
*/
import "C"

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"
)

var darwinActive atomic.Pointer[dispatcher]

//export typhonInstanceMessage
func typhonInstanceMessage(raw *C.char) {
	d := darwinActive.Load()
	if d == nil {
		return
	}
	var m message
	if err := json.Unmarshal([]byte(C.GoString(raw)), &m); err != nil {
		return
	}
	d.push(m.Args)
}

// lockPath is where Wails keeps the first-instance lock of id on macOS. It
// asks NSTemporaryDirectory, as Wails does, because $TMPDIR may differ.
var lockPath = func(id string) string {
	dir := C.typhonTempDir()
	defer C.free(unsafe.Pointer(dir))
	return C.GoString(dir) + id + ".lock"
}

type darwinListener struct {
	d    *dispatcher
	file *os.File
	once sync.Once
}

// Takeover makes this process the first instance: it takes the flock Wails
// takes (the old process released it by exiting) and observes the
// notification second launches are sent with. A successor launcher started
// beside the running one by a seamless update calls it after the old process
// has exited; second launches then reach sink.
func Takeover(_ context.Context, id string, sink func(args []string)) (Listener, error) {
	if id == "" {
		return nil, ErrNoID
	}
	file, err := lockFirstInstance(lockPath(id))
	if err != nil {
		return nil, err
	}
	d := newDispatcher(sink)
	if !darwinActive.CompareAndSwap(nil, d) {
		d.close()
		if cerr := file.Close(); cerr != nil {
			return nil, errors.Join(ErrAlreadyListening, cerr)
		}
		return nil, ErrAlreadyListening
	}
	name := C.CString(id)
	defer C.free(unsafe.Pointer(name))
	C.typhonObserve(name)
	return &darwinListener{d: d, file: file}, nil
}

// lockFirstInstance creates the lock file and takes an exclusive lock on it.
func lockFirstInstance(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open the instance lock: %w", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		cerr := file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, errors.Join(ErrNotFirst, cerr)
		}
		return nil, errors.Join(fmt.Errorf("lock the instance lock: %w", err), cerr)
	}
	return file, nil
}

func (l *darwinListener) Close() error {
	var err error
	l.once.Do(func() {
		C.typhonStopObserving()
		darwinActive.Store(nil)
		l.d.close()
		path := l.file.Name()
		err = errors.Join(syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN), l.file.Close())
		if rerr := os.Remove(path); rerr != nil && !errors.Is(rerr, os.ErrNotExist) {
			err = errors.Join(err, rerr)
		}
	})
	return err
}
