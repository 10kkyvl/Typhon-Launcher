package feed

import "time"

const (
	MaxBytes         = 64 << 20
	MaxEntries       = 100000
	MaxURIsPerEntry  = 16
	MaxURILen        = 4096
	MaxTitleLen      = 512
	MinTitleLen      = 2
	MaxWarnings      = 50
	FetchTimeout     = 60 * time.Second
	MaxRedirects     = 5
	SupportedVersion = 1
)
