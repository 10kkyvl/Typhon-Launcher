//go:build devmock && (windows || production)

package devmock

// devmockMustNotBeCompiledIntoWindowsOrProductionBuilds is intentionally
// undefined: this file must fail to compile whenever the devmock tag reaches
// a Windows or production build, independent of any Taskfile discipline.
func init() {
	devmockMustNotBeCompiledIntoWindowsOrProductionBuilds()
}
