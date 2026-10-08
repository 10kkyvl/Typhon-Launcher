package install

import (
	"context"
	"os"
	"testing"
)

func TestPinnedInstallerCannotBeRewrittenOrRemovedWhileHeld(t *testing.T) {
	installer := installerFixture(t, t.TempDir())
	f, _, err := pinInstaller(context.Background(), workerSpec{InstallerPath: installer, InstallerSHA256: fileDigest(t, installer)})
	if err != nil {
		t.Fatalf("pinInstaller: %v", err)
	}
	defer closePinnedInstaller(f)

	if err := os.WriteFile(installer, []byte("payload"), 0o600); err == nil {
		t.Fatal("installer rewritten while the worker holds it")
	}
	if err := os.Remove(installer); err == nil {
		t.Fatal("installer removed while the worker holds it")
	}
	if err := os.Rename(installer, installer+".old"); err == nil {
		t.Fatal("installer renamed while the worker holds it")
	}
}
