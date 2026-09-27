package diagnostics

import (
	"typhon/internal/uierr"
	"typhon/internal/usagestats"
)

func diagnosticCode(err error) string {
	code := usagestats.Classify(err)
	if code == usagestats.CodeUnknown {
		if specific := uierr.Code(err); specific != "" {
			return specific
		}
	}
	return code
}
