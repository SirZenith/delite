package linked_list

import (
	"container/list"
	"fmt"

	lua_util "github.com/SirZenith/delite/lua_module/utils"
	lua "github.com/yuin/gopher-lua"
)

const ElementTypeName = "delite.docconv.linked_list.element"

func RegisterElementType(L *lua.LState) *lua.LTable {
	mt := L.NewTypeMetatable(ElementTypeName)

	L.SetFuncs(mt, elementStaticMethod)
	L.SetField(mt, "__index", L.SetFuncs(L.NewTable(), elementMethods))

	return mt
}

func CheckElement(L *lua.LState, index int) *list.Element {
	ud := L.CheckUserData(index)
	if v, ok := ud.Value.(*list.Element); ok {
		return v
	}

	L.ArgError(index, "Element expected")

	return nil
}

func WrapElement(L *lua.LState, element *list.Element) *lua.LUserData {
	ud := L.NewUserData()
	ud.Value = element

	L.SetMetatable(ud, L.GetTypeMetatable(ElementTypeName))

	return ud
}

func AddElementToState(L *lua.LState, element *list.Element) int {
	if element == nil {
		L.Push(lua.LNil)
		return 1
	}

	ud := WrapElement(L, element)
	L.Push(ud)

	return 1
}

func ElementValueToLuaValue(element *list.Element) (lua.LValue, error) {
	return lua_util.GoValueToLuaValue(element.Value)
}

func ElementValueToString(element *list.Element) (string, error) {
	switch v := element.Value.(type) {
	case string:
		return v, nil
	case int, int16, int32, int64, float32, float64:
		return fmt.Sprint(v), nil
	case bool:
		return fmt.Sprint(v), nil
	case lua.LValue:
		return v.String(), nil
	default:
		return "", fmt.Errorf("unsupported element value type: %T", element.Value)
	}
}

// ----------------------------------------------------------------------------

var elementStaticMethod = map[string]lua.LGFunction{
	"new":  newElement,
	"__eq": elementMetaEq,
}

func newElement(L *lua.LState) int {
	element := new(list.Element)
	return AddElementToState(L, element)
}

func elementMetaEq(L *lua.LState) int {
	elementA := CheckElement(L, 1)
	elementB := CheckElement(L, 2)

	if elementA == elementB {
		L.Push(lua.LTrue)
	} else {
		L.Push(lua.LFalse)
	}

	return 1
}

// ----------------------------------------------------------------------------

var elementMethods = map[string]lua.LGFunction{
	"next":  elementNext,
	"prev":  elementPrev,
	"value": elementValue,
}

func elementNext(L *lua.LState) int {
	element := CheckElement(L, 1)
	return AddElementToState(L, element.Next())
}

func elementPrev(L *lua.LState) int {
	element := CheckElement(L, 1)
	return AddElementToState(L, element.Prev())
}

func elementValue(L *lua.LState) int {
	element := CheckElement(L, 1)

	if L.GetTop() >= 2 {
		value := L.Get(2)
		element.Value = value
		return 0
	}

	value, err := ElementValueToLuaValue(element)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}

	L.Push(value)

	return 1
}
