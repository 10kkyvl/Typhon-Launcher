package platform

import (
	"encoding/binary"
	"errors"
	"fmt"
	"runtime"
)

var errNoCoreRecords = errors.New("no processor core records")

// baseSystemInfo carries only what the runtime knows for certain; every field
// that needs the OS is flagged unknown until a platform file fills it in.
func baseSystemInfo() SystemInfo {
	return SystemInfo{
		OS:                   runtime.GOOS,
		Arch:                 runtime.GOARCH,
		CPUUnknown:           true,
		Cores:                runtime.NumCPU(),
		PhysicalCoresUnknown: true,
	}
}

const relationProcessorCore = 0

// countCores counts RelationProcessorCore records in the buffer returned by
// GetLogicalProcessorInformationEx; each one is a physical core.
func countCores(buf []byte) (int, error) {
	cores := 0
	for off := 0; off < len(buf); {
		if len(buf)-off < 8 {
			return 0, fmt.Errorf("processor record at %d is truncated", off)
		}
		relation := binary.LittleEndian.Uint32(buf[off:])
		size := int(binary.LittleEndian.Uint32(buf[off+4:]))
		if size < 8 || off+size > len(buf) {
			return 0, fmt.Errorf("processor record at %d has size %d", off, size)
		}
		if relation == relationProcessorCore {
			cores++
		}
		off += size
	}
	if cores == 0 {
		return 0, errNoCoreRecords
	}
	return cores, nil
}
