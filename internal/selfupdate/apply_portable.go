//go:build windows || devmock || (darwin && !devmock)

package selfupdate

import (
	"context"

	"typhon/internal/settings"
)

// ApplyPortable is the portable counterpart of Apply: it never starts an
// installer, so a portable folder cannot end up with files written elsewhere.
func ApplyPortable(ctx context.Context, zipPath, target string) error {
	dir, err := settings.ConfigDir()
	if err != nil {
		return err
	}
	if err := validateInstallerPath(dir, zipPath); err != nil {
		return err
	}
	store, err := NewStore(dir)
	if err != nil {
		return err
	}
	st, err := store.Load()
	if err != nil {
		return err
	}
	if st.Artifact == nil || st.Artifact.Kind != KindPortable || st.ReadyPath == "" || st.ReadyPath != zipPath {
		return ErrNotReady
	}
	if err := VerifyFile(ctx, zipPath, *st.Artifact); err != nil {
		return err
	}
	return applyPortableZip(ctx, zipPath, target)
}
