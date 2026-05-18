package utils

import (
	"strings"

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
	"has_prefix": hasPrefix,
	"trim_space": trimSpace,
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
