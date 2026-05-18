package converter

import (
	"container/list"
	"fmt"
	"strings"
	"unicode"

	"github.com/SirZenith/delite/common/html_util"
	lua_base_utils "github.com/SirZenith/delite/lua_module/base/utils"
	"github.com/SirZenith/delite/lua_module/docconv/linked_list"
	lua_html "github.com/SirZenith/delite/lua_module/html"
	lua "github.com/yuin/gopher-lua"
	"golang.org/x/net/html"
)

func Loader(L *lua.LState) int {
	mod := L.SetFuncs(L.NewTable(), exports)

	L.Push(mod)

	return 1
}

var exports = map[string]lua.LGFunction{
	"no_opt":                    noOptConverter,
	"drop":                      dropContentConverter,
	"replace_content":           replaceContentConverter,
	"extract_inner_text":        extractInnerTextConverter,
	"surround":                  surroundConverter,
	"surround_each_line_action": surroundEachLineConverterActionExport,
	"surround_each_line":        surroundEachLineConverter,
	"trim_space":                trimSpaceConverter,
	"trim_space_each_element":   trimSpaceEachElementConverter,
	"replace_multiple_space":    replaceMultipleSpaceConverter,
	"chain":                     chainConverter,
	"with_attr":                 withAttrConverter,
	"conditional":               conditionalConverter,
}

func readConverterArgs(L *lua.LState) (*lua_html.Node, string, *list.List) {
	node := lua_html.CheckNode(L, 1)
	contextFile := L.CheckString(2)

	lstUd := L.CheckUserData(3)
	list, ok := lstUd.Value.(*list.List)
	if !ok {
		L.ArgError(3, "List userdata expected")
		return nil, "", nil
	}

	return node, contextFile, list
}

func addConverterToState(L *lua.LState, converter func(*lua.LState) int) int {
	lFunc := L.NewFunction(converter)
	L.Push(lFunc)
	return 1
}

func ReadConverterReturns(L *lua.LState) (*list.List, bool, string, error) {
	defer L.Pop(2)

	var (
		content           *list.List
		updateContextFile bool
		contextFile       string
		err               error
	)

	newContentRet := L.Get(-2)
	if wrapped, ok := newContentRet.(*lua.LUserData); ok {
		lst, ok := wrapped.Value.(*list.List)

		if ok {
			content = lst
		} else {
			err = fmt.Errorf("expected return type LinkedList, got %T", wrapped.Value)
		}
	} else {
		err = fmt.Errorf("value returned from converter is not a userdata")
	}

	newContextFileRet := L.Get(-1)
	if wrapped, ok := newContextFileRet.(lua.LString); ok {
		updateContextFile = true
		contextFile = string(wrapped)
	}

	return content, updateContextFile, contextFile, err
}

func CallConverterFunc(L *lua.LState, converterFunc *lua.LFunction, node *html.Node, contextFile string, content *list.List) (*list.List, bool, string, error) {
	var (
		updateContextFile bool
		err               error
	)

	err = L.CallByParam(
		lua.P{
			Fn:      converterFunc,
			NRet:    2,
			Protect: true,
		},
		lua_html.NewNodeUserData(L, node),
		lua.LString(contextFile),
		linked_list.WrapList(L, content),
	)

	if err == nil {
		content, updateContextFile, contextFile, err = ReadConverterReturns(L)
	} else {
		err = fmt.Errorf("failed to run converter for tag %q: %s", node.DataAtom, err)
	}

	return content, updateContextFile, contextFile, err
}

// ----------------------------------------------------------------------------

func noOptConverter(L *lua.LState) int {
	_, _, content := readConverterArgs(L)
	return linked_list.AddListToState(L, content)
}

func dropContentConverter(L *lua.LState) int {
	_, _, content := readConverterArgs(L)
	return linked_list.AddListToState(L, content.Init())
}

func replaceContentConverter(L *lua.LState) int {
	text := L.CheckString(1)
	return addConverterToState(L, func(L *lua.LState) int {
		_, _, content := readConverterArgs(L)
		content.Init().PushBack(lua.LString(text))
		return linked_list.AddListToState(L, content)
	})
}

func extractInnerTextConverter(L *lua.LState) int {
	arg := L.Get(1)

	var replacer *strings.Replacer
	if ud, ok := arg.(*lua.LUserData); ok {
		if r, ok := ud.Value.(*strings.Replacer); ok {
			replacer = r
		}
	}

	return addConverterToState(L, func(L *lua.LState) int {
		node, _, content := readConverterArgs(L)

		content.Init()

		textList := html_util.ExtractText(node.Node)
		text := strings.Join(textList, "")
		if replacer != nil {
			text = replacer.Replace(text)
		}

		content.PushFront(lua.LString(text))

		return linked_list.AddListToState(L, content)
	})
}

func surroundConverterAction(_ *html.Node, _ string, content *list.List, left, right string) *list.List {
	if left != "" {
		content.PushFront(lua.LString(left))
	}
	if right != "" {
		content.PushBack(lua.LString(right))
	}
	return content
}

func surroundConverter(L *lua.LState) int {
	left := L.CheckString(1)
	right := L.CheckString(2)

	return addConverterToState(L, func(L *lua.LState) int {
		node, contextFile, content := readConverterArgs(L)
		content = surroundConverterAction(node.Node, contextFile, content, left, right)
		return linked_list.AddListToState(L, content)
	})
}

func surroundEachLineConverterAction(_ *html.Node, _ string, content *list.List, left, right string) *list.List {
	luaRight := lua.LString(right)
	luaLeft := lua.LString(left)
	luaLf := lua.LString("\n")

	for elem := content.Front(); elem != nil; elem = elem.Next() {
		segment, err := linked_list.ElementValueToString(elem)
		if err != nil || segment == "" {
			continue
		}

		parts := strings.Split(segment, "\n")
		elem.Value = lua.LString(parts[0])

		for i := 1; i < len(parts); i++ {
			if right != "" {
				elem = content.InsertAfter(luaRight, elem)
			}

			elem = content.InsertAfter(luaLf, elem)

			if left != "" {
				elem = content.InsertAfter(luaLeft, elem)
			}

			elem = content.InsertAfter(lua.LString(parts[i]), elem)
		}
	}

	if left != "" {
		content.PushFront(luaLeft)
	}
	if right != "" {
		content.PushBack(luaRight)
	}

	return content
}

func surroundEachLineConverterActionExport(L *lua.LState) int {
	node, contextFile, content := readConverterArgs(L)

	left := L.CheckString(4)
	right := L.CheckString(5)

	content = surroundEachLineConverterAction(node.Node, contextFile, content, left, right)

	return linked_list.AddListToState(L, content)
}

func surroundEachLineConverter(L *lua.LState) int {
	left := L.CheckString(1)
	right := L.CheckString(2)

	return addConverterToState(L, func(L *lua.LState) int {
		node, contextFile, content := readConverterArgs(L)
		content = surroundEachLineConverterAction(node.Node, contextFile, content, left, right)
		return linked_list.AddListToState(L, content)
	})
}

func trimSpaceConverter(L *lua.LState) int {
	_, _, content := readConverterArgs(L)

	if front := content.Front(); front != nil {
		text, err := linked_list.ElementValueToString(front)
		if err == nil && text != "" {
			front.Value = lua.LString(strings.TrimLeftFunc(text, unicode.IsSpace))
		}
	}
	if back := content.Back(); back != nil {
		text, err := linked_list.ElementValueToString(back)
		if err != nil && text != "" {
			back.Value = lua.LString(strings.TrimRightFunc(text, unicode.IsSpace))
		}
	}

	return linked_list.AddListToState(L, content)
}

func trimSpaceEachElementConverter(L *lua.LState) int {
	_, _, content := readConverterArgs(L)

	for elem := content.Front(); elem != nil; elem = elem.Next() {
		value, _ := linked_list.ElementValueToString(elem)
		elem.Value = lua.LString(strings.TrimSpace(value))
	}

	return linked_list.AddListToState(L, content)
}

// replaceMultipleSpaceConverter replaces multiple white spaces and new line with single space.
func replaceMultipleSpaceConverter(L *lua.LState) int {
	patt := lua_base_utils.GetMultipleWhitespacePattern()

	_, _, content := readConverterArgs(L)

	luaSpace := lua.LString(" ")

	flagPostPone := false
	for elem := content.Front(); elem != nil; elem = elem.Next() {
		segment, _ := linked_list.ElementValueToString(elem)
		if segment == "" {
			continue
		}

		parts := patt.Split(segment, -1)
		totalCnt := len(parts)

		if flagPostPone {
			content.InsertBefore(luaSpace, elem)
			flagPostPone = false
		}

		elem.Value = lua.LString(parts[0])

		for i := 1; i < totalCnt; i++ {
			if parts[i] == "" {
				flagPostPone = true
				continue
			}

			elem = content.InsertAfter(luaSpace, elem)
			flagPostPone = false

			elem = content.InsertAfter(lua.LString(parts[i]), elem)
		}
	}

	return linked_list.AddListToState(L, content)
}

func chainConverter(L *lua.LState) int {
	nArgs := L.GetTop()

	converters := []*lua.LFunction{}

	for i := 1; i <= nArgs; i++ {
		converter := L.CheckFunction(i)
		converters = append(converters, converter)
	}

	chained := func(L *lua.LState) int {
		var (
			updateContextFile    bool
			anyContextFileUpdate bool
			newContextFile       string
			err                  error
		)

		node, contextFile, content := readConverterArgs(L)
		listUd := linked_list.WrapList(L, content)

		for i, converter := range converters {
			L.CallByParam(
				lua.P{
					Fn:   converter,
					NRet: 2,
				},
				lua_html.WrapNode(L, node),
				lua.LString(contextFile),
				listUd,
			)

			content, updateContextFile, newContextFile, err = ReadConverterReturns(L)
			if err != nil {
				L.RaiseError("chain converter error (index #%d): %s", i+1, err)
			}

			listUd.Value = content

			if updateContextFile {
				anyContextFileUpdate = true
				contextFile = newContextFile
			}
		}

		retCnt := 1
		L.Push(listUd)
		if anyContextFileUpdate {
			L.Push(lua.LString(contextFile))
			retCnt++
		}

		return retCnt
	}

	L.Push(L.NewFunction(chained))

	return 1
}

func withAttrConverter(L *lua.LState) int {
	attrName := L.CheckString(1)
	converter := L.CheckFunction(2)

	return addConverterToState(L, func(L *lua.LState) int {
		var (
			updateContextFile bool
			err               error
		)

		node, contextFile, content := readConverterArgs(L)

		listUd := linked_list.WrapList(L, content)

		attr := html_util.GetNodeAttr(node.Node, attrName)
		if attr != nil {
			L.CallByParam(
				lua.P{
					Fn:   converter,
					NRet: 2,
				},
				lua_html.WrapNode(L, node),
				lua.LString(contextFile),
				listUd,
				lua.LString(attr.Val),
			)

			content, updateContextFile, contextFile, err = ReadConverterReturns(L)
			if err != nil {
				L.RaiseError("with_attr converter error: %s", err)
			}

			listUd.Value = content
		}

		retCnt := 1
		L.Push(listUd)
		if updateContextFile {
			L.Push(lua.LString(contextFile))
			retCnt++
		}

		return retCnt
	})
}

func conditionalConverter(L *lua.LState) int {
	condFunc := L.CheckFunction(1)
	converter := L.CheckFunction(2)

	return addConverterToState(L, func(L *lua.LState) int {
		var (
			updateContextFile bool
			err               error
		)

		node, contextFile, content := readConverterArgs(L)

		nodeUd := lua_html.WrapNode(L, node)
		listUd := linked_list.WrapList(L, content)

		err = L.CallByParam(
			lua.P{
				Fn:      condFunc,
				NRet:    1,
				Protect: true,
			},
			nodeUd,
			lua.LString(contextFile),
			listUd,
		)
		if err != nil {
			L.RaiseError("conditional converter conditioin check error: %s", err)
		}

		condRet := L.Get(-1)
		L.Pop(1)
		if !lua.LVIsFalse(condRet) {
			content, updateContextFile, contextFile, err = CallConverterFunc(L, converter, node.Node, contextFile, content)
			if err != nil {
				L.RaiseError("with_attr converter error: %s", err)
			}

			listUd.Value = content
		}

		retCnt := 1
		L.Push(listUd)
		if updateContextFile {
			L.Push(lua.LString(contextFile))
			retCnt++
		}

		return retCnt
	})
}
