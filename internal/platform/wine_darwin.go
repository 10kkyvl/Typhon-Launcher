//go:build darwin

package platform

import "typhon/internal/wine"

// WineStatus отвечает интерфейсу на вопрос «нужен ли здесь CrossOver и есть
// ли он». На macOS без него можно скачивать игры, но нельзя ставить и
// запускать, поэтому интерфейсу нужно уметь показать это заранее, а не
// ошибкой в момент нажатия.
type WineStatus struct {
	Required  bool   `json:"required"`
	Installed bool   `json:"installed"`
	Version   string `json:"version"`
}

func Wine() WineStatus {
	rt, err := wine.Detect()
	if err != nil {
		return WineStatus{Required: true}
	}
	return WineStatus{Required: true, Installed: true, Version: rt.Version}
}
