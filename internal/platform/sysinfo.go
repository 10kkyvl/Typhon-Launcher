package platform

import (
	"runtime"
)

func baseSystemInfo() SystemInfo {
	return SystemInfo{
		OS:    runtime.GOOS,
		Arch:  runtime.GOARCH,
		CPU:   "Unknown CPU",
		Cores: runtime.NumCPU(),
	}
}
