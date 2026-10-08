//go:build qa && (production || !windows)

package main

// qaMustNotBeCompiledIntoProductionOrNonWindowsBuilds is intentionally
// undefined: this file must fail to compile whenever the qa tag reaches a
// production build or any OS without WebView2, independent of Taskfile
// discipline.
func init() {
	qaMustNotBeCompiledIntoProductionOrNonWindowsBuilds()
}
