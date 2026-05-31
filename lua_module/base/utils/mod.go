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

// hasPrefix checks if given string starts with certain substring.
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

// trimSpace removes leading and trailing whitespace from input string.
func trimSpace(L *lua.LState) int {
	str := L.CheckString(1)
	trimmed := strings.TrimSpace(str)
	L.Push(lua.LString(trimmed))
	return 1
}

var (
	patternTblFormattingSlots     *regexp.Regexp
	oncePatternTblFormattingSlots sync.Once
)

// formatWithTbl takes a string and a table, replace all occurrences of `{{ key }}`
// in the target string with the corresponding value from the table, and returns
// the formatted string.
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
