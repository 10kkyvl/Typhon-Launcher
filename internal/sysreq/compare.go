package sysreq

import (
	"regexp"
	"strconv"
	"strings"
)

type Status string

const (
	StatusOK               Status = "ok"
	StatusBelowRecommended Status = "below_recommended"
	StatusBelowMinimum     Status = "below_minimum"
	StatusUnknown          Status = "unknown"
)

type Verdict string

const (
	VerdictGood      Verdict = "good"
	VerdictMinimum   Verdict = "minimum"
	VerdictNotEnough Verdict = "not_enough"
	VerdictUnknown   Verdict = "unknown"
)

const (
	ReasonNoRequirements = "no_requirements"
	ReasonNotComparable  = "not_comparable"
	ReasonPartial        = "partial"
)

// ramSlack absorbs the gap between a marketed size and what the OS reports:
// an 8 GB machine shows 7.9 GB, and requirement texts round to the label.
const ramSlack = 1.1

// Check is one compared requirement. Required and Recommended are the
// original Steam texts; either may be empty when the block omits it.
type Check struct {
	Kind        Kind   `json:"kind"`
	Required    string `json:"required"`
	Recommended string `json:"recommended"`
	Have        Have   `json:"have"`
	Status      Status `json:"status"`
}

// Have is this machine's side of a check as raw values, so the UI words it in
// its own language. Zero values mean unknown; Free is set only when the free
// space is known, since a full disk has zero bytes free.
type Have struct {
	Text    string  `json:"text"`
	Cores   int     `json:"cores"`
	Bits    int     `json:"bits"`
	Bytes   uint64  `json:"bytes"`
	Free    *uint64 `json:"free"`
	DirectX int     `json:"directX"`
	// Installed marks a storage check of a game that already sits on the disk:
	// free space is irrelevant once the files are in place.
	Installed bool `json:"installed"`
}

type Report struct {
	Verdict Verdict `json:"verdict"`
	Reason  string  `json:"reason"`
	Checks  []Check `json:"checks"`
}

// Hardware is the local machine. Zero values mean "unknown", except FreeKnown
// which guards FreeBytes because a full disk legitimately has zero free bytes.
type Hardware struct {
	OSName        string
	WinRank       int
	Arch          string
	CPUName       string
	PhysicalCores int
	RAMBytes      uint64
	GPUName       string
	VRAMBytes     uint64
	FreeBytes     uint64
	FreeKnown     bool
	Installed     bool
}

type tri int

const (
	triNone tri = iota
	triUnknown
	triMet
	triUnmet
)

func combine(parts ...tri) tri {
	out := triNone
	for _, p := range parts {
		switch {
		case p == triUnmet:
			return triUnmet
		case p == triUnknown:
			out = triUnknown
		case p == triMet && out == triNone:
			out = triMet
		}
	}
	return out
}

func atLeast(have, need uint64) tri {
	if need == 0 {
		return triNone
	}
	if have == 0 {
		return triUnknown
	}
	if float64(have)*ramSlack >= float64(need) {
		return triMet
	}
	return triUnmet
}

func tierAtLeast(haveName string, haveTier func(string) int, need int, partial bool) tri {
	if need == 0 {
		return triNone
	}
	t := 0
	if haveName != "" {
		t = haveTier(haveName)
	}
	if t == 0 {
		return triUnknown
	}
	if t >= need {
		return triMet
	}
	if partial {
		return triUnknown
	}
	return triUnmet
}

func (h Hardware) is64() (bool, bool) {
	switch h.Arch {
	case "amd64", "arm64":
		return true, true
	case "386", "arm":
		return false, true
	}
	return false, false
}

// eval: a requirement row that was recognised but could not be read keeps the
// check unknown, unless another row of the same kind is already unmet.
func (h Hardware) eval(kind Kind, r *reqSet) tri {
	res := h.evalFacts(kind, r)
	if res != triUnmet && r.unparsed[kind] {
		return triUnknown
	}
	return res
}

func (h Hardware) evalFacts(kind Kind, r *reqSet) tri {
	switch kind {
	case KindOS:
		parts := []tri{triNone, triNone}
		if r.osRank > 0 {
			parts[0] = triUnknown
			if h.WinRank > 0 {
				parts[0] = triUnmet
				if h.WinRank >= r.osRank {
					parts[0] = triMet
				}
			}
		}
		if r.need64 {
			parts[1] = triUnknown
			if is64, known := h.is64(); known {
				parts[1] = triUnmet
				if is64 {
					parts[1] = triMet
				}
			}
		}
		return combine(parts...)
	case KindCPU:
		cores := triNone
		if r.cores > 0 {
			cores = triUnknown
			if h.PhysicalCores > 0 {
				cores = triUnmet
				if h.PhysicalCores >= r.cores {
					cores = triMet
				}
			}
		}
		return combine(tierAtLeast(h.CPUName, cpuTier, r.cpuTier, r.cpuPartial), cores, h.clockOnly(r.clockMHz))
	case KindRAM:
		return atLeast(h.RAMBytes, r.ram)
	case KindGPU:
		return tierAtLeast(h.GPUName, gpuTier, r.gpuTier, r.gpuPartial)
	case KindVRAM:
		return atLeast(h.VRAMBytes, r.vram)
	case KindDirectX:
		if r.dx == 0 {
			return triNone
		}
		have := h.directX()
		switch {
		case have == 0:
			return triUnknown
		case have >= r.dx:
			return triMet
		default:
			return triUnmet
		}
	case KindStorage:
		if r.storage == 0 {
			return triNone
		}
		if h.Installed {
			return triMet
		}
		if !h.FreeKnown {
			return triUnknown
		}
		if h.FreeBytes >= r.storage {
			return triMet
		}
		return triUnmet
	}
	return triNone
}

// A requirement that names only a clock speed comes from games older than any
// current CPU family; per-core speed has outgrown clock since, so a part from
// that tier onwards meets it. Faster clocks or an unknown part stay unknown.
const (
	clockOnlyTier   = 6 * 4
	clockOnlyMaxMHz = 4000
)

func (h Hardware) clockOnly(needMHz int) tri {
	if needMHz == 0 {
		return triNone
	}
	if h.CPUName != "" && cpuTier(h.CPUName) >= clockOnlyTier && needMHz <= clockOnlyMaxMHz {
		return triMet
	}
	return triUnknown
}

// directX is the newest DirectX the installed Windows ships, not what the
// video card supports; 0 when the Windows version is unknown.
func (h Hardware) directX() int {
	switch {
	case h.WinRank >= 10:
		return 12
	case h.WinRank >= 7:
		return 11
	case h.WinRank == 6:
		return 10
	case h.WinRank >= 1:
		return 9
	}
	return 0
}

func (h Hardware) have(kind Kind) Have {
	switch kind {
	case KindOS:
		if h.OSName == "" {
			return Have{}
		}
		out := Have{Text: h.OSName}
		if is64, known := h.is64(); known {
			out.Bits = 32
			if is64 {
				out.Bits = 64
			}
		}
		return out
	case KindCPU:
		if h.CPUName == "" {
			return Have{}
		}
		return Have{Text: h.CPUName, Cores: h.PhysicalCores}
	case KindRAM:
		return Have{Bytes: h.RAMBytes}
	case KindGPU:
		return Have{Text: h.GPUName}
	case KindVRAM:
		return Have{Bytes: h.VRAMBytes}
	case KindDirectX:
		return Have{DirectX: h.directX()}
	case KindStorage:
		if h.Installed {
			return Have{Installed: true}
		}
		if h.FreeKnown {
			free := h.FreeBytes
			return Have{Free: &free}
		}
	}
	return Have{}
}

func hasRows(rows []Row) bool {
	for _, r := range rows {
		if strings.TrimSpace(r.Value) != "" {
			return true
		}
	}
	return false
}

func compare(h Hardware, minRows, recRows []Row) Report {
	if !hasRows(minRows) && !hasRows(recRows) {
		return Report{Verdict: VerdictUnknown, Reason: ReasonNoRequirements, Checks: []Check{}}
	}
	minSet, recSet := parseRows(minRows), parseRows(recRows)
	checks := []Check{}
	for _, kind := range kindOrder {
		if !minSet.has(kind) && !recSet.has(kind) {
			continue
		}
		checks = append(checks, Check{
			Kind:        kind,
			Required:    minSet.text(kind),
			Recommended: recSet.text(kind),
			Have:        h.have(kind),
			Status:      status(h.eval(kind, &minSet), h.eval(kind, &recSet)),
		})
	}
	verdict, reason := verdictOf(checks)
	return Report{Verdict: verdict, Reason: reason, Checks: checks}
}

func status(min, rec tri) Status {
	switch {
	case min == triUnmet:
		return StatusBelowMinimum
	case rec == triUnmet:
		return StatusBelowRecommended
	case min == triUnknown || rec == triUnknown:
		return StatusUnknown
	}
	return StatusOK
}

// verdictOf: good needs every check known; one unknown check could hide a
// shortfall, so without a proven shortfall the verdict stays unknown.
func verdictOf(checks []Check) (Verdict, string) {
	known, unknown, belowRec := 0, 0, false
	for _, c := range checks {
		switch c.Status {
		case StatusBelowMinimum:
			return VerdictNotEnough, ""
		case StatusBelowRecommended:
			belowRec = true
			known++
		case StatusOK:
			known++
		default:
			unknown++
		}
	}
	switch {
	case known == 0:
		return VerdictUnknown, ReasonNotComparable
	case unknown > 0:
		return VerdictUnknown, ReasonPartial
	case belowRec:
		return VerdictMinimum, ""
	}
	return VerdictGood, ""
}

var winBuildRe = regexp.MustCompile(`(?i)\bwindows\s+(11|10|8\.1|8|7|vista|xp)\b`)
var buildNumRe = regexp.MustCompile(`(?i)build\s+(\d+)`)

// windowsRank reads the rank from the OS string platform produces
// ("Windows 10 Pro (build 22631)"). Windows 11 still reports ProductName
// "Windows 10", so the build number decides; without a readable build the
// rank is unknown. 0 means not a known Windows.
func windowsRank(osName string) int {
	m := winBuildRe.FindStringSubmatch(osName)
	if m == nil {
		return 0
	}
	rank := windowsRanks[strings.ToLower(m[1])]
	if rank == 10 {
		b := buildNumRe.FindStringSubmatch(osName)
		if b == nil {
			return 0
		}
		n, err := strconv.Atoi(b[1])
		if err != nil {
			return 0
		}
		if n >= 22000 {
			return 11
		}
	}
	return rank
}
