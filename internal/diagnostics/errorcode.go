package diagnostics

import (
	"typhon/internal/uierr"
	"typhon/internal/usagestats"
)

// A report keeps the dotted uierr code as is; usagestats.Classify folds the
// dots away for usage events, which have a narrower error_code alphabet.
func diagnosticCode(err error) string {
	code := usagestats.Class(err)
	if code == usagestats.CodeUnknown {
		if specific := uierr.Code(err); validDiagnosticCode(specific) {
			return specific
		}
	}
	return code
}
