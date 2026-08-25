package discovery

type Result struct {
	Roots        int       `json:"roots"`
	RootsSkipped int       `json:"rootsSkipped"`
	Candidates   int       `json:"candidates"`
	Added        int       `json:"added"`
	Updated      int       `json:"updated"`
	Known        int       `json:"known"`
	Skipped      int       `json:"skipped"`
	Errors       int       `json:"errors"`
	Cancelled    bool      `json:"cancelled"`
	Problems     []Problem `json:"problems,omitempty"`
}

type Problem struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type Progress struct {
	Processed int `json:"processed"`
	Total     int `json:"total"`
}

const maxProblems = 32

func (r *Result) fail(path, reason string) {
	r.Errors++
	r.note(path, reason)
}

func (r *Result) note(path, reason string) {
	if len(r.Problems) >= maxProblems {
		return
	}
	r.Problems = append(r.Problems, Problem{Path: path, Reason: reason})
}
