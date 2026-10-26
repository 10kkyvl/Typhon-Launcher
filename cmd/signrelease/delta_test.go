package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/selfupdate"
	"typhon/internal/selfupdate/delta"
)

func releaseBytes(seed uint64, n int) []byte {
	out := make([]byte, 0, n+sha256.Size)
	var block [16]byte
	binary.LittleEndian.PutUint64(block[:8], seed)
	for counter := uint64(0); len(out) < n; counter++ {
		binary.LittleEndian.PutUint64(block[8:], counter)
		sum := sha256.Sum256(block[:])
		out = append(out, sum[:]...)
	}
	return out[:n]
}

func sumOf(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

type deltaRelease struct {
	dir       string
	installer string
	zip       string
	newExe    string
	newBytes  []byte
	olds      map[string][]byte
	oldPaths  map[string]string
	patches   map[string]string
}

// newDeltaRelease writes a new executable, an installer and a zip around it,
// and for each older version an executable and the patch to the new one.
func newDeltaRelease(t *testing.T, versions ...string) *deltaRelease {
	t.Helper()
	dir := t.TempDir()
	r := &deltaRelease{dir: dir, olds: map[string][]byte{}, oldPaths: map[string]string{}, patches: map[string]string{}}
	base := releaseBytes(1, 96<<10)
	r.newBytes = append(append([]byte(nil), base...), releaseBytes(2, 2<<10)...)
	r.newExe = writeArtifactFile(t, dir, "typhon.exe", r.newBytes)
	r.installer = writeArtifactFile(t, dir, "typhon-amd64-installer.exe", releaseBytes(3, 200<<10))
	r.zip = writeArtifactFile(t, dir, "typhon-windows-portable.zip", releaseBytes(4, 150<<10))
	for i, v := range versions {
		old := append(append([]byte(nil), base[:len(base)-(i+1)*1024]...), releaseBytes(uint64(10+i), 512)...)
		r.olds[v] = old
		r.oldPaths[v] = writeArtifactFile(t, dir, "typhon-"+v+".exe", old)
		var patch bytes.Buffer
		if err := delta.Encode(&patch, old, r.newBytes); err != nil {
			t.Fatalf("delta.Encode: %v", err)
		}
		r.patches[v] = writeArtifactFile(t, dir, "typhon-"+v+"-to-1.3.0.delta", patch.Bytes())
	}
	return r
}

func (r *deltaRelease) args(t *testing.T, out string, versions ...string) []string {
	t.Helper()
	args := []string{
		"-version", "1.3.0",
		"-base-url", "https://api.typhon-launcher.com/launcher/download",
		"-out", out,
		"-artifact", "os=windows,arch=amd64,kind=installer,path=" + r.installer + ",portable=" + r.zip + ",exe=" + r.newExe,
	}
	for _, v := range versions {
		args = append(args, "-delta", "from="+v+",old="+r.oldPaths[v]+",patch="+r.patches[v])
	}
	return args
}

func TestRun_SignsPatchesAndTheExecutableTheyLeadTo(t *testing.T) {
	r := newDeltaRelease(t, "1.2.0", "1.1.0")
	priv, pub := generateKey(t)
	out := filepath.Join(r.dir, "manifest.json")
	env := envFor(map[string]string{privateKeyEnv: base64.StdEncoding.EncodeToString(priv)})

	signed, err := run(t.Context(), r.args(t, out, "1.2.0", "1.1.0"), &bytes.Buffer{}, env)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	m, err := selfupdate.VerifyManifest(signed, pub)
	if err != nil {
		t.Fatalf("VerifyManifest: %v", err)
	}
	a := m.Artifacts[0]
	if a.ExeSHA256 != sumOf(r.newBytes) || a.ExeSize != int64(len(r.newBytes)) {
		t.Fatalf("exe = %s / %d, want the new typhon.exe", a.ExeSHA256, a.ExeSize)
	}
	if len(a.Deltas) != 2 {
		t.Fatalf("deltas = %d, want 2", len(a.Deltas))
	}
	for i, v := range []string{"1.2.0", "1.1.0"} {
		d := a.Deltas[i]
		patch, err := os.ReadFile(r.patches[v])
		if err != nil {
			t.Fatal(err)
		}
		want := selfupdate.Delta{
			From: v, FromSHA256: sumOf(r.olds[v]), Name: "typhon-" + v + "-to-1.3.0.delta",
			URL:  "https://api.typhon-launcher.com/launcher/download/1.3.0/typhon-" + v + "-to-1.3.0.delta",
			Size: int64(len(patch)), SHA256: sumOf(patch),
		}
		if d != want {
			t.Errorf("delta %d = %+v, want %+v", i, d, want)
		}
	}
	portable, err := m.PortableArtifactFor("windows", "amd64")
	if err != nil {
		t.Fatalf("PortableArtifactFor: %v", err)
	}
	if portable.ExeSHA256 != a.ExeSHA256 || len(portable.Deltas) != 2 {
		t.Fatalf("the portable build lost the patches: %+v", portable)
	}
}

func TestRun_ExecutableWithoutPatchesStillSigns(t *testing.T) {
	r := newDeltaRelease(t)
	priv, pub := generateKey(t)
	out := filepath.Join(r.dir, "manifest.json")
	env := envFor(map[string]string{privateKeyEnv: base64.StdEncoding.EncodeToString(priv)})

	signed, err := run(t.Context(), r.args(t, out), &bytes.Buffer{}, env)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	m, err := selfupdate.VerifyManifest(signed, pub)
	if err != nil {
		t.Fatalf("VerifyManifest: %v", err)
	}
	if a := m.Artifacts[0]; a.ExeSHA256 != sumOf(r.newBytes) || len(a.Deltas) != 0 {
		t.Fatalf("artifact = %+v, want the executable hash and no patches", a)
	}
}

func TestRun_RefusesToSignWhatDoesNotWork(t *testing.T) {
	priv, _ := generateKey(t)
	env := envFor(map[string]string{privateKeyEnv: base64.StdEncoding.EncodeToString(priv)})

	tests := []struct {
		name    string
		mutate  func(t *testing.T, r *deltaRelease, args []string) []string
		wantErr error
		anyErr  bool
	}{
		{
			name: "patch made against another executable",
			mutate: func(t *testing.T, r *deltaRelease, args []string) []string {
				writeArtifactFile(t, r.dir, "typhon-1.2.0.exe", releaseBytes(99, 90<<10))
				return args
			},
			wantErr: delta.ErrCorrupt,
		},
		{
			name: "patch for another new executable",
			mutate: func(t *testing.T, r *deltaRelease, args []string) []string {
				var other bytes.Buffer
				if err := delta.Encode(&other, r.olds["1.2.0"], append(r.newBytes, 1)); err != nil {
					t.Fatal(err)
				}
				writeArtifactFile(t, r.dir, "typhon-1.2.0-to-1.3.0.delta", other.Bytes())
				return args
			},
			wantErr: delta.ErrOutputTooLarge,
		},
		{
			name: "truncated patch",
			mutate: func(t *testing.T, r *deltaRelease, args []string) []string {
				var patch bytes.Buffer
				if err := delta.Encode(&patch, r.olds["1.2.0"], r.newBytes); err != nil {
					t.Fatal(err)
				}
				writeArtifactFile(t, r.dir, "typhon-1.2.0-to-1.3.0.delta", patch.Bytes()[:patch.Len()/2])
				return args
			},
			wantErr: delta.ErrCorrupt,
		},
		{
			name: "old executable is missing",
			mutate: func(t *testing.T, r *deltaRelease, args []string) []string {
				if err := os.Remove(r.oldPaths["1.2.0"]); err != nil {
					t.Fatal(err)
				}
				return args
			},
			anyErr: true,
		},
		{
			name: "patches without the new executable",
			mutate: func(t *testing.T, r *deltaRelease, args []string) []string {
				args[len(args)-3] = "os=windows,arch=amd64,kind=installer,path=" + r.installer
				return args
			},
			wantErr: errDeltaNoExe,
		},
		{
			name: "two artifacts with an executable",
			mutate: func(t *testing.T, r *deltaRelease, args []string) []string {
				return append(args, "-artifact", "os=windows,arch=arm64,kind=installer,path="+r.installer+",exe="+r.newExe)
			},
			wantErr: errDeltaTwoExe,
		},
		{
			name: "patch name that is not safe",
			mutate: func(t *testing.T, r *deltaRelease, args []string) []string {
				return append(args, "-delta", "from=1.0.0,old="+r.oldPaths["1.2.0"]+",patch="+r.patches["1.2.0"]+",name=../evil.delta")
			},
			wantErr: selfupdate.ErrInvalidArtifactName,
		},
		{
			name: "patch from a version that is not older",
			mutate: func(t *testing.T, r *deltaRelease, args []string) []string {
				args[len(args)-1] = "from=1.3.0,old=" + r.oldPaths["1.2.0"] + ",patch=" + r.patches["1.2.0"]
				return args
			},
			wantErr: selfupdate.ErrInvalidArtifact,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newDeltaRelease(t, "1.2.0")
			out := filepath.Join(r.dir, "manifest.json")
			args := tt.mutate(t, r, r.args(t, out, "1.2.0"))

			_, err := run(t.Context(), args, &bytes.Buffer{}, env)

			if err == nil {
				t.Fatal("run() signed a manifest it should have refused")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("run() error = %v, want %v", err, tt.wantErr)
			}
			if _, statErr := os.Stat(out); statErr == nil {
				t.Fatal("a refused release left a manifest behind")
			}
		})
	}
}

func TestParseDeltaSpec(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    deltaSpec
		wantErr bool
	}{
		{"all fields", "from=1.2.0,old=a.exe,patch=p.delta,name=x.delta", deltaSpec{"1.2.0", "a.exe", "p.delta", "x.delta"}, false},
		{"name defaults to the patch file", "from=1.2.0, old=a.exe, patch=dir/p.delta", deltaSpec{"1.2.0", "a.exe", "dir/p.delta", "p.delta"}, false},
		{"missing from", "old=a.exe,patch=p", deltaSpec{}, true},
		{"missing old", "from=1.0.0,patch=p", deltaSpec{}, true},
		{"missing patch", "from=1.0.0,old=a", deltaSpec{}, true},
		{"unknown key", "from=1.0.0,old=a,patch=p,url=x", deltaSpec{}, true},
		{"not key=value", "from", deltaSpec{}, true},
		{"empty", "", deltaSpec{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDeltaSpec(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseDeltaSpec() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Fatalf("parseDeltaSpec() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseArtifactSpecExe(t *testing.T) {
	if got, err := parseArtifactSpec("os=windows,arch=amd64,kind=installer,path=i.exe,exe=typhon.exe"); err != nil || got.exePath != "typhon.exe" {
		t.Fatalf("parseArtifactSpec() = %+v, %v, want exePath typhon.exe", got, err)
	}
	if _, err := parseArtifactSpec("os=darwin,arch=arm64,kind=bundle,path=b.zip,exe=typhon.exe"); !errors.Is(err, errExeNotInstaller) {
		t.Fatalf("parseArtifactSpec() error = %v, want errExeNotInstaller", err)
	}
}
