package base

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/log"
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

// GetDurationOr takes two duration value, if the first value is greater
// than or equal to zero, then this function return this value, else the second
// value will be returned.
func GetDurationOr(timeout, defaultValue time.Duration) time.Duration {
	if timeout < 0 {
		return defaultValue
	} else {
		return timeout
	}
}

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

// logBannerMsg prints a block of message to log.
func LogBannerMsg(msgs []string, paddingLen int) {
	maxLen := 0
	for i := range msgs {
		l := len(msgs[i])
		if l > maxLen {
			maxLen = l
		}
	}

	padding := strings.Repeat(" ", paddingLen)
	stem := strings.Repeat("─", maxLen+paddingLen*2)

	log.Info("╭" + stem + "╮")
	for _, line := range msgs {
		log.Info("│" + padding + line + strings.Repeat(" ", maxLen-len(line)) + padding + " ")
	}
	log.Info("╰" + stem + "╯")
}
