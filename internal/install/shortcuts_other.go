//go:build !windows

package install

func shortcutRoots() ([]string, error) { return nil, nil }

func sharedShortcutRoots() ([]string, error) { return nil, nil }

func systemFolders() (string, []string, error) { return "", nil, nil }
