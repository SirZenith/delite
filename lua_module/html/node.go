package html

import (
	"bytes"
	"fmt"
	"strings"

	lua "github.com/yuin/gopher-lua"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

const NodeTypeName = "html.Node"

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

// ----------------------------------------------------------------------------

var nodeStaticMethods = map[string]lua.LGFunction{
	"new":         newNode,
	"new_text":    newTextNode,
	"new_doc":     newDocumentNode,
	"new_element": newElementNode,
	"new_comment": newCommentNode,
	"new_doctype": newDoctypeNode,
	"new_raw":     newRawNode,
	"__eq":        nodeMetaEqual,
	"__tostring":  nodeMetaTostring,
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

	"type":      nodeGetSetType,
	"data_atom": nodeGetSetDataAtom,
	"data":      nodeGetSetData,
	"namespace": nodeGetSetNamespace,
	"attr":      nodeGetSetAttr,

	"append_child":  nodeAppendChild,
	"insert_before": nodeInsertBefore,
	"remove_child":  nodeRemoveChild,

	"iter_children": nodeIterChildren,
}

// nodeParent is gatter for Node.Parent
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

// nodeRemoveChild is a wrapper for Node.RemoveChild(child)
func nodeRemoveChild(L *lua.LState) int {
	node := CheckNode(L, 1)
	child := CheckNode(L, 2)

	node.Node.RemoveChild(child.Node)

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

func nodeFindIter(L *lua.LState) int {
	ud := L.CheckUserData(1)

	root, ok := ud.Value.(*Node)
	if !ok {
		L.ArgError(1, "Node expected")
		return 0
	}

	tag := atom.Atom(L.CheckInt64(2))
	var lastMatch *html.Node

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

		node := wrapped.Node

		match := findMatchingNodeDeepFirst(node, tag, lastMatch)
		if match != nil {
			lastMatch = node
			return addNodeToState(L, node)
		}

		if node == root.Node {
			return addNodeToState(L, nil)
		}

		// looking matches among siblings
		for sibling := node.NextSibling; sibling != nil; sibling = sibling.NextSibling {
			match = findMatchingNodeDeepFirst(sibling, tag, lastMatch)
			if match != nil {
				return addNodeToState(L, node)
			}
		}

		// step back to node parent
		parent := node.Parent
		if parent == root.Node {
			return addNodeToState(L, nil)
		}

		for sibling := parent.NextSibling; sibling != nil; sibling = sibling.NextSibling {
			match = findMatchingNodeDeepFirst(sibling, tag, lastMatch)
			if match != nil {
				return addNodeToState(L, node)
			}
		}

		return addNodeToState(L, nil)
	}))
	L.Push(lua.LNil)
	L.Push(ud)

	return 3
}

func findMatchingNodeDeepFirst(root *html.Node, tag atom.Atom, exclude *html.Node) *html.Node {
	if root.DataAtom == tag && root != exclude {
		return root
	}

	var match *html.Node
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		match = findMatchingNodeDeepFirst(child, tag, exclude)
		if match != nil {
			break
		}
	}

	return match
}
