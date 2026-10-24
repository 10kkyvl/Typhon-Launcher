package app

import (
	"encoding/json"
	"testing"
)

func TestCurrentInfoReportsOrdinaryModeWithoutMarker(t *testing.T) {
	info := CurrentInfo()
	if info.Portable || info.DataDir != "" {
		t.Fatalf("CurrentInfo() = %+v, want an ordinary install next to a test binary", info)
	}
}

func TestAppInfoWireNamesForPortableMode(t *testing.T) {
	raw, err := json.Marshal(AppInfo{Portable: true, DataDir: `D:\Typhon\data`})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["portable"] != true || fields["dataDir"] != `D:\Typhon\data` {
		t.Fatalf("AppInfo JSON = %s, want portable and dataDir", raw)
	}
}

func TestCurrentInfoChannel(t *testing.T) {
	cases := []struct {
		version string
		want    string
	}{
		{"1.2.0", "stable"},
		{"1.3.0-beta.1", "beta"},
		{"1.3.0-beta.12", "beta"},
		{"dev", "stable"},
	}
	for _, c := range cases {
		t.Run(c.version, func(t *testing.T) {
			prev := Version
			Version = c.version
			t.Cleanup(func() { Version = prev })
			if got := CurrentInfo().Channel; got != c.want {
				t.Fatalf("Channel = %q, want %q", got, c.want)
			}
		})
	}
}
