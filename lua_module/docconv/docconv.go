package docconv

import (
	"github.com/SirZenith/delite/lua_module/docconv/linked_list"
	lua "github.com/yuin/gopher-lua"
)

func Loader(L *lua.LState) int {
	mod := L.SetFuncs(L.NewTable(), exports)

	listMt := linked_list.RegisterListType(L)
	L.SetField(mod, "List", listMt)

	elementMt := linked_list.RegisterElementType(L)
	L.SetField(mod, "Element", elementMt)

	L.Push(mod)

	return 1
}

var exports = map[string]lua.LGFunction{}
