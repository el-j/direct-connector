//go:build windows

package tunnel

import (
	"os/exec"
	"syscall"
)

// hideWindow prevents wsl.exe (and any other child process) from opening
// a visible console window when launched from the Wails GUI app.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}
