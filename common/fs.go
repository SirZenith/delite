package common

import (
	"errors"
	"fmt"
	"os"
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

// findAvailableFileName checks if given directory path does not exists yet. If
// not, it will append a numeric suffix to directory path, then check again.
// If no free diirectory name is available after `maxRetry` times of retry, an
// error will be returned.
func FindAvailableFileName(dirName, nameStem, extension string, maxRetry int) (string, error) {
	filePath := filepath.Join(dirName, nameStem+extension)

	var returnErr error

	_, err := os.Stat(filePath)
	i := 1
	for !errors.Is(err, os.ErrNotExist) {
		filePath = filepath.Join(dirName, fmt.Sprintf("%s (%d)%s", nameStem, i, extension))
		_, err = os.Stat(filePath)

		i++
		if i > maxRetry {
			returnErr = errors.New("maximum retry count reached")
			break
		}
	}

	return filePath, returnErr
}
