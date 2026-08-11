package metrics

import (
	"runtime"
	"sort"
)

func goArch() string { return runtime.GOARCH }

// deltaRate computes a per-second rate from two counter readings, guarding
// against counter resets (reboot, driver reload).
func deltaRate(cur, prev uint64, elapsedSec float64) float64 {
	if cur < prev || elapsedSec <= 0 {
		return 0
	}
	return float64(cur-prev) / elapsedSec
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
