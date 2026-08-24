// Package version contains the application identity shared by the UI and
// packaging workflow.
package version

const (
	Name        = "Nestworth"
	AppID       = "com.nestworth.app"
	Description = "A local-first personal finance desktop application for building and maintaining a personal or household balance sheet."
)

// Version and Build are variables so release packaging can stamp the exact
// version into the About window without maintaining a second code path.
var (
	Version = "v0.1.4"
	Build   = "1"
)
