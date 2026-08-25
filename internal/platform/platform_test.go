package platform

import (
	"errors"
	"os"
	"testing"
)

func TestGetStorageInfo(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	info, err := GetStorageInfo(home)
	if err != nil {
		t.Fatal(err)
	}
	if info.TotalBytes == 0 {
		t.Fatal("total bytes is zero")
	}
	if info.UsedBytes+info.FreeBytes > info.TotalBytes {
		t.Fatalf("used %d + free %d exceeds total %d", info.UsedBytes, info.FreeBytes, info.TotalBytes)
	}
}

func TestGetStorageInfoMissingPath(t *testing.T) {
	home, _ := os.UserHomeDir()
	info, err := GetStorageInfo(home + string(os.PathSeparator) + "definitely-missing-dir-typhon")
	if err != nil {
		t.Fatal(err)
	}
	if info.TotalBytes == 0 {
		t.Fatal("total bytes is zero for missing path")
	}
}

func TestGetSystemInfo(t *testing.T) {
	info, err := GetSystemInfo()
	if err != nil {
		t.Fatal(err)
	}
	if info.Cores <= 0 {
		t.Fatal("no cores reported")
	}
	if info.RAMBytes == 0 {
		t.Fatal("no ram reported")
	}
	if info.OS == "" || info.CPU == "" {
		t.Fatalf("empty os/cpu: %+v", info)
	}
}

func TestGetStorageInfoEmptyPath(t *testing.T) {
	for _, path := range []string{"", "   "} {
		if _, err := GetStorageInfo(path); !errors.Is(err, ErrEmptyPath) {
			t.Fatalf("GetStorageInfo(%q) err = %v, want ErrEmptyPath", path, err)
		}
	}
}
