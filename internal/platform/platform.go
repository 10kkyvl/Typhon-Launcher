package platform

import "errors"

var ErrEmptyPath = errors.New("path is empty")

// SaveRoot — каталог, под которым игры держат сохранения. Depth задаёт,
// на сколько уровней вглубь искать: 1 — только сам каталог, 2 — ещё и
// подкаталог издателя.
type SaveRoot struct {
	Path  string
	Depth int
}

type StorageInfo struct {
	Path       string `json:"path"`
	Volume     string `json:"volume"`
	Filesystem string `json:"filesystem"`
	TotalBytes uint64 `json:"totalBytes"`
	FreeBytes  uint64 `json:"freeBytes"`
	UsedBytes  uint64 `json:"usedBytes"`
}

type SystemInfo struct {
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	CPU      string `json:"cpu"`
	Cores    int    `json:"cores"`
	RAMBytes uint64 `json:"ramBytes"`
}
