//go:build !windows

package tunnel

// killProcessTree is a no-op on non-Windows platforms: killing the parent
// process via os.Process.Kill() sends SIGKILL to the whole process group.
func killProcessTree(_ int) {}
