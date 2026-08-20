package download

import "time"

type Status string

const (
	StatusQueued      Status = "queued"
	StatusMetadata    Status = "metadata"
	StatusDownloading Status = "downloading"
	StatusPaused      Status = "paused"
	StatusVerifying   Status = "verifying"
	StatusCompleted   Status = "completed"
	StatusFailed      Status = "failed"
)

type Type string

const TypeTorrent Type = "torrent"

type FileState struct {
	Path      string `json:"path"`
	Size      int64  `json:"size"`
	Selected  bool   `json:"selected"`
	BytesDone int64  `json:"bytesDone"`
}

type Download struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	Type          Type        `json:"type"`
	Source        string      `json:"source"`
	InfoHash      string      `json:"infoHash"`
	Destination   string      `json:"destination"`
	Status        Status      `json:"status"`
	Progress      float64     `json:"progress"`
	Downloaded    int64       `json:"downloaded"`
	Total         int64       `json:"total"`
	DownloadSpeed int64       `json:"downloadSpeed"`
	UploadSpeed   int64       `json:"uploadSpeed"`
	ETASeconds    int64       `json:"etaSeconds"`
	Seeders       int         `json:"seeders"`
	Peers         int         `json:"peers"`
	Files         []FileState `json:"files"`
	Seeding       bool        `json:"seeding"`
	AddedAt       time.Time   `json:"addedAt"`
	CompletedAt   *time.Time  `json:"completedAt"`
	Error         string      `json:"error"`
}

type TorrentInfo struct {
	InfoHash   string      `json:"infoHash"`
	Name       string      `json:"name"`
	TotalBytes int64       `json:"totalBytes"`
	Files      []FileState `json:"files"`
}

type RemovedEvent struct {
	ID string `json:"id"`
}

func occupiesSlot(s Status) bool {
	return s == StatusDownloading || s == StatusVerifying || s == StatusMetadata
}

func snapshot(d *Download) Download {
	out := *d
	out.Files = append([]FileState(nil), d.Files...)
	if d.CompletedAt != nil {
		at := *d.CompletedAt
		out.CompletedAt = &at
	}
	return out
}

func selectionOf(d *Download) []bool {
	selected := make([]bool, len(d.Files))
	for i := range d.Files {
		selected[i] = d.Files[i].Selected
	}
	return selected
}

func selectedIndices(d *Download) []int {
	indices := make([]int, 0, len(d.Files))
	for i := range d.Files {
		if d.Files[i].Selected {
			indices = append(indices, i)
		}
	}
	return indices
}

func selectedTotal(files []FileState) int64 {
	var total int64
	for _, f := range files {
		if f.Selected {
			total += f.Size
		}
	}
	return total
}

func ratio(done, total int64) float64 {
	if total <= 0 {
		return 0
	}
	if done >= total {
		return 1
	}
	return float64(done) / float64(total)
}
