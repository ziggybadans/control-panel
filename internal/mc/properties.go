package mc

import (
	"strings"
)

// ParseProperties extracts key=value pairs from server.properties content.
// Values are kept verbatim (no Java-properties unescaping) so that writing
// back is bit-perfect for untouched keys.
func ParseProperties(content string) []PropEntry {
	var out []PropEntry
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "!") {
			continue
		}
		key, value, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		out = append(out, PropEntry{Key: strings.TrimSpace(key), Value: value})
	}
	return out
}

// UpdateProperties rewrites content applying changes while preserving
// comments, blank lines, and key order. Keys not present are appended.
func UpdateProperties(content string, changes map[string]string) string {
	remaining := make(map[string]string, len(changes))
	for k, v := range changes {
		remaining[k] = v
	}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "!") {
			continue
		}
		key, _, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if v, ok := remaining[key]; ok {
			lines[i] = key + "=" + v
			delete(remaining, key)
		}
	}
	// Append new keys in stable order.
	if len(remaining) > 0 {
		// Trim a single trailing empty line so appends stay adjacent.
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		for _, k := range sortedKeys(remaining) {
			lines = append(lines, k+"="+remaining[k])
		}
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// PropValue returns the value for key in content, or "".
func PropValue(content, key string) string {
	for _, e := range ParseProperties(content) {
		if e.Key == key {
			return e.Value
		}
	}
	return ""
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}
