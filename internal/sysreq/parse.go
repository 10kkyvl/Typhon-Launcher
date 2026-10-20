package sysreq

import (
	"regexp"
	"strconv"
	"strings"
)

type Kind string

const (
	KindOS      Kind = "os"
	KindCPU     Kind = "cpu"
	KindRAM     Kind = "ram"
	KindGPU     Kind = "gpu"
	KindVRAM    Kind = "vram"
	KindDirectX Kind = "directx"
	KindStorage Kind = "storage"
)

var kindOrder = []Kind{KindOS, KindCPU, KindRAM, KindGPU, KindVRAM, KindDirectX, KindStorage}

// Row is one "label: value" line of a Steam requirements block.
type Row struct {
	Label string
	Value string
}

// reqSet is the machine-readable content of one requirements block. A zero
// field means the block states nothing about it, never "no requirement met".
type reqSet struct {
	texts map[Kind][]string

	osRank   int
	need64   bool
	cores    int
	cpuTier  int
	clockMHz int
	ram      uint64
	gpuTier  int
	vram     uint64
	dx       int
	storage  uint64

	cpuPartial bool
	gpuPartial bool
	unparsed   map[Kind]bool
}

func (r *reqSet) markUnparsed(kind Kind) {
	if r.unparsed == nil {
		r.unparsed = map[Kind]bool{}
	}
	r.unparsed[kind] = true
}

func (r *reqSet) add(kind Kind, text string) {
	if r.texts == nil {
		r.texts = map[Kind][]string{}
	}
	r.texts[kind] = append(r.texts[kind], text)
}

func (r *reqSet) text(kind Kind) string {
	return strings.Join(r.texts[kind], "; ")
}

func (r *reqSet) has(kind Kind) bool {
	return len(r.texts[kind]) > 0
}

var (
	labelDX      = regexp.MustCompile(`(?i)direct\s*x`)
	labelVRAM    = regexp.MustCompile(`(?i)vram|video\s+memory|видеопамят`)
	labelGPU     = regexp.MustCompile(`(?i)graphics|video\s*card|videocard|\bgpu\b|видеокарт|видеоадаптер|графическ`)
	labelCPU     = regexp.MustCompile(`(?i)processor|\bcpu\b|процессор`)
	labelStorage = regexp.MustCompile(`(?i)storage|hard\s*(?:drive|disk)|disk|available\s+space|free\s+space|место\s+на|жест?к|жёстк|хранилищ|свободное\s+место`)
	labelRAM     = regexp.MustCompile(`(?i)memory|\bram\b|памят|озу`)
	labelOS      = regexp.MustCompile(`(?i)^[^\p{L}\p{N}]*(os|operating\s+system|операционн\p{L}*\s+систем\p{L}*|ос|систем\p{L}*)[^\p{L}\p{N}]*$`)
	valueOS64    = regexp.MustCompile(`(?i)requires\s+a\s+64|64[\s-]*(?:bit|бит|разряд)\w*\s+(?:processor|процессор)|требуется\s+64`)
	labelSplit   = regexp.MustCompile(`^\s*([^:]{2,40}):\s*(.+)$`)
)

func classify(label, value string) Kind {
	label = strings.TrimSpace(label)
	switch {
	case label == "":
	case labelDX.MatchString(label):
		return KindDirectX
	case labelVRAM.MatchString(label):
		return KindVRAM
	case labelGPU.MatchString(label):
		return KindGPU
	case labelCPU.MatchString(label):
		return KindCPU
	case labelStorage.MatchString(label):
		return KindStorage
	case labelRAM.MatchString(label):
		return KindRAM
	case labelOS.MatchString(label):
		return KindOS
	}
	if valueOS64.MatchString(value) {
		return KindOS
	}
	return ""
}

func parseRows(rows []Row) reqSet {
	var r reqSet
	for _, row := range rows {
		label, value := strings.TrimSpace(row.Label), strings.TrimSpace(row.Value)
		if value == "" {
			continue
		}
		kind := classify(label, value)
		if kind == "" && label == "" {
			if m := labelSplit.FindStringSubmatch(value); m != nil {
				if k := classify(m[1], m[2]); k != "" {
					kind, value = k, strings.TrimSpace(m[2])
				}
			}
		}
		if kind != "" {
			r.apply(kind, value)
		}
		if kind != KindDirectX {
			if dx := directXIn(value); dx > 0 && r.dx == 0 {
				r.dx = dx
				r.add(KindDirectX, dxSnippet(value))
			}
		}
	}
	return r
}

func (r *reqSet) apply(kind Kind, value string) {
	switch kind {
	case KindOS:
		rank := minWindowsRank(value)
		need64 := bit64Re.MatchString(value)
		r.add(KindOS, value)
		if rank == 0 && !need64 {
			r.markUnparsed(KindOS)
			return
		}
		r.osRank = max(r.osRank, rank)
		r.need64 = r.need64 || need64
	case KindCPU:
		cores := coreCount(value)
		tier, partial := minTier(value, cpuTier)
		r.add(KindCPU, value)
		if cores == 0 && tier == 0 {
			mhz := clockMHz(value)
			if mhz == 0 {
				r.markUnparsed(KindCPU)
				return
			}
			r.clockMHz = max(r.clockMHz, mhz)
			return
		}
		r.cores = max(r.cores, cores)
		r.cpuTier = max(r.cpuTier, tier)
		r.cpuPartial = r.cpuPartial || partial
	case KindRAM:
		r.add(KindRAM, value)
		b, ok := firstSize(value)
		if !ok {
			r.markUnparsed(KindRAM)
			return
		}
		r.ram = max(r.ram, b)
	case KindStorage:
		r.add(KindStorage, value)
		b, ok := firstSize(value)
		if !ok {
			r.markUnparsed(KindStorage)
			return
		}
		r.storage = max(r.storage, b)
	case KindVRAM:
		r.add(KindVRAM, value)
		b, ok := firstSize(value)
		if !ok {
			r.markUnparsed(KindVRAM)
			return
		}
		r.vram = max(r.vram, b)
	case KindGPU:
		tier, partial := minTier(value, gpuTier)
		vram := gpuVRAM(value)
		switch {
		case tier > 0:
			r.add(KindGPU, value)
			r.gpuTier = max(r.gpuTier, tier)
			r.gpuPartial = r.gpuPartial || partial
		case vram == 0 && directXIn(value) == 0:
			r.add(KindGPU, value)
			r.markUnparsed(KindGPU)
		}
		if vram > 0 {
			r.vram = max(r.vram, vram)
			r.add(KindVRAM, value)
		}
	case KindDirectX:
		r.add(KindDirectX, value)
		dx := directXValue(value)
		if dx == 0 {
			r.markUnparsed(KindDirectX)
			return
		}
		r.dx = max(r.dx, dx)
	}
}

var (
	bit64Re      = regexp.MustCompile(`(?i)64[\s-]*(?:bit|бит|разряд)|\bx64\b|requires\s+a\s+64`)
	windowsRe    = regexp.MustCompile(`(?i)\bwin(?:dows)?\W*(?:®|\(r\)|\(tm\))?\s*(11|10|8\.1|8|7|vista|xp|2000|me|98|95)\b`)
	coreNumRe    = regexp.MustCompile(`(?i)(\d+)[\s-]*(?:cores?\b|ядер\p{L}*|ядра|ядро)`)
	clockRe      = regexp.MustCompile(`(?i)(\d+(?:[.,]\d+)?)\s*(ghz|ггц|mhz|мгц)`)
	coreWordRe   = regexp.MustCompile(`(?i)\b(dual|triple|quad|hexa|six|octa|eight)[\s-]*(?:core)?\b|(двух|трёх|трех|четыр[её]х|шести|восьми)ъ?ядерн`)
	dxPrefixRe   = regexp.MustCompile(`(?i)(?:direct\s*x|\bdx)\s*(?:®|\(r\))?\s*:?\s*(?:version|ver\.?|версии|версия)?\s*:?\s*(\d+)`)
	dxValueRe    = regexp.MustCompile(`\d+`)
	vramPhraseRe = regexp.MustCompile(`(?i)(\d+(?:[.,]\d+)?)\s*(gb|mb|гб|мб)\s*(?:of\s+)?(?:(?:dedicated\s+)?(?:video|graphics)\s+(?:ram|memory)|dedicated\s+(?:ram|memory)|vram|видеопамят|памят)`)
	vramLeadRe   = regexp.MustCompile(`(?i)(?:vram|video\s+memory|видеопамят\w*)\W{0,3}(\d+(?:[.,]\d+)?)\s*(gb|mb|гб|мб)`)
	sysRAMRe     = regexp.MustCompile(`(?i)^\s*(?:of\s+)?(?:system\s+)?(?:ram|озу|оперативн)`)
)

var windowsRanks = map[string]int{
	"95": 1, "98": 2, "me": 3, "2000": 4, "xp": 5, "vista": 6,
	"7": 7, "8": 8, "8.1": 9, "10": 10, "11": 11,
}

func minWindowsRank(text string) int {
	best := 0
	for _, m := range windowsRe.FindAllStringSubmatch(text, -1) {
		if r := windowsRanks[strings.ToLower(m[1])]; r > 0 && (best == 0 || r < best) {
			best = r
		}
	}
	return best
}

func coreCount(text string) int {
	if m := coreNumRe.FindStringSubmatch(text); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n
		}
	}
	if m := coreWordRe.FindStringSubmatch(text); m != nil {
		word := strings.ToLower(m[1] + m[2])
		return map[string]int{
			"dual": 2, "triple": 3, "quad": 4, "hexa": 6, "six": 6, "octa": 8, "eight": 8,
			"двух": 2, "трёх": 3, "трех": 3, "четырёх": 4, "четырех": 4, "шести": 6, "восьми": 8,
		}[word]
	}
	return 0
}

const maxClockMHz = 100000

func clockMHz(text string) int {
	m := clockRe.FindStringSubmatch(text)
	if m == nil {
		return 0
	}
	v, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", "."), 64)
	if err != nil || v <= 0 {
		return 0
	}
	if u := strings.ToLower(m[2]); u == "ghz" || u == "ггц" {
		v *= 1000
	}
	if v > maxClockMHz {
		return 0
	}
	return int(v)
}

func directXIn(text string) int {
	if m := dxPrefixRe.FindStringSubmatch(text); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n
		}
	}
	return 0
}

func dxSnippet(text string) string {
	if loc := dxPrefixRe.FindStringIndex(text); loc != nil {
		return text[loc[0]:loc[1]]
	}
	return text
}

func directXValue(text string) int {
	if dx := directXIn(text); dx > 0 {
		return dx
	}
	if m := dxValueRe.FindString(text); m != "" {
		if n, err := strconv.Atoi(m); err == nil {
			return n
		}
	}
	return 0
}

// gpuVRAM returns the smallest video memory size of a graphics row, or 0
// when any real alternative leaves it unstated: a size printed next to one
// card says nothing about the others, so it cannot become a requirement.
func gpuVRAM(text string) uint64 {
	var best uint64
	for _, alt := range altSplit.Split(text, -1) {
		if strings.TrimSpace(alt) == "" || (gpuTier(alt) == 0 && fillerAlt.MatchString(alt)) {
			continue
		}
		v := altVRAM(alt)
		if v == 0 {
			return 0
		}
		if best == 0 || v < best {
			best = v
		}
	}
	return best
}

func altVRAM(alt string) uint64 {
	if m := vramLeadRe.FindStringSubmatch(alt); m != nil {
		if b, ok := toBytes(m[1], m[2]); ok {
			return b
		}
	}
	if m := vramPhraseRe.FindStringSubmatch(alt); m != nil {
		if b, ok := toBytes(m[1], m[2]); ok {
			return b
		}
	}
	for _, s := range findSizes(alt) {
		if !sysRAMRe.MatchString(alt[s.end:]) {
			return s.bytes
		}
	}
	return 0
}
