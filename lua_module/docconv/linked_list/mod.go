package linked_list

import (
	lua "github.com/yuin/gopher-lua"
)

func Loader(L *lua.LState) int {
	mod := L.SetFuncs(L.NewTable(), exports)

	listMt := RegisterListType(L)
	L.SetField(mod, "List", listMt)

	elementMt := RegisterElementType(L)
	L.SetField(mod, "Element", elementMt)

	L.Push(mod)

	return 1
}

var exports = map[string]lua.LGFunction{}
