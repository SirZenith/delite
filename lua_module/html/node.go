package html

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/SirZenith/delite/common/html_util"
	"github.com/SirZenith/delite/format/common"
	lua "github.com/yuin/gopher-lua"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

const NodeTypeName = "delite.html.Node"

type Node struct {
	*html.Node

	attrMap map[string]*html.Attribute
}

func getQuailifiedAttrName(attr *html.Attribute) string {
	if attr.Namespace == "" {
		return attr.Key
	}
	return attr.Namespace + ":" + attr.Key
}

func splitQualifiedAttrName(name string) (string, string) {
	if !strings.Contains(name, ":") {
		return "", name
	}

	index := 0
	for i, c := range name {
		if c == ':' {
			index = i
		}
	}

	return name[:index], name[index+1:]
}

func (node *Node) initializeAttrMap() {
	if node.attrMap != nil {
		return
	}

	attrMap := map[string]*html.Attribute{}
	for i := range node.Attr {
		attr := &node.Attr[i]
		key := getQuailifiedAttrName(attr)
		attrMap[key] = attr
	}

	node.attrMap = attrMap
}

func (node *Node) getAttr(key string) *html.Attribute {
	node.initializeAttrMap()
	return node.attrMap[key]
}

func (node *Node) setAttr(key string, val string) {
	node.initializeAttrMap()

	attr := node.attrMap[key]
	if attr == nil {
		index := len(node.Attr)

		namespace, name := splitQualifiedAttrName(key)
		node.Attr = append(node.Attr, html.Attribute{
			Namespace: namespace,
			Key:       name,
		})

		attr = &node.Attr[index]
		node.attrMap[key] = attr
	}

	attr.Val = val
}

func RegisterNodeType(L *lua.LState) *lua.LTable {
	mt := L.NewTypeMetatable(NodeTypeName)

	L.SetFuncs(mt, nodeStaticMethods)
	L.SetField(mt, "__index", L.SetFuncs(L.NewTable(), nodeMethods))

	return mt
}

func CheckNode(L *lua.LState, index int) *Node {
	ud := L.CheckUserData(index)
	if v, ok := ud.Value.(*Node); ok {
		return v
	}

	L.ArgError(index, "Node expected")

	return nil
}

func WrapNode(L *lua.LState, node *Node) *lua.LUserData {
	ud := L.NewUserData()
	ud.Value = node

	L.SetMetatable(ud, L.GetTypeMetatable(NodeTypeName))

	return ud
}

func NewNodeUserData(L *lua.LState, node *html.Node) *lua.LUserData {
	return WrapNode(L, &Node{Node: node})
}

func MakeMatchingMapFromTblIntField[Num atom.Atom | html.NodeType](tbl *lua.LTable, key string) map[Num]bool {
	value := tbl.RawGetString(key)

	if num, ok := value.(lua.LNumber); ok {
		return map[Num]bool{Num(num): true}
	} else if tbl, ok := value.(*lua.LTable); ok {
		set := map[Num]bool{}

		totalCnt := tbl.Len()
		for i := 1; i <= totalCnt; i++ {
			if element, ok := tbl.RawGetInt(i).(lua.LNumber); ok {
				set[Num(element)] = true
			}
		}

		return set
	}

	return nil
}

func MakeMatchingMapFromTblStringField(tbl *lua.LTable, key string) map[string]bool {
	value := tbl.RawGetString(key)

	if str, ok := value.(lua.LString); ok {
		return map[string]bool{string(str): true}
	} else if tbl, ok := value.(*lua.LTable); ok {
		set := map[string]bool{}

		totalCnt := tbl.Len()
		for i := 1; i <= totalCnt; i++ {
			if element, ok := tbl.RawGetInt(i).(lua.LString); ok {
				set[string(element)] = true
			}
		}

		return set
	}

	return nil
}

func MakeNumberListFromMap[Num atom.Atom | html.NodeType](L *lua.LState, set map[Num]bool) *lua.LTable {
	tbl := L.NewTable()

	for key := range set {
		tbl.Append(lua.LNumber(key))
	}

	return tbl
}

func MakeStringListFromMap(L *lua.LState, set map[string]bool) *lua.LTable {
	tbl := L.NewTable()

	for key := range set {
		tbl.Append(lua.LString(key))
	}

	return tbl
}

func MatchingArgsToTable(L *lua.LState, args *html_util.NodeMatchArgs) *lua.LTable {
	tbl := L.NewTable()

	tbl.RawSetString("type", MakeNumberListFromMap(L, args.Type))
	tbl.RawSetString("tag", MakeNumberListFromMap(L, args.Tag))
	tbl.RawSetString("id", MakeStringListFromMap(L, args.Id))
	tbl.RawSetString("class", MakeStringListFromMap(L, args.Class))
	tbl.RawSetString("attr", MakeStringListFromMap(L, args.Attr))

	return tbl
}

func UpdateMatchingArgsFromTable(L *lua.LState, args *html_util.NodeMatchArgs, tbl *lua.LTable) {
	args.Type = MakeMatchingMapFromTblIntField[html.NodeType](tbl, "type")
	args.Tag = MakeMatchingMapFromTblIntField[atom.Atom](tbl, "tag")
	args.Id = MakeMatchingMapFromTblStringField(tbl, "id")
	args.Class = MakeMatchingMapFromTblStringField(tbl, "class")
	args.Attr = MakeMatchingMapFromTblStringField(tbl, "attr")

	if matchFunc, ok := tbl.RawGetString("match_func").(*lua.LFunction); ok {
		args.MatchFunc = func(node *html.Node, args *html_util.NodeMatchArgs) bool {
			L.CallByParam(
				lua.P{
					Fn:   matchFunc,
					NRet: 1,
				},
				NewNodeUserData(L, node),
				MatchingArgsToTable(L, args),
			)

			ret := L.Get(-1)
			L.Pop(1)

			if boolValue, ok := ret.(lua.LBool); ok {
				return bool(boolValue)
			}

			L.RaiseError("custom node matching function does not returns bool value")
			return false
		}
	}
}

// ----------------------------------------------------------------------------

var nodeStaticMethods = map[string]lua.LGFunction{
	"new":                        newNode,
	"new_text":                   newTextNode,
	"new_doc":                    newDocumentNode,
	"new_element":                newElementNode,
	"new_comment":                newCommentNode,
	"new_doctype":                newDoctypeNode,
	"new_raw":                    newRawNode,
	"new_raw_text_comment":       newRawTextComment,
	"new_raw_text_comment_lines": newRawTextCommentLines,
	"__eq":                       nodeMetaEqual,
	"__tostring":                 nodeMetaTostring,
}

// addNodeToState is a helper function for adding a html.Node pointer to Lua state
// as userdata. If `node` is a nil pointer, then a nil value will be passed to
// Lua.
func addNodeToState(L *lua.LState, node *html.Node) int {
	if node == nil {
		L.Push(lua.LNil)
		return 1
	}

	ud := L.NewUserData()
	ud.Value = &Node{Node: node}

	L.SetMetatable(ud, L.GetTypeMetatable(NodeTypeName))
	L.Push(ud)

	return 1
}

// newNode creates a new node in Lua.
func newNode(L *lua.LState) int {
	node := new(html.Node)

	return addNodeToState(L, node)
}

// newTextNode creates a new node of type TextNode in Lua.
func newTextNode(L *lua.LState) int {
	str := L.CheckString(1)
	node := &html.Node{
		Type: html.TextNode,
		Data: str,
	}

	return addNodeToState(L, node)
}

// newDocumentNode creates a new node of type DocumentNode in Lua.
func newDocumentNode(L *lua.LState) int {
	node := &html.Node{
		Type: html.DocumentNode,
	}

	return addNodeToState(L, node)
}

// newElementNode creates a new node of type ElementNode in Lua.
func newElementNode(L *lua.LState) int {
	tag := atom.Atom(L.CheckInt64(1))
	node := &html.Node{
		Type:     html.ElementNode,
		DataAtom: tag,
		Data:     tag.String(),
	}

	return addNodeToState(L, node)
}

// newCommentNode creates a new node of type CommentNode in Lua.
func newCommentNode(L *lua.LState) int {
	str := L.CheckString(1)
	node := &html.Node{
		Type: html.CommentNode,
		Data: str,
	}

	return addNodeToState(L, node)
}

// newDoctypeNode creates a new node of type DoctypeNode in Lua.
func newDoctypeNode(L *lua.LState) int {
	str := L.CheckString(1)
	node := &html.Node{
		Type: html.DoctypeNode,
		Data: str,
	}

	return addNodeToState(L, node)
}

// newRawNode creates a new node of type RawNode in Lua.
func newRawNode(L *lua.LState) int {
	str := L.CheckString(1)
	node := &html.Node{
		Type: html.RawNode,
		Data: str,
	}

	return addNodeToState(L, node)
}

// newRawTextComment creates raw text meta comment nodes with given content.
func newRawTextComment(L *lua.LState) int {
	str := L.CheckString(1)
	result := WrapNode(L, &Node{
		Node: &html.Node{
			Type: html.CommentNode,
			Data: common.MetaCommentRawText + string(str),
		},
	})

	L.Push(result)

	return 1
}

// newRawTextCommentLines creates a list of raw text meta comment nodes with a list
// of string lines.
func newRawTextCommentLines(L *lua.LState) int {
	tbl := L.CheckTable(1)
	result := L.NewTable()

	totalCnt := tbl.Len()
	for i := 1; i <= totalCnt; i++ {
		value := tbl.RawGetInt(i)

		str, ok := value.(lua.LString)
		if !ok {
			L.RaiseError("invalid element at index #%d, expecting string, got %q", i, value.Type())
			return 0
		}

		if i > 1 {
			result.Append(WrapNode(L, &Node{
				Node: &html.Node{
					Type: html.TextNode,
					Data: "\n",
				},
			}))
		}

		if str != "" {
			result.Append(WrapNode(L, &Node{
				Node: &html.Node{
					Type: html.CommentNode,
					Data: common.MetaCommentRawText + string(str),
				},
			}))
		}
	}

	L.Push(result)

	return 1
}

// nodeMetaEqual is __eq meta method of Node type.
func nodeMetaEqual(L *lua.LState) int {
	nodeA := CheckNode(L, 1)
	nodeB := CheckNode(L, 2)

	if nodeA.Node == nodeB.Node {
		L.Push(lua.LTrue)
	} else {
		L.Push(lua.LFalse)
	}

	return 1
}

// nodeMetaTostring is __tostring meta method of Node type.
func nodeMetaTostring(L *lua.LState) int {
	node := CheckNode(L, 1)

	writer := bytes.NewBufferString("")

	if err := html.Render(writer, node.Node); err != nil {
		msg := fmt.Sprintf("failed to reander node: %s", err)
		L.Push(lua.LString(""))
		L.Push(lua.LString(msg))

		return 2
	}

	L.Push(lua.LString(writer.String()))

	return 1
}

// ----------------------------------------------------------------------------

var nodeMethods = map[string]lua.LGFunction{
	"parent":       nodeParent,
	"first_child":  nodeFirstChild,
	"last_child":   nodeLastChild,
	"prev_sibling": nodePrevSibling,
	"next_sibling": nodeNextSibling,

	"type":       nodeGetSetType,
	"data_atom":  nodeGetSetDataAtom,
	"data":       nodeGetSetData,
	"namespace":  nodeGetSetNamespace,
	"attr":       nodeGetSetAttr,
	"change_tag": nodeChangeTag,

	"append_child":       nodeAppendChild,
	"insert_before":      nodeInsertBefore,
	"insert_after":       nodeInsertAfter,
	"remove_child":       nodeRemoveChild,
	"remove_all_child":   nodeRemoveAllChild,
	"remove_from_parent": nodeRemoveFromParent,
	"elevate_children":   nodeElevateChildren,

	"set_type_matching":      nodeSetTypeMatching,
	"set_data_atom_matching": nodeSetDataAtomMatching,
	"set_data_matching":      nodeSetDataMatching,
	"set_namespace_matching": nodeSetNamespaceMatching,
	"set_attr_matching":      nodeSetAttrMatching,
	"change_tag_matching":    nodeChangeTagMatching,
	"remove_matching":        nodeRemoveMatching,

	"iter_children": nodeIterChildren,
	"find":          nodeFind,
	"find_all":      nodeFindAll,
	"iter_match":    nodeIterMatch,

	"replace_all_text":       nodeReplaceAllText,
	"replace_all_text_regex": nodeReplaceAllTextRegex,
}

// nodeParent is getter for Node.Parent
func nodeParent(L *lua.LState) int {
	node := CheckNode(L, 1)
	return addNodeToState(L, node.Parent)
}

// nodeFirstChild is getter for Node.FirstChild
func nodeFirstChild(L *lua.LState) int {
	node := CheckNode(L, 1)
	return addNodeToState(L, node.FirstChild)
}

// nodeLastChild is getter for Node.LastChild
func nodeLastChild(L *lua.LState) int {
	node := CheckNode(L, 1)
	return addNodeToState(L, node.LastChild)
}

// nodePrevSibling is getter for Node.PrevSibling
func nodePrevSibling(L *lua.LState) int {
	node := CheckNode(L, 1)
	return addNodeToState(L, node.PrevSibling)
}

// nodeNextSibling is getter for Node.NextSibling
func nodeNextSibling(L *lua.LState) int {
	node := CheckNode(L, 1)
	return addNodeToState(L, node.NextSibling)
}

// nodeGetSetType is getter/setter for Node.Type
func nodeGetSetType(L *lua.LState) int {
	node := CheckNode(L, 1)

	if L.GetTop() == 1 {
		L.Push(lua.LNumber(node.Type))
		return 1
	}

	val := L.CheckInt64(2)
	node.Type = html.NodeType(val)

	return 0
}

// nodeGetSetDataAtom is getter/setter for Node.DataAtom
func nodeGetSetDataAtom(L *lua.LState) int {
	node := CheckNode(L, 1)

	if L.GetTop() == 1 {
		L.Push(lua.LNumber(node.DataAtom))
		return 1
	}

	val := L.CheckInt64(2)
	node.DataAtom = atom.Atom(val)

	return 0
}

// nodeGetSetData is getter/setter for Node.Data
func nodeGetSetData(L *lua.LState) int {
	node := CheckNode(L, 1)

	if L.GetTop() == 1 {
		L.Push(lua.LString(node.Data))
		return 1
	}

	node.Data = L.CheckString(2)

	return 0
}

// nodeGetSetNamespace is getter/settter for Node.Namespace
func nodeGetSetNamespace(L *lua.LState) int {
	node := CheckNode(L, 1)

	if L.GetTop() == 1 {
		L.Push(lua.LString(node.Namespace))
		return 1
	}

	node.Namespace = L.CheckString(2)

	return 0
}

// nodeGetSetAttr reads or modifies attribute of node with given name.
// When attribute with given doesn't exist yet, a new attribute entry will be
// added to node.
func nodeGetSetAttr(L *lua.LState) int {
	node := CheckNode(L, 1)
	key := L.CheckString(2)

	if L.GetTop() == 2 {
		attr := node.getAttr(key)

		if attr == nil {
			L.Push(lua.LNil)
		} else {
			L.Push(lua.LString(attr.Val))
		}

		return 1
	}

	val := L.CheckString(3)
	node.setAttr(key, val)

	return 0
}

// nodeAppendChild is a wrapper for Node.AppendChild(child)
func nodeAppendChild(L *lua.LState) int {
	node := CheckNode(L, 1)
	child := CheckNode(L, 2)

	node.Node.AppendChild(child.Node)

	return 0
}

// nodeInsertBefore is a wrapper for Node.InsertBefore(newChild, oldChild)
func nodeInsertBefore(L *lua.LState) int {
	node := CheckNode(L, 1)
	newChild := CheckNode(L, 2)
	oldChild := CheckNode(L, 3)

	node.Node.InsertBefore(newChild.Node, oldChild.Node)

	return 0
}

// nodeInsertAfter inserts a new node after given mark node
func nodeInsertAfter(L *lua.LState) int {
	node := CheckNode(L, 1)
	newChild := CheckNode(L, 2)
	oldChild := CheckNode(L, 3)

	nextSib := oldChild.NextSibling
	if nextSib == nil {
		node.Node.AppendChild(newChild.Node)
	} else {
		node.Node.InsertBefore(newChild.Node, nextSib)
	}

	return 0
}

// nodeRemoveChild is a wrapper for Node.RemoveChild(child)
func nodeRemoveChild(L *lua.LState) int {
	node := CheckNode(L, 1)
	child := CheckNode(L, 2)

	node.Node.RemoveChild(child.Node)

	return 0
}

// nodeRemoveAllChild delete all child nodes of current node.
func nodeRemoveAllChild(L *lua.LState) int {
	node := CheckNode(L, 1)

	child := node.FirstChild
	for child != nil {
		current := child
		child = child.NextSibling
		node.RemoveChild(current)
	}

	return 0
}

// nodeRemoveFromParent detaches current node from its parent. If current node
// has no parent, this function does nothing.
func nodeRemoveFromParent(L *lua.LState) int {
	node := CheckNode(L, 1)

	parent := node.Parent
	if parent != nil {
		parent.RemoveChild(node.Node)
	}

	return 0
}

// nodeElevateChildren removes all children of current node, and add then after
// current node. If current node has no parent, this function does nothing.
func nodeElevateChildren(L *lua.LState) int {
	wrapped := CheckNode(L, 1)
	node := wrapped.Node

	parent := node.Parent
	if parent == nil {
		return 0
	}

	sib := node.NextSibling
	child := node.FirstChild
	for child != nil {
		nextChild := child.NextSibling

		node.RemoveChild(child)
		if sib == nil {
			parent.AppendChild(child)
		} else {
			parent.InsertBefore(child, sib)
		}

		child = nextChild
	}

	return 0
}

// nodeChangeTag takes a atom.Atom value, changes Node.DataAtom and Node.Data
// at the same time.
func nodeChangeTag(L *lua.LState) int {
	wrapped := CheckNode(L, 1)
	num := L.CheckInt(2)

	atomValue := atom.Atom(num)
	data := atomValue.String()

	node := wrapped.Node
	node.DataAtom = atomValue
	node.Data = data

	return 0
}

// nodeSetTypeMatching sets node type for all child nodes matches given argument.
func nodeSetTypeMatching(L *lua.LState) int {
	wrapped := CheckNode(L, 1)
	num := L.CheckInt(2)
	argTbl := L.CheckTable(3)

	root := wrapped.Node
	typeValue := html.NodeType(num)

	args := &html_util.NodeMatchArgs{
		Root: root,
	}
	UpdateMatchingArgsFromTable(L, args, argTbl)

	match := html_util.FindNextMatchingNode(root, args)
	args.LastMatch = match
	for match != nil {
		match.Type = typeValue

		match = html_util.FindNextMatchingNode(match, args)
		args.LastMatch = match
	}

	return 0
}

// nodeSetDataAtomMatching sets data atom for all child nodes matches given argument.
func nodeSetDataAtomMatching(L *lua.LState) int {
	wrapped := CheckNode(L, 1)
	num := L.CheckInt(2)
	argTbl := L.CheckTable(3)

	root := wrapped.Node
	atomValue := atom.Atom(num)

	args := &html_util.NodeMatchArgs{
		Root: root,
	}
	UpdateMatchingArgsFromTable(L, args, argTbl)

	match := html_util.FindNextMatchingNode(root, args)
	args.LastMatch = match
	for match != nil {
		match.DataAtom = atomValue

		match = html_util.FindNextMatchingNode(match, args)
		args.LastMatch = match
	}

	return 0
}

// nodeSetDataMatching sets data for all child nodes matches given argument.
func nodeSetDataMatching(L *lua.LState) int {
	wrapped := CheckNode(L, 1)
	str := L.CheckString(2)
	argTbl := L.CheckTable(3)

	root := wrapped.Node

	args := &html_util.NodeMatchArgs{
		Root: root,
	}
	UpdateMatchingArgsFromTable(L, args, argTbl)

	match := html_util.FindNextMatchingNode(root, args)
	args.LastMatch = match
	for match != nil {
		match.Data = str

		match = html_util.FindNextMatchingNode(match, args)
		args.LastMatch = match
	}

	return 0
}

// nodeSetNamespaceMatching sets namespace for all child nodes matches given argument.
func nodeSetNamespaceMatching(L *lua.LState) int {
	wrapped := CheckNode(L, 1)
	str := L.CheckString(2)
	argTbl := L.CheckTable(3)

	root := wrapped.Node

	args := &html_util.NodeMatchArgs{
		Root: root,
	}
	UpdateMatchingArgsFromTable(L, args, argTbl)

	match := html_util.FindNextMatchingNode(root, args)
	args.LastMatch = match
	for match != nil {
		match.Namespace = str

		match = html_util.FindNextMatchingNode(match, args)
		args.LastMatch = match
	}

	return 0
}

// nodeSetAttrMatching sets attribution value for all child nodes matches given argument.
func nodeSetAttrMatching(L *lua.LState) int {
	wrapped := CheckNode(L, 1)
	key := L.CheckString(2)
	value := L.CheckString(3)
	argTbl := L.CheckTable(4)

	root := wrapped.Node

	args := &html_util.NodeMatchArgs{
		Root: root,
	}
	UpdateMatchingArgsFromTable(L, args, argTbl)

	match := html_util.FindNextMatchingNode(root, args)
	args.LastMatch = match
	for match != nil {
		if attr := html_util.GetNodeAttr(match, key); attr != nil {
			attr.Val = value
		} else {
			newAttr := html.Attribute{Key: key, Val: value}
			match.Attr = append(match.Attr, newAttr)
		}

		match = html_util.FindNextMatchingNode(match, args)
		args.LastMatch = match
	}

	return 0
}

// nodeChangeTagMatching changes Node.DataAtom and Node.Data value for all
// matching children and grandchildren.
func nodeChangeTagMatching(L *lua.LState) int {
	wrapped := CheckNode(L, 1)
	num := L.CheckInt(2)
	argTbl := L.CheckTable(3)

	root := wrapped.Node
	atomValue := atom.Atom(num)
	data := atomValue.String()

	args := &html_util.NodeMatchArgs{
		Root: root,
	}
	UpdateMatchingArgsFromTable(L, args, argTbl)

	match := html_util.FindNextMatchingNode(root, args)
	args.LastMatch = match
	for match != nil {
		match.DataAtom = atomValue
		match.Data = data

		match = html_util.FindNextMatchingNode(match, args)
		args.LastMatch = match
	}

	return 0
}

// nodeRemoveMatching removes all children and grandchildren from current node.
func nodeRemoveMatching(L *lua.LState) int {
	wrapped := CheckNode(L, 1)
	root := wrapped.Node

	argTbl := L.CheckTable(2)
	args := &html_util.NodeMatchArgs{
		Root: root,
	}
	UpdateMatchingArgsFromTable(L, args, argTbl)

	matches := html_util.FindAllMatchingNodes(root, args)
	for _, match := range matches {
		if parent := match.Parent; parent != nil {
			parent.RemoveChild(match)
		}
	}

	return 0
}

// nodeIterChildren returns iterator function and control variables for iterating
// over node's children. This enables `for ... in` syntax.
func nodeIterChildren(L *lua.LState) int {
	ud := L.CheckUserData(1)

	if _, ok := ud.Value.(*Node); !ok {
		L.ArgError(1, "Node expected")
		return 0
	}

	L.Push(L.NewFunction(iterNodeSibling))
	L.Push(ud)
	L.Push(lua.LNil)

	return 3
}

func iterNodeSibling(L *lua.LState) int {
	node := CheckNode(L, 1)

	value := L.Get(2)
	if value == lua.LNil {
		return addNodeToState(L, node.FirstChild)
	}

	ud, ok := value.(*lua.LUserData)
	if !ok {
		L.ArgError(2, "userdata expected")
		return 0
	}

	child, ok := ud.Value.(*Node)
	if !ok {
		L.ArgError(2, "node expected")
	}

	return addNodeToState(L, child.NextSibling)
}

// nodeFind takes a node and matching argument, finds first matching (depth first)
// among its children and grandchildren.
func nodeFind(L *lua.LState) int {
	node := CheckNode(L, 1)

	argTbl := L.CheckTable(2)
	args := &html_util.NodeMatchArgs{
		Root: node.Node,
	}
	UpdateMatchingArgsFromTable(L, args, argTbl)

	match := html_util.FindMatchingNodeDFS(node.Node, args)

	return addNodeToState(L, match)
}

// nodeFindAll takes a node and matching argument, finds all matching nodes among
// its children and grandchildren, returns all matching nodes as a list.
func nodeFindAll(L *lua.LState) int {
	wrapped := CheckNode(L, 1)
	root := wrapped.Node

	argTbl := L.CheckTable(2)
	args := &html_util.NodeMatchArgs{
		Root: root,
	}
	UpdateMatchingArgsFromTable(L, args, argTbl)

	matches := html_util.FindAllMatchingNodes(root, args)
	matchTbl := L.NewTable()
	for _, match := range matches {
		matchTbl.Append(NewNodeUserData(L, match))
	}

	L.Push(matchTbl)

	return 1
}

// nodeIterMatch makes iterator function and control variable for `for ... in`
// loop. This enables iterating over matching children of a node.
func nodeIterMatch(L *lua.LState) int {
	ud := L.CheckUserData(1)

	wrappedRoot, ok := ud.Value.(*Node)
	if !ok {
		L.ArgError(1, "Node expected")
		return 0
	}
	root := wrappedRoot.Node

	argTbl := L.CheckTable(2)
	args := &html_util.NodeMatchArgs{
		Root: root,
	}
	UpdateMatchingArgsFromTable(L, args, argTbl)

	L.Push(L.NewFunction(func(L *lua.LState) int {
		value := L.Get(2)

		ud, ok := value.(*lua.LUserData)
		if !ok {
			L.ArgError(2, "userdata expected")
			return 0
		}

		wrapped, ok := ud.Value.(*Node)
		if !ok {
			L.ArgError(2, "node expected")
		}

		match := html_util.FindNextMatchingNode(wrapped.Node, args)
		args.LastMatch = match

		return addNodeToState(L, match)
	}))
	L.Push(lua.LNil)
	L.Push(ud)

	return 3
}

// nodeReplaceAllText replaces given pattern in all text nodes with new pattern.
func nodeReplaceAllText(L *lua.LState) int {
	wrapped := CheckNode(L, 1)
	oldText := L.CheckString(2)
	newText := L.CheckString(3)

	root := wrapped.Node

	args := &html_util.NodeMatchArgs{
		Root: root,
		Type: map[html.NodeType]bool{
			html.TextNode: true,
		},
	}

	match := html_util.FindNextMatchingNode(root, args)
	args.LastMatch = match
	for match != nil {
		match.Data = strings.ReplaceAll(match.Data, oldText, newText)

		match = html_util.FindNextMatchingNode(match, args)
		args.LastMatch = match
	}

	return 0
}

// nodeReplaceAllTextRegex replaces given pattern in all text nodes with new pattern.
func nodeReplaceAllTextRegex(L *lua.LState) int {
	wrapped := CheckNode(L, 1)
	expr := L.CheckString(2)
	newText := L.CheckString(3)

	pattern, err := regexp.Compile(expr)
	if err != nil {
		L.RaiseError("invalid regular expression %q: %s", expr, err)
		return 0
	}

	root := wrapped.Node

	args := &html_util.NodeMatchArgs{
		Root: root,
		Type: map[html.NodeType]bool{
			html.TextNode: true,
		},
	}

	match := html_util.FindNextMatchingNode(root, args)
	args.LastMatch = match
	for match != nil {
		match.Data = pattern.ReplaceAllString(match.Data, newText)

		match = html_util.FindNextMatchingNode(match, args)
		args.LastMatch = match
	}

	return 0
}
