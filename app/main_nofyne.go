//go:build nofyne

package main

// main_nofyne.go is compiled instead of main.go when the "nofyne" build tag is
// set.  It produces a fully headless, pure-Go, statically-linkable binary with
// no dependency on OpenGL, X11, or any display server — suitable for Docker.
//
// Build:
//   CGO_ENABLED=0 go build -tags nofyne -o direct-connector .
//
// No Fyne helpers (enableWidgets / disableWidgets) are available here because
// those are only meaningful in a GUI context; the CLI never calls them.

func main() {
	runCLI()
}
