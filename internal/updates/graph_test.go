package updates

import "testing"

func patch(id, from, to string, size int64) Patch {
	return Patch{ID: id, FromVersion: from, ToVersion: to, ReleaseID: id, Size: size}
}

func TestFindPatchPathPrefersCheapestChain(t *testing.T) {
	patches := []Patch{
		patch("a", "1.0", "1.1", 2<<30),
		patch("b", "1.1", "1.2", 3<<30),
		patch("c", "1.2", "1.3", 2<<30),
		patch("d", "1.0", "1.3", 15<<30),
	}
	path, ok := FindPatchPath(patches, "1.0", "1.3")
	if !ok {
		t.Fatal("expected a path from 1.0 to 1.3")
	}
	if len(path.Steps) != 3 {
		t.Fatalf("steps = %d, want 3", len(path.Steps))
	}
	if path.Bytes != 7<<30 {
		t.Fatalf("bytes = %d, want %d", path.Bytes, int64(7)<<30)
	}
	for i, want := range []string{"a", "b", "c"} {
		if path.Steps[i].ID != want {
			t.Fatalf("step %d = %q, want %q", i, path.Steps[i].ID, want)
		}
	}
}

func TestFindPatchPathPrefersDirectWhenCheaper(t *testing.T) {
	patches := []Patch{
		patch("a", "1.0", "1.1", 9<<30),
		patch("b", "1.1", "1.2", 9<<30),
		patch("direct", "1.0", "1.2", 4<<30),
	}
	path, ok := FindPatchPath(patches, "1.0", "1.2")
	if !ok || len(path.Steps) != 1 || path.Steps[0].ID != "direct" {
		t.Fatalf("path = %+v, ok = %v", path, ok)
	}
}

func TestFindPatchPathMissing(t *testing.T) {
	patches := []Patch{patch("a", "1.0", "1.1", 1)}
	if _, ok := FindPatchPath(patches, "1.0", "2.0"); ok {
		t.Fatal("expected no path to 2.0")
	}
	if _, ok := FindPatchPath(patches, "1.0", "1.0"); ok {
		t.Fatal("expected no path to the same version")
	}
	if _, ok := FindPatchPath(nil, "1.0", "1.1"); ok {
		t.Fatal("expected no path without patches")
	}
}

func TestFindPatchPathNormalizesVersions(t *testing.T) {
	patches := []Patch{patch("a", "v1.0", "1.1", 0), patch("b", "1.10", "1.2", 0)}
	path, ok := FindPatchPath(patches, "1.0.0", "1.1")
	if !ok || len(path.Steps) != 1 || path.Steps[0].ID != "a" {
		t.Fatalf("path = %+v, ok = %v", path, ok)
	}
	if path.Unknown != 1 {
		t.Fatalf("unknown = %d, want 1", path.Unknown)
	}
}

func TestFindPatchPathAvoidsUnknownSize(t *testing.T) {
	patches := []Patch{
		patch("free", "1.0", "1.1", 0),
		patch("known", "1.0", "1.1", 5<<30),
	}
	path, ok := FindPatchPath(patches, "1.0", "1.1")
	if !ok || path.Steps[0].ID != "known" {
		t.Fatalf("path = %+v, ok = %v", path, ok)
	}
}
