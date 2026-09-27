//go:build !darwin

package platform

// WineStatus отвечает интерфейсу на вопрос «нужен ли здесь CrossOver и есть
// ли он». Required=false значит, что и спрашивать не о чем.
type WineStatus struct {
	Required  bool   `json:"required"`
	Installed bool   `json:"installed"`
	Version   string `json:"version"`
}

func Wine() WineStatus { return WineStatus{} }
