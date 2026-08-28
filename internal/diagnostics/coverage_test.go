package diagnostics

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// sanitizedFields are the free-text fields sanitizeReport must scrub. Every
// one of them can carry an operating system error string, and every operating
// system error string can carry a path.
var sanitizedFields = map[string]bool{
	"Component": true,
	"Operation": true,
	"Message":   true,
	"Stack":     true,
}

// exemptFields are machine-generated and deliberately left alone, with the
// reason each one cannot carry user data. Scrubbing them is not merely
// unnecessary but wrong: a four-part AppVersion would be rewritten as an
// address by the IPv4 rule.
var exemptFields = map[string]string{
	"ErrorID":    "uuid.NewRandom per report, never derived from input",
	"AppVersion": "app.Version, fixed at build time",
	"OS":         "runtime.GOOS, a closed set",
	"Arch":       "runtime.GOARCH, a closed set",
	"ErrorCode":  "usagestats.Classify, a closed set of package constants",
}

const poison = `C:\Users\vict1m\Games\Repack\payload.exe`

const poisonMarker = "vict1m"

// A field added to Report reaches the wire through toPayload, which is a type
// conversion and so compiles silently for any new field. This test is the part
// that does not stay silent: a new string field must be added to one of the
// two maps above, and adding it to sanitizedFields without teaching
// sanitizeReport about it fails on the poison check.
func TestEveryReportFieldIsClassified(t *testing.T) {
	rt := reflect.TypeFor[Report]()
	for i := range rt.NumField() {
		f := rt.Field(i)
		t.Run(f.Name, func(t *testing.T) {
			if f.Type.Kind() != reflect.String {
				assertCannotCarryText(t, f.Name, f.Type)
				return
			}
			_, sanitized := sanitizedFields[f.Name]
			_, exempt := exemptFields[f.Name]
			switch {
			case sanitized && exempt:
				t.Fatalf("field %s is listed as both sanitized and exempt", f.Name)
			case !sanitized && !exempt:
				t.Fatalf("field %s is a new string field on Report and reaches the wire "+
					"unclassified: add it to sanitizeReport and to sanitizedFields, or to "+
					"exemptFields with the reason it cannot carry user data", f.Name)
			case exempt:
				return
			}

			in := Report{Timestamp: time.Now()}
			reflect.ValueOf(&in).Elem().Field(i).SetString(poison)
			out, err := sanitizeReport(in)
			if err != nil {
				t.Fatalf("sanitizeReport refused a scrubbable %s: %v", f.Name, err)
			}
			got := reflect.ValueOf(out).Field(i).String()
			if strings.Contains(got, poisonMarker) {
				t.Fatalf("sanitizeReport left %s unscrubbed: %q", f.Name, got)
			}
		})
	}
}

// A non-string field is only safe because its type cannot hold free text. Any
// other kind is a hole this test refuses to let through unexamined.
func assertCannotCarryText(t *testing.T, name string, typ reflect.Type) {
	t.Helper()
	switch typ.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int64, reflect.Float64:
		return
	case reflect.Struct:
		if typ == reflect.TypeFor[time.Time]() {
			return
		}
	}
	t.Fatalf("field %s has type %s, which can carry free text and is not covered "+
		"by sanitizeReport", name, typ)
}

// The poison value must actually be reducible, or the test above would pass by
// scrubbing nothing on a corpus that was never sensitive.
func TestPoisonIsActuallySensitive(t *testing.T) {
	in := Report{Timestamp: time.Now(), Message: poison, Stack: poison}
	out, err := sanitizeReport(in)
	if err != nil {
		t.Fatalf("sanitizeReport refused the poison fixture: %v", err)
	}
	if out.Message == poison || out.Stack == poison {
		t.Fatalf("the poison fixture passed through unchanged: %q / %q", out.Message, out.Stack)
	}
}
