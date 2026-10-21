//go:build !devmock

package settings

func configDirOverride() (string, error) {
	dir, ok, err := PortableDir()
	if err != nil {
		return "", err
	}
	if !ok {
		return "", nil
	}
	return dir, nil
}
