package sysreq

import "testing"

const gb = gib

func TestParseRows(t *testing.T) {
	tests := []struct {
		name  string
		label string
		value string
		check func(t *testing.T, r reqSet)
	}{
		{"ram gb", "Memory", "8 GB RAM", func(t *testing.T, r reqSet) { eq(t, r.ram, 8*gb) }},
		{"ram mb", "Memory", "4096 MB RAM", func(t *testing.T, r reqSet) { eq(t, r.ram, 4*gb) }},
		{"ram decimal comma", "Memory", "1,5 GB RAM", func(t *testing.T, r reqSet) { eq(t, r.ram, gb+gb/2) }},
		{"ram russian label", "Оперативная память", "8 GB ОЗУ", func(t *testing.T, r reqSet) { eq(t, r.ram, 8*gb) }},
		{"ram russian units", "Память", "4 ГБ ОЗУ", func(t *testing.T, r reqSet) { eq(t, r.ram, 4*gb) }},
		{"ram label with colon", "Memory:", "16 GB RAM", func(t *testing.T, r reqSet) { eq(t, r.ram, 16*gb) }},
		{"storage available", "Storage", "50 GB available space", func(t *testing.T, r reqSet) { eq(t, r.storage, 50*gb) }},
		{"storage hard drive", "Hard Drive", "2 GB", func(t *testing.T, r reqSet) { eq(t, r.storage, 2*gb) }},
		{"storage russian", "Место на диске", "50 GB", func(t *testing.T, r reqSet) { eq(t, r.storage, 50*gb) }},
		{"storage mb", "Disk Space", "800 MB", func(t *testing.T, r reqSet) { eq(t, r.storage, 800*mib) }},
		{"os win10 64", "OS", "Windows 10 64-bit", func(t *testing.T, r reqSet) {
			eq(t, r.osRank, 10)
			ok(t, r.need64)
		}},
		{"os win7 sp1", "OS", "Windows 7 SP1 or later", func(t *testing.T, r reqSet) { eq(t, r.osRank, 7) }},
		{"os win slash list", "OS", "Windows 7/8/10", func(t *testing.T, r reqSet) { eq(t, r.osRank, 7) }},
		{"os windows with registered mark", "OS", "Windows® 8.1", func(t *testing.T, r reqSet) { eq(t, r.osRank, 9) }},
		{"os russian", "ОС", "Windows 10 (64-разрядная)", func(t *testing.T, r reqSet) {
			eq(t, r.osRank, 10)
			ok(t, r.need64)
		}},
		{"os xp", "Operating System", "Windows XP", func(t *testing.T, r reqSet) { eq(t, r.osRank, 5) }},
		{"64-bit statement", "", "Requires a 64-bit processor and operating system", func(t *testing.T, r reqSet) {
			ok(t, r.need64)
			eq(t, r.osRank, 0)
			ok(t, r.has(KindOS))
		}},
		{"os without windows is unparsed", "OS", "macOS 10.15", func(t *testing.T, r reqSet) {
			ok(t, r.has(KindOS))
			ok(t, r.unparsed[KindOS])
		}},
		{"cpu models", "Processor", "Intel Core i5-4460 or AMD FX-6300", func(t *testing.T, r reqSet) { eq(t, r.cpuTier, 12) }},
		{"cpu quad core", "Processor", "Quad core Intel or AMD, 2.5 GHz", func(t *testing.T, r reqSet) { eq(t, r.cores, 4) }},
		{"cpu numeric cores", "Processor", "4 cores @ 3.0 GHz", func(t *testing.T, r reqSet) { eq(t, r.cores, 4) }},
		{"cpu russian cores", "Процессор", "4-ядерный Intel Core i5", func(t *testing.T, r reqSet) { eq(t, r.cores, 4) }},
		{"cpu russian word", "Процессор", "Четырёхъядерный процессор", func(t *testing.T, r reqSet) { eq(t, r.cores, 4) }},
		{"cpu clock only", "Processor", "2.4 GHz", func(t *testing.T, r reqSet) {
			eq(t, r.clockMHz, 2400)
			ok(t, !r.unparsed[KindCPU])
		}},
		{"cpu absurd clock is unparsed", "Processor", "99999999999999999999 GHz", func(t *testing.T, r reqSet) {
			eq(t, r.clockMHz, 0)
			ok(t, r.unparsed[KindCPU])
		}},
		{"cpu without model, cores or clock is unparsed", "Processor", "Any modern processor", func(t *testing.T, r reqSet) {
			ok(t, r.has(KindCPU))
			ok(t, r.unparsed[KindCPU])
		}},
		{"cpu model not in tables is unparsed", "Processor", "Intel Xeon E5-2670", func(t *testing.T, r reqSet) { ok(t, r.unparsed[KindCPU]) }},
		{"cpu mixed alternatives are partial", "Processor", "Intel Core i5-4460 or AMD Ryzen 3", func(t *testing.T, r reqSet) {
			ok(t, r.cpuPartial)
			ok(t, !r.unparsed[KindCPU])
		}},
		{"gpu with alternatives", "Graphics", "NVIDIA GeForce GTX 1060 6GB / AMD Radeon RX 580", func(t *testing.T, r reqSet) {
			eq(t, r.gpuTier, 6)
			eq(t, r.vram, 0)
		}},
		{"gpu vram stated for every alternative", "Graphics", "GTX 1060 6GB or RX 580 8GB", func(t *testing.T, r reqSet) { eq(t, r.vram, 6*gb) }},
		{"gpu vram stated for one alternative only", "Graphics", "GTX 1060 or RX 570 4GB", func(t *testing.T, r reqSet) {
			eq(t, r.gpuTier, 6)
			eq(t, r.vram, 0)
			ok(t, !r.has(KindVRAM))
		}},
		{"gpu not in tables is unparsed", "Graphics", "GeForce GTX 1630 or RX 6400", func(t *testing.T, r reqSet) {
			ok(t, r.has(KindGPU))
			ok(t, r.unparsed[KindGPU])
		}},
		{"gpu partly rated is partial", "Graphics", "GTX 1630 or GTX 1060", func(t *testing.T, r reqSet) {
			ok(t, r.gpuPartial)
			eq(t, r.gpuTier, 6)
		}},
		{"gpu generation with similar tail", "Graphics", "GeForce GTX 700 series or similar", func(t *testing.T, r reqSet) {
			eq(t, r.gpuTier, 4)
			ok(t, !r.gpuPartial)
			ok(t, !r.unparsed[KindGPU])
		}},
		{"gpu nonexistent generation is unparsed", "Graphics", "GeForce GTX 9900 series or similar", func(t *testing.T, r reqSet) {
			eq(t, r.gpuTier, 0)
			ok(t, r.unparsed[KindGPU])
		}},
		{"ram without size is unparsed", "Memory", "Enough RAM", func(t *testing.T, r reqSet) { ok(t, r.unparsed[KindRAM]) }},
		{"storage without size is unparsed", "Storage", "SSD recommended", func(t *testing.T, r reqSet) { ok(t, r.unparsed[KindStorage]) }},
		{"directx without version is unparsed", "DirectX", "Compatible", func(t *testing.T, r reqSet) { ok(t, r.unparsed[KindDirectX]) }},
		{"gpu vram phrase", "Graphics", "GeForce GTX 660, 2 GB VRAM", func(t *testing.T, r reqSet) {
			eq(t, r.gpuTier, 5)
			eq(t, r.vram, 2*gb)
		}},
		{"gpu video memory phrase", "Graphics", "Radeon HD 7850 with 2GB video memory", func(t *testing.T, r reqSet) {
			eq(t, r.gpuTier, 4)
			eq(t, r.vram, 2*gb)
		}},
		{"gpu system ram size ignored", "Graphics", "GeForce GTX 750 Ti, 8 GB RAM", func(t *testing.T, r reqSet) {
			eq(t, r.gpuTier, 4)
			eq(t, r.vram, 0)
		}},
		{"gpu russian label", "Видеокарта", "NVIDIA GeForce GTX 970 или AMD Radeon R9 290", func(t *testing.T, r reqSet) { eq(t, r.gpuTier, 6) }},
		{"gpu dx only has no tier", "Graphics", "DirectX 11 compatible video card", func(t *testing.T, r reqSet) {
			eq(t, r.gpuTier, 0)
			eq(t, r.dx, 11)
		}},
		{"vram label", "Video Memory", "2 GB", func(t *testing.T, r reqSet) { eq(t, r.vram, 2*gb) }},
		{"directx version", "DirectX", "Version 11", func(t *testing.T, r reqSet) { eq(t, r.dx, 11) }},
		{"directx with colon", "Graphics", "DirectX: Version 12", func(t *testing.T, r reqSet) { eq(t, r.dx, 12) }},
		{"directx 9c", "DirectX", "Version 9.0c", func(t *testing.T, r reqSet) { eq(t, r.dx, 9) }},
		{"label packed in value", "", "Видеокарта: GeForce GTX 1060", func(t *testing.T, r reqSet) { eq(t, r.gpuTier, 6) }},
		{"packed ram russian", "", "Оперативная память: 8 GB ОЗУ", func(t *testing.T, r reqSet) { eq(t, r.ram, 8*gb) }},
		{"packed storage russian", "", "Место на диске: 50 GB", func(t *testing.T, r reqSet) { eq(t, r.storage, 50*gb) }},
		{"additional notes ignored", "Additional Notes", "Requires 8 GB free", func(t *testing.T, r reqSet) {
			eq(t, r.storage, 0)
			eq(t, r.ram, 0)
		}},
		{"empty value skipped", "Memory", "  ", func(t *testing.T, r reqSet) { eq(t, r.ram, 0) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.check(t, parseRows([]Row{{Label: tc.label, Value: tc.value}}))
		})
	}
}

func TestParseKeepsOriginalText(t *testing.T) {
	r := parseRows([]Row{{Label: "Memory", Value: "8 GB RAM"}, {Label: "Storage", Value: "50 GB available space"}})
	if got := r.text(KindRAM); got != "8 GB RAM" {
		t.Fatalf("ram text = %q", got)
	}
	if got := r.text(KindStorage); got != "50 GB available space" {
		t.Fatalf("storage text = %q", got)
	}
}

func TestWindowsRank(t *testing.T) {
	tests := []struct {
		os   string
		want int
	}{
		{"Windows 10 Pro (build 19045)", 10},
		{"Windows 10 Pro (build 22631)", 11},
		{"Windows 10 Home", 0},
		{"Windows 10 Pro (build abc)", 0},
		{"Windows 7 Professional", 7},
		{"Windows 8.1 Pro", 9},
		{"Windows 11 Home", 11},
		{"macOS 14.2", 0},
		{"windows", 0},
		{"", 0},
	}
	for _, tc := range tests {
		if got := windowsRank(tc.os); got != tc.want {
			t.Errorf("windowsRank(%q) = %d, want %d", tc.os, got, tc.want)
		}
	}
}

func eq[T comparable](t *testing.T, got, want T) {
	t.Helper()
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func ok(t *testing.T, cond bool) {
	t.Helper()
	if !cond {
		t.Fatal("condition is false")
	}
}
