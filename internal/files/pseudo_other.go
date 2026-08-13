//go:build !linux

package files

// Real providers are Linux-only; mock roots never contain pseudo
// filesystems.
func isPseudoFS(path string) bool { return false }
