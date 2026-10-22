package platform

import (
	"encoding/binary"
	"errors"
	"testing"
)

func TestDecodeLE(t *testing.T) {
	b4 := make([]byte, 4)
	binary.LittleEndian.PutUint32(b4, 2<<30)
	b8 := make([]byte, 8)
	binary.LittleEndian.PutUint64(b8, 16<<30)
	tests := []struct {
		name string
		in   []byte
		want uint64
		ok   bool
	}{
		{"dword", b4, 2 << 30, true},
		{"qword", b8, 16 << 30, true},
		{"odd length", []byte{1, 2, 3}, 0, false},
		{"empty", nil, 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := decodeLE(tc.in)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("got %d,%v want %d,%v", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestPickGPU(t *testing.T) {
	const gb = 1 << 30
	q := func(desc string, v uint64) adapter { return adapter{Desc: desc, QWord: v, HasQWord: true} }
	tests := []struct {
		name    string
		in      []adapter
		want    GPUInfo
		wantErr error
	}{
		{"empty", nil, GPUInfo{}, ErrNoGPU},
		{"only basic adapter", []adapter{{Desc: "Microsoft Basic Display Adapter"}}, GPUInfo{}, ErrNoGPU},
		{"blank name", []adapter{{Desc: "  "}}, GPUInfo{}, ErrNoGPU},
		{"ghost only", []adapter{{Desc: "NVIDIA GeForce GTX 1080", Absent: true, QWord: 8 * gb, HasQWord: true}}, GPUInfo{}, ErrNoGPU},
		{
			"ghost of a removed card is ignored",
			[]adapter{
				{Desc: "NVIDIA GeForce RTX 4090", Absent: true, QWord: 24 * gb, HasQWord: true},
				q("NVIDIA GeForce GTX 1060 6GB", 6*gb),
			},
			GPUInfo{Name: "NVIDIA GeForce GTX 1060 6GB", VRAMBytes: 6 * gb, VRAMKnown: true}, nil,
		},
		{"qword memory", []adapter{q("NVIDIA GeForce GTX 1060 6GB", 6*gb)},
			GPUInfo{Name: "NVIDIA GeForce GTX 1060 6GB", VRAMBytes: 6 * gb, VRAMKnown: true}, nil},
		{"dword memory", []adapter{{Desc: "AMD Radeon HD 7850", DWord: 2 * gb, HasDWord: true}},
			GPUInfo{Name: "AMD Radeon HD 7850", VRAMBytes: 2 * gb, VRAMKnown: true}, nil},
		{"dword cap without qword is unknown", []adapter{{Desc: "NVIDIA GeForce RTX 3080", DWord: 0xFFFFFFFF, HasDWord: true}},
			GPUInfo{Name: "NVIDIA GeForce RTX 3080"}, nil},
		{"qword wins over capped dword", []adapter{{Desc: "NVIDIA GeForce RTX 3080", DWord: 0xFFFFFFFF, HasDWord: true, QWord: 10 * gb, HasQWord: true}},
			GPUInfo{Name: "NVIDIA GeForce RTX 3080", VRAMBytes: 10 * gb, VRAMKnown: true}, nil},
		{"zero memory is unknown", []adapter{q("Intel(R) UHD Graphics 630", 0)},
			GPUInfo{Name: "Intel(R) UHD Graphics 630"}, nil},
		{
			"discrete beats integrated and virtual",
			[]adapter{
				q("Intel(R) UHD Graphics 630", 128<<20),
				q("Parsec Virtual Display Adapter", 32*gb),
				q("NVIDIA GeForce RTX 3060", 12*gb),
			},
			GPUInfo{Name: "NVIDIA GeForce RTX 3060", VRAMBytes: 12 * gb, VRAMKnown: true}, nil,
		},
		{
			"discrete with unknown memory beats integrated with known",
			[]adapter{
				q("Intel(R) HD Graphics 4600", 1*gb),
				{Desc: "NVIDIA GeForce GTX 750"},
			},
			GPUInfo{Name: "NVIDIA GeForce GTX 750"}, nil,
		},
		{
			"discrete capped dword beats integrated",
			[]adapter{
				q("AMD Radeon(TM) Graphics", 512<<20),
				{Desc: "AMD Radeon RX 6800", DWord: 0xFFFFFFFF, HasDWord: true},
			},
			GPUInfo{Name: "AMD Radeon RX 6800"}, nil,
		},
		{
			"known memory breaks a discrete tie",
			[]adapter{{Desc: "NVIDIA GeForce GTX 750"}, q("NVIDIA GeForce GTX 1050", 2*gb)},
			GPUInfo{Name: "NVIDIA GeForce GTX 1050", VRAMBytes: 2 * gb, VRAMKnown: true}, nil,
		},
		{
			"intel arc is not integrated",
			[]adapter{{Desc: "Intel(R) UHD Graphics 770"}, {Desc: "Intel(R) Arc(TM) A750 Graphics"}},
			GPUInfo{Name: "Intel(R) Arc(TM) A750 Graphics"}, nil,
		},
		{
			"real card with remote in its name is kept",
			[]adapter{{Desc: "Remote Gaming GeForce Pro"}},
			GPUInfo{Name: "Remote Gaming GeForce Pro"}, nil,
		},
		{"microsoft remote display is virtual", []adapter{{Desc: "Microsoft Remote Display Adapter"}}, GPUInfo{}, ErrNoGPU},
		{"hyper-v video is virtual", []adapter{{Desc: "Microsoft Hyper-V Video"}}, GPUInfo{}, ErrNoGPU},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := pickGPU(tc.in)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestDetectGPUPropagatesReadError(t *testing.T) {
	boom := errors.New("access denied")
	_, err := detectGPU(func() ([]adapter, error) { return nil, boom })
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
}

func TestDetectGPUFromReader(t *testing.T) {
	got, err := detectGPU(func() ([]adapter, error) {
		return []adapter{{Desc: "AMD Radeon RX 580", QWord: 8 << 30, HasQWord: true}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "AMD Radeon RX 580" || !got.VRAMKnown || got.VRAMBytes != 8<<30 {
		t.Fatalf("got %+v", got)
	}
}

func record(relation, size uint32) []byte {
	b := make([]byte, size)
	binary.LittleEndian.PutUint32(b, relation)
	binary.LittleEndian.PutUint32(b[4:], size)
	return b
}

func TestCountCores(t *testing.T) {
	join := func(parts ...[]byte) []byte {
		var out []byte
		for _, p := range parts {
			out = append(out, p...)
		}
		return out
	}
	tests := []struct {
		name    string
		buf     []byte
		want    int
		wantErr bool
	}{
		{"empty", nil, 0, true},
		{"six cores", join(record(0, 48), record(0, 48), record(0, 48), record(0, 48), record(0, 48), record(0, 48)), 6, false},
		{"other relations ignored", join(record(0, 32), record(2, 40), record(0, 32)), 2, false},
		{"only caches", join(record(2, 40)), 0, true},
		{"truncated header", []byte{0, 0, 0}, 0, true},
		{"size past buffer", join(record(0, 32))[:20], 0, true},
		{"size too small", func() []byte { b := record(0, 8); binary.LittleEndian.PutUint32(b[4:], 4); return b }(), 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := countCores(tc.buf)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}
