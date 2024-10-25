package common

import (
	"path/filepath"
	"strings"
)

// Expand `target` relative to given path if its a relative path, else it will
// be returned unchanged. Empty string will be returned as empty string.
func ResolveRelativePath(target, relativeTo string) string {
	if target == "" {
		return target
	}

	if filepath.IsAbs(target) {
		return target
	}

	target = filepath.Join(relativeTo, target)
	target = filepath.Clean(target)

	return target
}

// Retuns a copy of `name` with all invalid path characters replaced.
func InvalidPathCharReplace(name string) string {
	replacer := strings.NewReplacer(
		"<", "〈",
		">", "〉",
		":", "：",
		"\"", "“",
		"/", "／",
		"\\", "＼",
		"|", "｜",
		"?", "？",
		"*", "＊",
	)

	return replacer.Replace(name)
}
