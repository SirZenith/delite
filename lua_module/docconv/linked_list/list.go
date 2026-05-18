package linked_list

import (
	"container/list"

	lua "github.com/yuin/gopher-lua"
)

const ListTypeName = "delite.docconv.linked_list.list"

func RegisterListType(L *lua.LState) *lua.LTable {
	mt := L.NewTypeMetatable(ListTypeName)

	L.SetFuncs(mt, listStaticMethod)
	L.SetField(mt, "__index", L.SetFuncs(L.NewTable(), listMethods))

	return mt
}

func CheckList(L *lua.LState, index int) *list.List {
	ud := L.CheckUserData(index)
	if v, ok := ud.Value.(*list.List); ok {
		return v
	}

	L.ArgError(index, "List expected")

	return nil
}

func WrapList(L *lua.LState, lst *list.List) *lua.LUserData {
	ud := L.NewUserData()
	ud.Value = lst

	L.SetMetatable(ud, L.GetTypeMetatable(ListTypeName))

	return ud
}

func AddListToState(L *lua.LState, lst *list.List) int {
	if lst == nil {
		L.Push(lua.LNil)
		return 1
	}

	ud := WrapList(L, lst)
	L.Push(ud)

	return 1
}

// ----------------------------------------------------------------------------

var listStaticMethod = map[string]lua.LGFunction{
	"new": newList,
}

func newList(L *lua.LState) int {
	lst := list.New()
	return AddListToState(L, lst)
}

// ----------------------------------------------------------------------------

var listMethods = map[string]lua.LGFunction{
	"init":    listInit,
	"totable": listToTable,

	"len":   listLen,
	"front": listFront,
	"back":  listBack,

	"prepend":      listPrepend,
	"append":       listAppend,
	"prepend_list": listPrependList,
	"append_list":  listAppendList,

	"insert_before": listInsertBefore,
	"insert_after":  listInsertAfter,

	"move_after":    listMoveAfter,
	"move_before":   listMoveBefore,
	"move_to_front": listMoveToFront,
	"move_to_back":  listMoveToBack,

	"remove": listRemove,
}

func listInit(L *lua.LState) int {
	lst := CheckList(L, 1)
	return AddListToState(L, lst.Init())
}

func listToTable(L *lua.LState) int {
	lst := CheckList(L, 1)

	tbl := L.NewTable()

	for elem := lst.Front(); elem != nil; elem = elem.Next() {
		value, err := ElementValueToLuaValue(elem)
		if err != nil {
			L.RaiseError(err.Error())
			return 0
		}
		tbl.Append(value)
	}

	L.Push(tbl)

	return 1
}

func listLen(L *lua.LState) int {
	lst := CheckList(L, 1)
	L.Push(lua.LNumber(lst.Len()))
	return 1
}

func listFront(L *lua.LState) int {
	lst := CheckList(L, 1)
	return AddElementToState(L, lst.Front())
}

func listBack(L *lua.LState) int {
	lst := CheckList(L, 1)
	return AddElementToState(L, lst.Back())
}

func listPrepend(L *lua.LState) int {
	lst := CheckList(L, 1)

	nArg := L.GetTop()
	for i := nArg; i > 1; i-- {
		value := L.CheckAny(i)
		lst.PushFront(value)
	}

	return 0
}

func listAppend(L *lua.LState) int {
	lst := CheckList(L, 1)

	nArg := L.GetTop()
	for i := 2; i <= nArg; i++ {
		value := L.CheckAny(i)
		lst.PushBack(value)
	}

	return 0
}

func listPrependList(L *lua.LState) int {
	lst := CheckList(L, 1)
	other := CheckList(L, 2)

	lst.PushFrontList(other)

	return 0
}

func listAppendList(L *lua.LState) int {
	lst := CheckList(L, 1)
	other := CheckList(L, 2)

	lst.PushBackList(other)

	return 0
}

func listInsertBefore(L *lua.LState) int {
	lst := CheckList(L, 1)
	element := CheckElement(L, 2)

	nArg := L.GetTop()
	for i := 3; i <= nArg; i++ {
		value := L.CheckAny(i)
		lst.InsertBefore(value, element)
	}

	return 0
}

func listInsertAfter(L *lua.LState) int {
	lst := CheckList(L, 1)
	element := CheckElement(L, 2)

	nArg := L.GetTop()
	for i := 3; i <= nArg; i++ {
		value := L.CheckAny(i)
		lst.InsertAfter(value, element)
	}

	return 0
}

func listMoveAfter(L *lua.LState) int {
	lst := CheckList(L, 1)
	element := CheckElement(L, 2)
	mark := CheckElement(L, 3)

	lst.MoveAfter(element, mark)

	return 0
}

func listMoveBefore(L *lua.LState) int {
	lst := CheckList(L, 1)
	element := CheckElement(L, 2)
	mark := CheckElement(L, 3)

	lst.MoveBefore(element, mark)

	return 0
}

func listMoveToFront(L *lua.LState) int {
	lst := CheckList(L, 1)
	element := CheckElement(L, 2)

	lst.MoveToFront(element)

	return 0
}

func listMoveToBack(L *lua.LState) int {
	lst := CheckList(L, 1)
	element := CheckElement(L, 2)

	lst.MoveToBack(element)

	return 0
}

func listRemove(L *lua.LState) int {
	lst := CheckList(L, 1)
	element := CheckElement(L, 2)

	lst.Remove(element)

	return 0
}
