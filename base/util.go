package base

import (
	"path/filepath"
	"strings"
)

const FontDecypherAttr = "font-decypher"

// If given `value` is not empty, returns it. Else `defaultValue` will be returned.
func GetStrOr(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	} else {
		return value
	}
}

// Expand `target` relative to given path if its a relative path, else it will
// be returned unchanged.
func ResolveRelativePath(target, relativeTo string) string {
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
