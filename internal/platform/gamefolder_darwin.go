//go:build darwin

package platform

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"typhon/internal/wine"
)

func OpenGameFolder(path, executable string) error {
	if !strings.EqualFold(filepath.Ext(executable), ".exe") {
		return OpenFolder(path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("not a folder: %s", path)
	}
	rt, err := wine.Detect()
	if err != nil {
		return err
	}
	manager := wine.NewManager(rt)
	bottle, err := manager.SharedBottle(path)
	if err != nil {
		if !wine.SharedBottleUnavailable(err) {
			return err
		}
		var ok bool
		bottle, ok = manager.Lookup(path)
		if !ok {
			return fmt.Errorf("CrossOver bottle not found for %s", path)
		}
	}
	winPath, err := bottle.ToWindows(path)
	if err != nil {
		return err
	}
	//nolint:forbidigo // standalone UI/helper operation owns its lifetime; cancellation is handled by its dialog or cancel marker.
	return manager.StartDetached(context.Background(), bottle, wine.Cmd{Path: `C:\windows\explorer.exe`, Args: []string{winPath}})
}
