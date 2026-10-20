package platform

import (
	"encoding/binary"
	"errors"
	"regexp"
	"strings"
)

var (
	ErrNoGPU          = errors.New("no physical display adapter found")
	ErrGPUUnsupported = errors.New("gpu detection is not available on this platform")
)

// GPUInfo describes the strongest physical display adapter. VRAMKnown is false
// when the driver does not publish the dedicated memory size; VRAMBytes is
// then meaningless.
type GPUInfo struct {
	Name      string
	VRAMBytes uint64
	VRAMKnown bool
}

// adapter is one display-class registry entry before interpretation. Absent
// marks a ghost: the key outlived a removed or unplugged card.
type adapter struct {
	Desc     string
	Absent   bool
	QWord    uint64
	HasQWord bool
	DWord    uint32
	HasDWord bool
}

// dwordCap is what a 32-bit MemorySize holds for any card of 4 GiB or more.
const dwordCap = 0xFFFFFFFF

// virtualAdapterPrefixes are exact vendor names of software adapters; a
// substring like "remote" would also drop real cards with unlucky names.
var virtualAdapterPrefixes = []string{
	"microsoft basic", "microsoft remote display", "microsoft hyper-v video",
	"microsoft render only", "parsec virtual display", "citrix indirect display",
	"vmware svga", "virtualbox graphics", "meta virtual monitor", "spacedesk",
	"virtual display", "idd ",
}

func isVirtualAdapter(name string) bool {
	lower := strings.ToLower(name)
	for _, p := range virtualAdapterPrefixes {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	return false
}

var amdIntegrated = regexp.MustCompile(`(?i)\bradeon\W*(?:tm\W*)?(?:vega\s*\d+\s*)?graphics\b`)

func isIntegratedName(name string) bool {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "intel") && strings.Contains(lower, "arc"):
		return false
	case strings.Contains(lower, "intel"):
		return true
	case amdIntegrated.MatchString(name):
		return true
	}
	return false
}

// decodeLE reads a little-endian 4- or 8-byte registry binary value.
func decodeLE(b []byte) (value uint64, ok bool) {
	switch len(b) {
	case 4:
		return uint64(binary.LittleEndian.Uint32(b)), true
	case 8:
		return binary.LittleEndian.Uint64(b), true
	}
	return 0, false
}

func (a adapter) vram() (uint64, bool) {
	if a.HasQWord && a.QWord > 0 {
		return a.QWord, true
	}
	if a.HasDWord && a.DWord > 0 && a.DWord != dwordCap {
		return uint64(a.DWord), true
	}
	return 0, false
}

func pickGPU(list []adapter) (GPUInfo, error) {
	var best GPUInfo
	found := false
	for _, a := range list {
		name := strings.TrimSpace(a.Desc)
		if a.Absent || name == "" || isVirtualAdapter(name) {
			continue
		}
		vram, known := a.vram()
		cand := GPUInfo{Name: name, VRAMBytes: vram, VRAMKnown: known}
		if !found || betterGPU(cand, best) {
			best, found = cand, true
		}
	}
	if !found {
		return GPUInfo{}, ErrNoGPU
	}
	return best, nil
}

// betterGPU prefers a discrete adapter over an integrated one whatever the
// drivers report, since games run on the strongest card; a published memory
// size and then its amount only break ties.
func betterGPU(a, b GPUInfo) bool {
	if ai, bi := isIntegratedName(a.Name), isIntegratedName(b.Name); ai != bi {
		return bi
	}
	if a.VRAMKnown != b.VRAMKnown {
		return a.VRAMKnown
	}
	return a.VRAMBytes > b.VRAMBytes
}

func detectGPU(read func() ([]adapter, error)) (GPUInfo, error) {
	list, err := read()
	if err != nil {
		return GPUInfo{}, err
	}
	return pickGPU(list)
}

// DetectGPU never leaves the machine: the result feeds only the local
// requirements comparison.
func DetectGPU() (GPUInfo, error) {
	return detectGPU(readAdapters)
}
