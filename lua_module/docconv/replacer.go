package docconv

import (
	"strings"

	lua "github.com/yuin/gopher-lua"
)

const ReplacerTypeName = "delite.docconv.replacer"

func RegisterReplacerType(L *lua.LState) *lua.LTable {
	mt := L.NewTypeMetatable(ReplacerTypeName)

	L.SetFuncs(mt, replacerStaticMethod)
	L.SetField(mt, "__index", L.SetFuncs(L.NewTable(), replacerMethods))

	return mt
}

func CheckReplacer(L *lua.LState, index int) *strings.Replacer {
	ud := L.CheckUserData(index)
	if v, ok := ud.Value.(*strings.Replacer); ok {
		return v
	}

	L.ArgError(index, "Replacer expected")

	return nil
}

func WrapReplacer(L *lua.LState, replacer *strings.Replacer) *lua.LUserData {
	ud := L.NewUserData()
	ud.Value = replacer

	L.SetMetatable(ud, L.GetTypeMetatable(ReplacerTypeName))

	return ud
}

func AddReplacerToState(L *lua.LState, replacer *strings.Replacer) int {
	if replacer == nil {
		L.Push(lua.LNil)
		return 1
	}

	ud := WrapReplacer(L, replacer)
	L.Push(ud)

	return 1
}

// ----------------------------------------------------------------------------

var replacerStaticMethod = map[string]lua.LGFunction{
	"new": newReplacer,
}

func newReplacer(L *lua.LState) int {
	args := []string{}
	nArgs := L.GetTop()
	for i := 1; i <= nArgs; i++ {
		arg := L.CheckString(i)
		args = append(args, arg)
	}

	if len(args)%2 != 0 {
		L.RaiseError("element count in replacement list must be event")
		return 0
	}

	replacer := strings.NewReplacer(args...)
	return AddReplacerToState(L, replacer)
}

// ----------------------------------------------------------------------------

var replacerMethods = map[string]lua.LGFunction{
	"replace": replacerReplace,
}

func replacerReplace(L *lua.LState) int {
	replacer := CheckReplacer(L, 1)
	s := L.CheckString(2)

	result := replacer.Replace(s)

	L.Push(lua.LString(result))

	return 1
}
