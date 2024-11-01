package html

import (
	"bytes"
	"fmt"
	"strings"

	lua "github.com/yuin/gopher-lua"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

const NodeTypeName = "html.node"

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

	L.SetField(mt, "new", L.NewFunction(newNode))
	L.SetField(mt, "new_text", L.NewFunction(newTextNode))
	L.SetField(mt, "new_doc", L.NewFunction(newDocumentNode))
	L.SetField(mt, "new_element", L.NewFunction(newElementNode))
	L.SetField(mt, "new_comment", L.NewFunction(newCommentNode))
	L.SetField(mt, "new_doctype", L.NewFunction(newDoctypeNode))
	L.SetField(mt, "new_raw", L.NewFunction(newRawNode))
	L.SetField(mt, "__index", L.SetFuncs(L.NewTable(), nodeMethods))
	L.SetField(mt, "__eq", L.NewFunction(nodeMetaEqual))
	L.SetField(mt, "__tostring", L.NewFunction(nodeMetaTostring))

	return mt
}

func NewNode(L *lua.LState, node *html.Node) *lua.LUserData {
	ud := L.NewUserData()
	ud.Value = &Node{Node: node}

	L.SetMetatable(ud, L.GetTypeMetatable(NodeTypeName))

	return ud
}

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

func newNode(L *lua.LState) int {
	node := new(html.Node)

	return addNodeToState(L, node)
}

func newTextNode(L *lua.LState) int {
	str := L.CheckString(1)
	node := &html.Node{
		Type: html.TextNode,
		Data: str,
	}

	return addNodeToState(L, node)
}

func newDocumentNode(L *lua.LState) int {
	node := &html.Node{
		Type: html.DocumentNode,
	}

	return addNodeToState(L, node)
}

func newElementNode(L *lua.LState) int {
	tag := atom.Atom(L.CheckInt64(1))
	node := &html.Node{
		Type:     html.ElementNode,
		DataAtom: tag,
		Data:     tag.String(),
	}

	return addNodeToState(L, node)
}

func newCommentNode(L *lua.LState) int {
	str := L.CheckString(1)
	node := &html.Node{
		Type: html.CommentNode,
		Data: str,
	}

	return addNodeToState(L, node)
}

func newDoctypeNode(L *lua.LState) int {
	str := L.CheckString(1)
	node := &html.Node{
		Type: html.DoctypeNode,
		Data: str,
	}

	return addNodeToState(L, node)
}

func newRawNode(L *lua.LState) int {
	str := L.CheckString(1)
	node := &html.Node{
		Type: html.RawNode,
		Data: str,
	}

	return addNodeToState(L, node)
}

func checkNode(L *lua.LState, index int) *Node {
	ud := L.CheckUserData(index)
	if v, ok := ud.Value.(*Node); ok {
		return v
	}

	L.ArgError(index, "Node expected")

	return nil
}

func nodeMetaEqual(L *lua.LState) int {
	nodeA := checkNode(L, 1)
	nodeB := checkNode(L, 2)

	if nodeA.Node == nodeB.Node {
		L.Push(lua.LTrue)
	} else {
		L.Push(lua.LFalse)
	}

	return 1
}

func nodeMetaTostring(L *lua.LState) int {
	node := checkNode(L, 1)
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

func nodeParent(L *lua.LState) int {
	node := checkNode(L, 1)
	return addNodeToState(L, node.Parent)
}

func nodeFirstChild(L *lua.LState) int {
	node := checkNode(L, 1)
	return addNodeToState(L, node.FirstChild)
}

func nodeLastChild(L *lua.LState) int {
	node := checkNode(L, 1)
	return addNodeToState(L, node.LastChild)
}

func nodePrevSibling(L *lua.LState) int {
	node := checkNode(L, 1)
	return addNodeToState(L, node.PrevSibling)
}

func nodeNextSibling(L *lua.LState) int {
	node := checkNode(L, 1)
	return addNodeToState(L, node.NextSibling)
}

func nodeGetSetType(L *lua.LState) int {
	node := checkNode(L, 1)

	if L.GetTop() == 1 {
		L.Push(lua.LNumber(node.Type))
		return 1
	}

	val := L.CheckInt64(2)
	node.Type = html.NodeType(val)

	return 0
}

func nodeGetSetDataAtom(L *lua.LState) int {
	node := checkNode(L, 1)

	if L.GetTop() == 1 {
		L.Push(lua.LNumber(node.DataAtom))
		return 1
	}

	val := L.CheckInt64(2)
	node.DataAtom = atom.Atom(val)

	return 0
}

func nodeGetSetData(L *lua.LState) int {
	node := checkNode(L, 1)

	if L.GetTop() == 1 {
		L.Push(lua.LString(node.Data))
		return 1
	}

	node.Data = L.CheckString(2)

	return 0
}

func nodeGetSetNamespace(L *lua.LState) int {
	node := checkNode(L, 1)

	if L.GetTop() == 1 {
		L.Push(lua.LString(node.Namespace))
		return 1
	}

	node.Namespace = L.CheckString(2)

	return 0
}

func nodeGetSetAttr(L *lua.LState) int {
	node := checkNode(L, 1)
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

func nodeAppendChild(L *lua.LState) int {
	node := checkNode(L, 1)
	child := checkNode(L, 2)

	node.Node.AppendChild(child.Node)

	return 0
}

func nodeInsertBefore(L *lua.LState) int {
	node := checkNode(L, 1)
	newChild := checkNode(L, 2)
	oldChild := checkNode(L, 3)

	node.Node.InsertBefore(newChild.Node, oldChild.Node)

	return 0
}

func nodeRemoveChild(L *lua.LState) int {
	node := checkNode(L, 1)
	child := checkNode(L, 2)

	node.Node.RemoveChild(child.Node)

	return 0
}

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
	node := checkNode(L, 1)

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
