package utils

import (
	"regexp"
	"strings"
	"sync"

	lua "github.com/yuin/gopher-lua"
)

func Loader(L *lua.LState) int {
	mod := L.SetFuncs(L.NewTable(), exports)

	urlMt := RegisterUrlType(L)
	L.SetField(mod, "URL", urlMt)

	L.Push(mod)

	return 1
}

var exports = map[string]lua.LGFunction{
	"has_prefix":      hasPrefix,
	"trim_space":      trimSpace,
	"format_with_tbl": formatWithTbl,
}

func hasPrefix(L *lua.LState) int {
	str := L.CheckString(1)
	prefix := L.CheckString(2)

	if strings.HasPrefix(str, prefix) {
		L.Push(lua.LTrue)
	} else {
		L.Push(lua.LFalse)
	}

	return 1
}

func trimSpace(L *lua.LState) int {
	str := L.CheckString(1)
	trimmed := strings.TrimSpace(str)
	L.Push(lua.LString(trimmed))
	return 1
}

var (
	patternMultipleWhitespace     *regexp.Regexp
	oncePatternMultipleWhitespace sync.Once
)

func GetMultipleWhitespacePattern() *regexp.Regexp {
	oncePatternMultipleWhitespace.Do(func() {
		patternMultipleWhitespace = regexp.MustCompile(`\s+`)
	})

	return patternMultipleWhitespace
}

var (
	patternTblFormattingSlots     *regexp.Regexp
	oncePatternTblFormattingSlots sync.Once
)

func formatWithTbl(L *lua.LState) int {
	oncePatternTblFormattingSlots.Do(func() {
		patternTblFormattingSlots = regexp.MustCompile(`{{\s*(\w+)\s*}}`)
	})

	str := L.CheckString(1)
	tbl := L.CheckTable(2)

	str = patternTblFormattingSlots.ReplaceAllStringFunc(str, func(match string) string {
		matches := patternTblFormattingSlots.FindStringSubmatch(match)
		if len(matches) < 1 {
			return match
		}

		value := tbl.RawGetString(matches[1])
		if lua.LVIsFalse(value) {
			return match
		}

		return value.String()
	})

	L.Push(lua.LString(str))

	return 1
}
