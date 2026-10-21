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
