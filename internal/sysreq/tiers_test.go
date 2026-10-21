package sysreq

import "testing"

func TestGPUTier(t *testing.T) {
	tests := []struct {
		name string
		want int
	}{
		{"NVIDIA GeForce GTX 1060 6GB", 6},
		{"NVIDIA GeForce GTX 1050 Ti", 5},
		{"NVIDIA GeForce GTX 1050", 4},
		{"NVIDIA GeForce RTX 3060 Laptop GPU", 8},
		{"NVIDIA GeForce RTX 3050 Ti Laptop GPU", 7},
		{"NVIDIA GeForce RTX 4090", 11},
		{"GeForce GT 710", 1},
		{"GeForce GTX 460", 3},
		{"GeForce GTX 9999", 0},
		{"AMD Radeon RX 580", 7},
		{"AMD Radeon RX 6600 XT", 8},
		{"AMD Radeon RX 7900 XTX", 11},
		{"Radeon HD 7850", 4},
		{"AMD Radeon R9 280X", 6},
		{"AMD Radeon R7 260X", 4},
		{"AMD Radeon RX Vega 56", 8},
		{"Intel(R) UHD Graphics 630", 2},
		{"Intel(R) HD Graphics 4000", 1},
		{"Intel(R) HD Graphics", 1},
		{"Intel(R) Iris(R) Xe Graphics", 3},
		{"Intel(R) Arc(TM) A750 Graphics", 8},
		{"AMD Radeon(TM) Graphics", 2},
		{"Something Unheard Of", 0},
		{"", 0},
	}
	for _, tc := range tests {
		if got := gpuTier(tc.name); got != tc.want {
			t.Errorf("gpuTier(%q) = %d, want %d", tc.name, got, tc.want)
		}
	}
}

// A "<N>00 series" generation rates as its lowest model of tier 4 or above, so
// office-grade and entry parts (tier 1-3) never stand for the whole generation;
// NVIDIA and AMD follow the same rule, and a generation with no such model in
// the tables stays 0 (unknown) instead of passing.
func TestGPUTierGeneration(t *testing.T) {
	tests := []struct {
		name string
		want int
	}{
		{"GeForce GTX 700 series", 4},
		{"GeForce GTX 700-series", 4},
		{"GeForce GTX 700 Series", 4},
		{"GeForce GTX 400 series", 4},
		{"GeForce GTX 500 series", 4},
		{"GeForce GTX 900 series", 5},
		{"GeForce GTX 1000 series", 4},
		{"GeForce GTX 10 series", 4},
		{"GeForce GTX 16 series", 6},
		{"GeForce GTX 1600 series", 6},
		{"GeForce RTX 20 series", 8},
		{"GeForce RTX 2000 series", 8},
		{"GeForce RTX 30 series", 7},
		{"GeForce RTX 40 series", 8},
		{"GeForce GTX 1050 series", 4},
		{"GeForce GTX 1060 series", 6},
		{"GeForce GTX 750 series", 4},
		{"Radeon RX 500 series", 4},
		{"Radeon RX 500-series", 4},
		{"Radeon RX 400 series", 4},
		{"Radeon RX 5000 series", 6},
		{"Radeon RX 6000 series", 6},
		{"Radeon RX 7000 series", 8},
		{"Radeon HD 7000 series", 4},
		{"Radeon HD 6000 series", 4},
		{"GeForce GTX 9900 series", 0},
		{"GeForce GTX 99 series", 0},
		{"GeForce GTX 100 series", 0},
		{"Radeon RX 9900 series", 0},
		{"Radeon RX 900 series", 0},
		{"Radeon HD 9000 series", 0},
		{"GeForce GTX 9999", 0},
	}
	for _, tc := range tests {
		if got := gpuTier(tc.name); got != tc.want {
			t.Errorf("gpuTier(%q) = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestCPUTier(t *testing.T) {
	tests := []struct {
		name string
		want int
	}{
		{"Intel(R) Core(TM) i5-4460 CPU @ 3.20GHz", 17},
		{"Intel Core i3-6100", 24},
		{"Intel(R) Core(TM) i7-8700K CPU @ 3.70GHz", 34},
		{"12th Gen Intel(R) Core(TM) i7-12700K", 50},
		{"Intel(R) Core(TM) i5-1135G7 @ 2.40GHz", 45},
		{"Intel(R) Core(TM) i9-13900K", 55},
		{"Intel Core i7-920", 6},
		{"Intel Core Ultra 7 155H", 62},
		{"Intel Core 2 Duo E8400", 1},
		{"Intel Core 2 Quad Q9550", 3},
		{"Intel Pentium G4560", 1},
		{"AMD FX-6300", 12},
		{"AMD FX(tm)-8350 Eight-Core Processor", 13},
		{"AMD Ryzen 5 3600 6-Core Processor", 37},
		{"AMD Ryzen 7 5800X3D", 46},
		{"AMD Ryzen 3 1200", 24},
		{"AMD Phenom II X4 965", 6},
		{"AMD Athlon 64 X2", 1},
		{"Apple M2 Pro", 50},
		{"Intel Core i5", 0},
		{"2.4 GHz", 0},
		{"", 0},
	}
	for _, tc := range tests {
		if got := cpuTier(tc.name); got != tc.want {
			t.Errorf("cpuTier(%q) = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestCPUTierReadsGenerationOfFourDigitModels(t *testing.T) {
	tests := []struct {
		name string
		gen  int
		cls  int
	}{
		{"Intel(R) Core(TM) i5-1235U", 12, 1},
		{"Intel(R) Core(TM) i7-1255U", 12, 2},
		{"Intel(R) Core(TM) i5-1240P", 12, 1},
		{"Intel(R) Core(TM) i7-1355U", 13, 2},
		{"Intel(R) Core(TM) i5-10210U", 10, 1},
		{"Intel(R) Core(TM) i7-1065G7", 10, 2},
		{"Intel(R) Core(TM) i3-1005G1", 10, 0},
		{"Intel(R) Core(TM) i5-2500K", 2, 1},
		{"Intel(R) Core(TM) i7-4790", 4, 2},
	}
	for _, tc := range tests {
		if got, want := cpuTier(tc.name), tc.gen*4+tc.cls; got != want {
			t.Errorf("cpuTier(%q) = %d, want %d", tc.name, got, want)
		}
	}
}

func TestCPUTierGenerationOutranksClass(t *testing.T) {
	older := []string{"Intel Core i9-9900K", "Intel Core i7-11700K", "Intel Core i9-10900K", "AMD Ryzen 9 3900X"}
	newer := []string{"Intel Core i5-12400F", "Intel Core i3-12100", "AMD Ryzen 5 7600"}
	for _, o := range older {
		for _, n := range newer {
			if cpuTier(o) >= cpuTier(n) {
				t.Errorf("cpuTier(%q)=%d should be below cpuTier(%q)=%d", o, cpuTier(o), n, cpuTier(n))
			}
			if cpuTier(n) <= cpuTier(o) {
				t.Errorf("cpuTier(%q)=%d should be above cpuTier(%q)=%d", n, cpuTier(n), o, cpuTier(o))
			}
		}
	}
	if cpuTier("Intel Core i9-13900K") <= cpuTier("Intel Core i5-13400") {
		t.Error("within a generation the higher class must win")
	}
}

func TestMinTierTakesEasiestAlternative(t *testing.T) {
	tests := []struct {
		text    string
		rate    func(string) int
		want    int
		partial bool
	}{
		{"Intel Core i5-4460 or AMD FX-6300", cpuTier, 12, false},
		{"Intel Core i7-8700 / AMD Ryzen 5 2600", cpuTier, 29, false},
		{"GTX 1060 или Radeon RX 580", gpuTier, 6, false},
		{"GTX 1060 or equivalent", gpuTier, 6, false},
		{"GTX 1060 or AMD equivalent", gpuTier, 6, false},
		{"or equivalent", gpuTier, 0, false},
		{"GTX 1630 or GTX 1060", gpuTier, 6, true},
		{"GTX 1630 or RX 6400", gpuTier, 0, false},
		{"GeForce GTX 1050 series or similar", gpuTier, 4, false},
		{"GeForce GTX 700 series or similar", gpuTier, 4, false},
		{"GeForce GTX 700 series or equivalent", gpuTier, 4, false},
		{"GeForce GTX 700 series or better", gpuTier, 4, false},
		{"GeForce GTX 700 series and above", gpuTier, 4, false},
		{"GeForce GTX 700 series или аналог", gpuTier, 4, false},
		{"GeForce GTX 700 series и выше", gpuTier, 4, false},
		{"GeForce GTX 700 series or similar AMD", gpuTier, 4, false},
		{"GeForce GTX 9900 series or similar", gpuTier, 0, false},
		{"GeForce GTX 700 series or GTX 1630", gpuTier, 4, true},
	}
	for _, tc := range tests {
		got, partial := minTier(tc.text, tc.rate)
		if got != tc.want || partial != tc.partial {
			t.Errorf("minTier(%q) = %d,%v want %d,%v", tc.text, got, partial, tc.want, tc.partial)
		}
	}
}
