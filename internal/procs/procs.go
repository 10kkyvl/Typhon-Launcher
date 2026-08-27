// Package procs enumerates operating system processes with their full
// image path and start time. It has no notion of games, the library, or
// any other higher-level Typhon concept — it sits below all of that.
package procs

import "time"

// Process is one entry read from the OS process table.
type Process struct {
	PID  uint32
	Path string // full image path; empty when PathUnknown
	// PathUnknown is set when the image path could not be read: no
	// permission, anti-cheat protection, or the process exited mid-scan.
	PathUnknown bool
	CreatedAt   time.Time
	// CreatedAtUnknown is set when the OS start time could not be read.
	CreatedAtUnknown bool
}
