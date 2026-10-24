package crashlog

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	HintVCRedist  = "vcredist"
	HintDirectX   = "directx"
	HintCompat    = "compat"
	HintAntivirus = "antivirus"
	HintRuntimes  = "runtimes"
	HintUnknown   = "unknown"
)

const reasonDLLNotFound = "dll_not_found"

type moduleRule struct {
	hint     string
	prefixes []string
	exact    []string
}

var moduleRules = []moduleRule{
	{hint: HintVCRedist, prefixes: []string{"msvcp", "vcruntime", "msvcr7", "msvcr8", "msvcr9", "msvcr1", "concrt", "vcomp", "mfc", "ucrtbase"}},
	{hint: HintDirectX, prefixes: []string{"d3dx9_", "d3dx10", "d3dx11", "xinput1_", "xaudio2_", "x3daudio", "d3dcompiler_4", "xactengine"}},
	{hint: HintCompat, prefixes: []string{"glide"}, exact: []string{"d3d8", "d3d9", "ddraw", "dinput"}},
}

func ClassifyModule(module string) string {
	name := strings.ToLower(strings.TrimSpace(module))
	name = strings.TrimSuffix(name, filepath.Ext(name))
	if name == "" {
		return ""
	}
	for _, r := range moduleRules {
		for _, p := range r.prefixes {
			if strings.HasPrefix(name, p) {
				return r.hint
			}
		}
		for _, e := range r.exact {
			if name == e {
				return r.hint
			}
		}
	}
	return ""
}

type DiagnoseInput struct {
	ExePath    string
	GameDir    string
	Module     string
	ModulePath string
	ReasonCode string
}

type Diagnosis struct {
	Hint        string
	MissingFile string
}

// Diagnose returns the best hint it can and an error describing the evidence
// it could not collect; a stat failure other than "not exist" is not proof of
// a deleted file and must not turn into an antivirus verdict.
func Diagnose(in DiagnoseInput) (Diagnosis, error) {
	var errs []error
	if in.ExePath != "" {
		_, err := os.Stat(in.ExePath)
		switch {
		case err == nil:
		case errors.Is(err, fs.ErrNotExist):
			return Diagnosis{Hint: HintAntivirus, MissingFile: filepath.Base(in.ExePath)}, nil
		default:
			errs = append(errs, fmt.Errorf("check game executable: %w", err))
		}
	}
	if in.ModulePath != "" && in.GameDir != "" && within(in.GameDir, in.ModulePath) {
		_, err := os.Stat(in.ModulePath)
		switch {
		case err == nil:
		case errors.Is(err, fs.ErrNotExist):
			return Diagnosis{Hint: HintAntivirus, MissingFile: cleanName(in.Module)}, nil
		default:
			errs = append(errs, fmt.Errorf("check faulting module: %w", err))
		}
	}
	if h := ClassifyModule(in.Module); h != "" {
		return Diagnosis{Hint: h}, errors.Join(errs...)
	}
	if in.Module == "" && in.ReasonCode == reasonDLLNotFound {
		return Diagnosis{Hint: HintRuntimes}, errors.Join(errs...)
	}
	return Diagnosis{Hint: HintUnknown}, errors.Join(errs...)
}

func within(dir, path string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
