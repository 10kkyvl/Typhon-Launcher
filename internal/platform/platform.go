package platform

import "errors"

var ErrEmptyPath = errors.New("path is empty")

var ErrNoForegroundRight = errors.New("процесс не вправе выводить окна на передний план")

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

// SystemInfo: CPUUnknown, OSUnknown and PhysicalCoresUnknown say that the
// matching field could not be read; the field then holds no usable value.
type SystemInfo struct {
	OS                   string `json:"os"`
	OSUnknown            bool   `json:"osUnknown"`
	Arch                 string `json:"arch"`
	CPU                  string `json:"cpu"`
	CPUUnknown           bool   `json:"cpuUnknown"`
	Cores                int    `json:"cores"`
	PhysicalCores        int    `json:"physicalCores"`
	PhysicalCoresUnknown bool   `json:"physicalCoresUnknown"`
	RAMBytes             uint64 `json:"ramBytes"`
}
