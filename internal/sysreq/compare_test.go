package sysreq

import (
	"encoding/json"
	"testing"
)

func rig() Hardware {
	return Hardware{
		OSName:        "Windows 10 Pro (build 19045)",
		WinRank:       10,
		Arch:          "amd64",
		CPUName:       "Intel(R) Core(TM) i5-8400 CPU @ 2.80GHz",
		PhysicalCores: 6,
		RAMBytes:      16 * gb,
		GPUName:       "NVIDIA GeForce GTX 1060 6GB",
		VRAMBytes:     6 * gb,
		FreeBytes:     200 * gb,
		FreeKnown:     true,
	}
}

func rows(pairs ...string) []Row {
	out := make([]Row, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, Row{Label: pairs[i], Value: pairs[i+1]})
	}
	return out
}

func statusOf(t *testing.T, r Report, kind Kind) Status {
	t.Helper()
	for _, c := range r.Checks {
		if c.Kind == kind {
			return c.Status
		}
	}
	t.Fatalf("no %s check in %+v", kind, r.Checks)
	return ""
}

func TestCompareVerdicts(t *testing.T) {
	tests := []struct {
		name    string
		hw      func(*Hardware)
		min     []Row
		rec     []Row
		verdict Verdict
	}{
		{"meets recommended", nil,
			rows("Memory", "8 GB RAM", "Graphics", "GTX 960"),
			rows("Memory", "16 GB RAM", "Graphics", "GTX 1060"), VerdictGood},
		{"minimum only meets it", nil,
			rows("Memory", "8 GB RAM", "Storage", "50 GB available space"), nil, VerdictGood},
		{"below recommended", func(h *Hardware) { h.RAMBytes = 8 * gb },
			rows("Memory", "4 GB RAM"), rows("Memory", "16 GB RAM"), VerdictMinimum},
		{"below minimum ram", func(h *Hardware) { h.RAMBytes = 2 * gb },
			rows("Memory", "4 GB RAM"), rows("Memory", "8 GB RAM"), VerdictNotEnough},
		{"below minimum gpu", func(h *Hardware) { h.GPUName = "Intel(R) HD Graphics 4000" },
			rows("Graphics", "GeForce GTX 660"), nil, VerdictNotEnough},
		{"below minimum vram", func(h *Hardware) { h.VRAMBytes = 1 * gb },
			rows("Graphics", "NVIDIA GeForce GTX 1060 6GB"), nil, VerdictNotEnough},
		{"disk too small", func(h *Hardware) { h.FreeBytes = 10 * gb },
			rows("Storage", "50 GB available space"), nil, VerdictNotEnough},
		{"disk exactly enough", func(h *Hardware) { h.FreeBytes = 50 * gb },
			rows("Storage", "50 GB available space"), nil, VerdictGood},
		{"old windows", func(h *Hardware) { h.WinRank = 7; h.OSName = "Windows 7" },
			rows("OS", "Windows 10 64-bit"), nil, VerdictNotEnough},
		{"32-bit machine", func(h *Hardware) { h.Arch = "386" },
			rows("", "Requires a 64-bit processor and operating system"), nil, VerdictNotEnough},
		{"directx too new for windows 7", func(h *Hardware) { h.WinRank = 7 },
			rows("DirectX", "Version 12"), nil, VerdictNotEnough},
		{"cpu cores below", func(h *Hardware) { h.PhysicalCores = 2 },
			rows("Processor", "Quad core"), nil, VerdictNotEnough},
		{"cpu tier below recommended", func(h *Hardware) { h.CPUName = "Intel(R) Core(TM) i3-4130" },
			rows("Processor", "Intel Core i3-4130"), rows("Processor", "Intel Core i7-8700K"), VerdictMinimum},
		{"one unmet outweighs ok", func(h *Hardware) { h.FreeBytes = 1 * gb },
			rows("Memory", "4 GB RAM", "Storage", "50 GB"), nil, VerdictNotEnough},
		{"unknown check keeps good out", func(h *Hardware) { h.GPUName = "" },
			rows("Memory", "8 GB RAM", "Graphics", "GTX 960"), nil, VerdictUnknown},
		{"unrated requirement keeps good out", nil,
			rows("Memory", "8 GB RAM", "Graphics", "GeForce GTX 1630 or RX 6400"), nil, VerdictUnknown},
		{"known shortfall still wins over unknown", func(h *Hardware) { h.RAMBytes = 2 * gb },
			rows("Memory", "8 GB RAM", "Graphics", "GeForce GTX 1630 or RX 6400"), nil, VerdictNotEnough},
		{"intel 12th gen u-series meets old minimum", func(h *Hardware) { h.CPUName = "12th Gen Intel(R) Core(TM) i5-1235U" },
			rows("Processor", "Intel Core i5-6600"), nil, VerdictGood},
		{"partly rated alternatives below tier stay unknown", func(h *Hardware) { h.GPUName = "NVIDIA GeForce GTX 750" },
			rows("Graphics", "GTX 1630 or GTX 1060"), nil, VerdictUnknown},
		{"all unknown", func(h *Hardware) { *h = Hardware{} },
			rows("Memory", "8 GB RAM", "Graphics", "GTX 960"), nil, VerdictUnknown},
		{"ram rounding slack", func(h *Hardware) { h.RAMBytes = 8*gb - 300*mib },
			rows("Memory", "8 GB RAM"), nil, VerdictGood},
		{"russian rows", nil,
			rows("Оперативная память", "8 GB ОЗУ", "Видеокарта", "GeForce GTX 960", "Место на диске", "50 GB"), nil, VerdictGood},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hw := rig()
			if tc.hw != nil {
				tc.hw(&hw)
			}
			got := compare(hw, tc.min, tc.rec)
			if got.Verdict != tc.verdict {
				t.Fatalf("verdict = %s, want %s; checks %+v", got.Verdict, tc.verdict, got.Checks)
			}
			if got.Verdict == VerdictUnknown && got.Reason == "" {
				t.Fatal("unknown verdict without reason")
			}
			if got.Verdict != VerdictUnknown && got.Reason != "" {
				t.Fatalf("reason %q on known verdict", got.Reason)
			}
		})
	}
}

func TestCompareUnknownIsNeverPass(t *testing.T) {
	tests := []struct {
		name string
		hw   func(*Hardware)
		min  []Row
		kind Kind
	}{
		{"no ram", func(h *Hardware) { h.RAMBytes = 0 }, rows("Memory", "8 GB RAM"), KindRAM},
		{"no gpu name", func(h *Hardware) { h.GPUName = "" }, rows("Graphics", "GTX 960"), KindGPU},
		{"unrated gpu", func(h *Hardware) { h.GPUName = "Mystery Adapter 9000" }, rows("Graphics", "GTX 960"), KindGPU},
		{"no vram", func(h *Hardware) { h.VRAMBytes = 0 }, rows("Video Memory", "6 GB"), KindVRAM},
		{"no cpu name", func(h *Hardware) { h.CPUName = "" }, rows("Processor", "Intel Core i5-4460"), KindCPU},
		{"no physical cores", func(h *Hardware) { h.PhysicalCores = 0 }, rows("Processor", "Quad core"), KindCPU},
		{"no free space", func(h *Hardware) { h.FreeKnown = false }, rows("Storage", "50 GB"), KindStorage},
		{"unknown windows", func(h *Hardware) { h.WinRank = 0 }, rows("OS", "Windows 10"), KindOS},
		{"unknown arch", func(h *Hardware) { h.Arch = "riscv64" }, rows("", "Requires a 64-bit processor and operating system"), KindOS},
		{"unknown directx", func(h *Hardware) { h.WinRank = 0 }, rows("DirectX", "Version 11"), KindDirectX},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hw := rig()
			tc.hw(&hw)
			r := compare(hw, tc.min, nil)
			if got := statusOf(t, r, tc.kind); got != StatusUnknown {
				t.Fatalf("status = %s, want unknown", got)
			}
			if r.Verdict != VerdictUnknown {
				t.Fatalf("verdict = %s, want unknown", r.Verdict)
			}
			if r.Reason != ReasonNotComparable {
				t.Fatalf("reason = %q", r.Reason)
			}
		})
	}
}

func TestCompareNoRequirements(t *testing.T) {
	for name, in := range map[string][]Row{
		"nil":        nil,
		"blank rows": rows("Memory", "  ", "Graphics", ""),
	} {
		t.Run(name, func(t *testing.T) {
			r := compare(rig(), in, nil)
			if r.Verdict != VerdictUnknown || r.Reason != ReasonNoRequirements {
				t.Fatalf("got %+v", r)
			}
			if r.Checks == nil || len(r.Checks) != 0 {
				t.Fatalf("checks = %#v, want empty non-nil", r.Checks)
			}
		})
	}
}

func TestCompareRowsNothingParsed(t *testing.T) {
	t.Run("unrecognised rows only", func(t *testing.T) {
		r := compare(rig(), rows("Additional Notes", "Headphones recommended"), nil)
		if r.Verdict != VerdictUnknown || r.Reason != ReasonNotComparable || len(r.Checks) != 0 {
			t.Fatalf("got %+v", r)
		}
	})
	t.Run("recognised but unreadable rows", func(t *testing.T) {
		r := compare(rig(),
			rows("Processor", "Any modern processor", "Graphics", "GeForce GTX 1630 or RX 6400", "Memory", "Plenty of RAM", "Storage", "SSD"), nil)
		if r.Verdict != VerdictUnknown || r.Reason != ReasonNotComparable {
			t.Fatalf("got %+v", r)
		}
		if len(r.Checks) != 4 {
			t.Fatalf("checks = %+v", r.Checks)
		}
		for _, c := range r.Checks {
			if c.Status != StatusUnknown || c.Required == "" {
				t.Fatalf("check = %+v", c)
			}
		}
	})
}

func TestCompareUnparsedRowIsPartial(t *testing.T) {
	r := compare(rig(), rows("Memory", "8 GB RAM", "Graphics", "GeForce GTX 1630 or RX 6400"), nil)
	if r.Verdict != VerdictUnknown || r.Reason != ReasonPartial {
		t.Fatalf("got %s/%s", r.Verdict, r.Reason)
	}
	if got := statusOf(t, r, KindRAM); got != StatusOK {
		t.Fatalf("ram = %s", got)
	}
	gpu := r.Checks[1]
	if gpu.Kind != KindGPU || gpu.Status != StatusUnknown || gpu.Required != "GeForce GTX 1630 or RX 6400" {
		t.Fatalf("gpu = %+v", gpu)
	}
}

func TestCheckCarriesTextsAndHave(t *testing.T) {
	r := compare(rig(),
		rows("Memory", "8 GB RAM", "Storage", "50 GB available space"),
		rows("Memory", "16 GB RAM"))
	var ram, disk Check
	for _, c := range r.Checks {
		switch c.Kind {
		case KindRAM:
			ram = c
		case KindStorage:
			disk = c
		}
	}
	if ram.Required != "8 GB RAM" || ram.Recommended != "16 GB RAM" || ram.Have.Bytes != 16*gib || ram.Status != StatusOK {
		t.Fatalf("ram = %+v", ram)
	}
	if disk.Required != "50 GB available space" || disk.Recommended != "" || disk.Have.Free == nil || *disk.Have.Free != 200*gib {
		t.Fatalf("disk = %+v", disk)
	}
	order := []Kind{}
	for _, c := range r.Checks {
		order = append(order, c.Kind)
	}
	if order[0] != KindRAM || order[1] != KindStorage {
		t.Fatalf("order = %v", order)
	}
}

func TestReportJSONShape(t *testing.T) {
	r := compare(rig(), rows("Memory", "8 GB RAM"), nil)
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"verdict":"good","reason":"","checks":[{"kind":"ram","required":"8 GB RAM","recommended":"","have":{"text":"","cores":0,"bits":0,"bytes":17179869184,"free":null,"directX":0},"status":"ok"}]}`
	if string(raw) != want {
		t.Fatalf("json = %s\nwant   %s", raw, want)
	}
}

func TestHaveValues(t *testing.T) {
	hw := rig()
	free := 200 * gib
	tests := []struct {
		kind Kind
		want Have
	}{
		{KindOS, Have{Text: "Windows 10 Pro (build 19045)", Bits: 64}},
		{KindCPU, Have{Text: "Intel(R) Core(TM) i5-8400 CPU @ 2.80GHz", Cores: 6}},
		{KindRAM, Have{Bytes: 16 * gib}},
		{KindGPU, Have{Text: "NVIDIA GeForce GTX 1060 6GB"}},
		{KindVRAM, Have{Bytes: 6 * gib}},
		{KindDirectX, Have{DirectX: 12}},
		{KindStorage, Have{Free: &free}},
	}
	for _, tc := range tests {
		got := hw.have(tc.kind)
		if got.Text != tc.want.Text || got.Cores != tc.want.Cores || got.Bits != tc.want.Bits ||
			got.Bytes != tc.want.Bytes || got.DirectX != tc.want.DirectX || (got.Free == nil) != (tc.want.Free == nil) ||
			(got.Free != nil && *got.Free != *tc.want.Free) {
			t.Errorf("have(%s) = %+v, want %+v", tc.kind, got, tc.want)
		}
	}
	if got := (Hardware{}).have(KindStorage); got.Free != nil {
		t.Errorf("unknown free space = %+v, want no value", got)
	}
	full := hw
	full.FreeBytes = 0
	if got := full.have(KindStorage); got.Free == nil || *got.Free != 0 {
		t.Errorf("full disk = %+v, want a known zero", got)
	}
	if got := (Hardware{}).have(KindRAM); got.Bytes != 0 {
		t.Errorf("unknown ram have = %+v", got)
	}
}

func TestClockOnlyCPURequirement(t *testing.T) {
	old := rig()
	old.CPUName = "Intel(R) Core(TM)2 Duo CPU E8400 @ 3.00GHz"
	unknown := rig()
	unknown.CPUName = "Some Vendor CPU"
	tests := []struct {
		name string
		hw   Hardware
		cpu  string
		want Status
	}{
		{"modern part, 2 Ghz", rig(), "2 Ghz", StatusOK},
		{"modern part, MHz", rig(), "1500 MHz", StatusOK},
		{"modern part, comma", rig(), "2,4 ГГц", StatusOK},
		{"too fast to judge", rig(), "4.5 GHz", StatusUnknown},
		{"old part", old, "2 Ghz", StatusUnknown},
		{"unknown part", unknown, "2 Ghz", StatusUnknown},
		{"no clock", rig(), "a fast one", StatusUnknown},
		{"cores still decide", rig(), "Dual Core 2.0 Ghz", StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := compare(tc.hw, rows("Processor", tc.cpu), nil)
			if len(r.Checks) != 1 || r.Checks[0].Status != tc.want {
				t.Fatalf("checks = %+v, want %s", r.Checks, tc.want)
			}
		})
	}
}
