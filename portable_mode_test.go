package main

import (
	"strings"
	"testing"
)

func TestPortableInstanceIDSeparatesDataDirectories(t *testing.T) {
	a := portableInstanceID(`D:\Games\Typhon\data`)
	b := portableInstanceID(`E:\Typhon\data`)
	if a == b {
		t.Fatalf("two data directories share the instance id %q", a)
	}
	if a == singleInstanceID || !strings.HasPrefix(a, singleInstanceID+"-") {
		t.Fatalf("instance id %q must extend %q", a, singleInstanceID)
	}
	if got := len(a) - len(singleInstanceID) - 1; got != portableInstanceHexLen {
		t.Fatalf("suffix length = %d, want %d", got, portableInstanceHexLen)
	}
}

func TestPortableInstanceIDIgnoresCase(t *testing.T) {
	if portableInstanceID(`D:\Typhon\Data`) != portableInstanceID(`d:\typhon\data`) {
		t.Fatal("the same folder spelled in another case got a different instance id")
	}
}
