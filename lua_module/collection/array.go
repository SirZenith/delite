package collection

import (
	"fmt"

	lua "github.com/yuin/gopher-lua"
)

const ArrayTypeName = "Array"

func RegisterArrayType(L *lua.LState, typeName string) {
	mt := L.NewTypeMetatable(ArrayTypeName)

	L.SetField(mt, "__index", L.NewFunction(arrayMetaIndex))
	L.SetField(mt, "__newindex", L.NewFunction(arrayMetaNewindex))
	L.SetField(mt, "__len", L.NewFunction(arrayMetaLen))
}

func WrapArray(L *lua.LState, array []lua.LValue) *lua.LUserData {
	ud := L.NewUserData()
	ud.Value = array

	L.SetMetatable(ud, L.GetTypeMetatable(ArrayTypeName))

	return ud
}

func UnwrapArray[T any](value lua.LValue) ([]T, error) {
	ud, ok := value.(*lua.LUserData)
	if !ok {
		return nil, fmt.Errorf("not userdata")
	}

	array, ok := ud.Value.([]lua.LValue)
	if !ok {
		return nil, fmt.Errorf("not an array")
	}

	innerList := []T{}
	for _, element := range array {
		if inner, ok := element.(T); ok {
			innerList = append(innerList, inner)
		}
	}

	return innerList, nil
}

func checkArray(L *lua.LState, index int) []lua.LValue {
	ud := L.CheckUserData(index)
	if v, ok := ud.Value.([]lua.LValue); ok {
		return v
	}

	L.ArgError(index, "array expected")

	return nil
}

func arrayMetaIndex(L *lua.LState) int {
	array := checkArray(L, 1)
	index := L.CheckInt(2)

	if index <= 0 || index > len(array) {
		L.Push(lua.LNil)
	} else {
		L.Push(array[index-1])
	}

	return 1
}

func arrayMetaNewindex(L *lua.LState) int {
	array := checkArray(L, 1)
	index := L.CheckInt(2)
	value := L.Get(3)

	if index <= 0 || index > len(array) {
		L.RaiseError("index out of range, trying to write element %d in array of length %d", index, len(array))
	} else {
		array[index-1] = value
	}

	return 0
}

func arrayMetaLen(L *lua.LState) int {
	array := checkArray(L, 1)
	L.Push(lua.LNumber(len(array)))
	return 1
}
