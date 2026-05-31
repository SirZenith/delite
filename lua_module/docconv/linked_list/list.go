package linked_list

import (
	"container/list"
	"strings"

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

func CheckListOptional(L *lua.LState, index int) *list.List {
	value := L.Get(index)
	if value == lua.LNil {
		return nil
	}

	ud, ok := value.(*lua.LUserData)
	if !ok {
		L.ArgError(index, "List expected")
		return nil
	}

	lst, ok := ud.Value.(*list.List)
	if !ok {
		L.ArgError(index, "List expected")
		return nil
	}

	return lst
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
	"new":        newList,
	"__tostring": listMetaTostring,
}

// newList makes a new linked list.
func newList(L *lua.LState) int {
	lst := list.New()
	return AddListToState(L, lst)
}

// listMetaTostring is meta method __tostring for List object.
func listMetaTostring(L *lua.LState) int {
	lst := CheckList(L, 1)

	var builder strings.Builder
	for ele := lst.Front(); ele != nil; ele = ele.Next() {
		switch v := ele.Value.(type) {
		case lua.LValue:
			builder.WriteString(v.String())
		case string:
			builder.WriteString(v)
		default:
		}
	}

	L.Push(lua.LString(builder.String()))

	return 1
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

	"all": listAll,
	"any": listAny,
}

// listInit initialize or clear the list.
func listInit(L *lua.LState) int {
	lst := CheckList(L, 1)
	return AddListToState(L, lst.Init())
}

// listToTable converts current linked list to a table.
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

// listLen returns length of current linked list.
func listLen(L *lua.LState) int {
	lst := CheckList(L, 1)
	L.Push(lua.LNumber(lst.Len()))
	return 1
}

// listFront returns frist element in linked list.
func listFront(L *lua.LState) int {
	lst := CheckList(L, 1)
	return AddElementToState(L, lst.Front())
}

// listBack returns last element in linked list.
func listBack(L *lua.LState) int {
	lst := CheckList(L, 1)
	return AddElementToState(L, lst.Back())
}

// listPrepend adds elements to the front of the list.
func listPrepend(L *lua.LState) int {
	lst := CheckList(L, 1)

	nArg := L.GetTop()
	for i := nArg; i > 1; i-- {
		value := L.CheckAny(i)
		lst.PushFront(value)
	}

	return 0
}

// listAppend adds elements to the back of the list.
func listAppend(L *lua.LState) int {
	lst := CheckList(L, 1)

	nArg := L.GetTop()
	for i := 2; i <= nArg; i++ {
		value := L.CheckAny(i)
		lst.PushBack(value)
	}

	return 0
}

// listPrependList adds all elements in other list to the front of current list.
func listPrependList(L *lua.LState) int {
	lst := CheckList(L, 1)
	other := CheckList(L, 2)

	lst.PushFrontList(other)

	return 0
}

// listAppendList adds all elements in other list to the back of current list.
func listAppendList(L *lua.LState) int {
	lst := CheckList(L, 1)
	other := CheckList(L, 2)

	lst.PushBackList(other)

	return 0
}

// listInsertBefore inserts new values before given element.
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

// listInsertAfter inserts new values after given element.
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

// listMoveAfter moves given element to the position after mark element, if element == mark or one
// of them is not an element of current list, the list will not be modified.
func listMoveAfter(L *lua.LState) int {
	lst := CheckList(L, 1)
	element := CheckElement(L, 2)
	mark := CheckElement(L, 3)

	lst.MoveAfter(element, mark)

	return 0
}

// listMoveBefore moves given element to the position before mark element, if element == mark or one
// of them is not an element of current list, the list will not be modified.
func listMoveBefore(L *lua.LState) int {
	lst := CheckList(L, 1)
	element := CheckElement(L, 2)
	mark := CheckElement(L, 3)

	lst.MoveBefore(element, mark)

	return 0
}

// listMoveToFront moves given element to the front of current list, if `element`
// is not an element of current list, the list will not be modified.
func listMoveToFront(L *lua.LState) int {
	lst := CheckList(L, 1)
	element := CheckElement(L, 2)

	lst.MoveToFront(element)

	return 0
}

// listMoveToBack moves given element to the back of current list, if `element` is
// not an element of current list, the list will not be modified.
func listMoveToBack(L *lua.LState) int {
	lst := CheckList(L, 1)
	element := CheckElement(L, 2)

	lst.MoveToBack(element)

	return 0
}

// listRemove removes an element from current list if it is an element of it.
// `element` must not be nil.
func listRemove(L *lua.LState) int {
	lst := CheckList(L, 1)
	element := CheckElement(L, 2)

	lst.Remove(element)

	return 0
}

// listAll takes a checker function, returns true if all elements in list makes
// checker function return true.
func listAll(L *lua.LState) int {
	lst := CheckList(L, 1)
	checker := L.CheckFunction(2)

	result := true
	for elem := lst.Front(); elem != nil; elem = elem.Next() {
		value, err := ElementValueToLuaValue(elem)
		if err != nil {
			L.RaiseError(err.Error())
		}

		L.CallByParam(
			lua.P{
				Fn:   checker,
				NRet: 1,
			},
			value,
		)

		ret := L.Get(-1)
		L.Pop(1)

		if lua.LVIsFalse(ret) {
			result = false
			break
		}
	}

	L.Push(lua.LBool(result))

	return 1
}

// listAny takes a checker function, returns true if any elements in list makes
// checker function return true.
func listAny(L *lua.LState) int {
	lst := CheckList(L, 1)
	checker := L.CheckFunction(2)

	result := false
	for elem := lst.Front(); elem != nil; elem = elem.Next() {
		value, err := ElementValueToLuaValue(elem)
		if err != nil {
			L.RaiseError(err.Error())
		}

		L.CallByParam(
			lua.P{
				Fn:   checker,
				NRet: 1,
			},
			value,
		)

		ret := L.Get(-1)
		L.Pop(1)

		if !lua.LVIsFalse(ret) {
			result = true
			break
		}
	}

	L.Push(lua.LBool(result))

	return 1
}
