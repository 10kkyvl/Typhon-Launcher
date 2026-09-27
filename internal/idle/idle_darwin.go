package idle

/*
#cgo LDFLAGS: -framework CoreGraphics
#include <CoreGraphics/CoreGraphics.h>
*/
import "C"

import "time"

// Since returns the time since the last keyboard or mouse event of the current
// session. The second value is false when the answer is unknown: a caller must
// not read that as "the user is here" or as "the user is away".
func Since() (time.Duration, bool) {
	seconds := float64(C.CGEventSourceSecondsSinceLastEventType(
		C.kCGEventSourceStateCombinedSessionState,
		C.CGEventType(C.kCGAnyInputEventType),
	))
	if seconds < 0 {
		return 0, false
	}
	return time.Duration(seconds * float64(time.Second)), true
}
