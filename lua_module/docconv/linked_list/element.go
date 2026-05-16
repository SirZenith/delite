package linked_list

import (
	"container/list"
	"fmt"
	"reflect"

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
	var err error

	value := element.Value
	lValue, ok := value.(lua.LValue)
	if ok {
		return lValue, err
	}

	switch v := value.(type) {
	case int, int16, int32, int64, float32, float64:
		lValue = lua.LNumber(reflect.ValueOf(v).Convert(reflect.TypeOf(float64(0))).Float())
	case string:
		lValue = lua.LString(v)
	case bool:
		if v {
			lValue = lua.LTrue
		} else {
			lValue = lua.LFalse
		}
	default:
		err = fmt.Errorf("unsupported element value type: %T", value)
		lValue = lua.LNil
	}

	return lValue, err
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

	value, err := ElementValueToLuaValue(element)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}

	L.Push(value)

	return 1
}
