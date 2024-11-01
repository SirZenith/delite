package html

import (
	"bytes"
	"fmt"
	"strings"

	lua "github.com/yuin/gopher-lua"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

const luaNodeTypeName = "html.node"

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

func registerNodeType(L *lua.LState, mod *lua.LTable) {
	mt := L.NewTypeMetatable(luaNodeTypeName)

	L.SetField(mod, "Node", mt)

	L.SetField(mt, "new", L.NewFunction(newNode))
	L.SetField(mt, "new_text", L.NewFunction(newTextNode))
	L.SetField(mt, "new_doc", L.NewFunction(newDocumentNode))
	L.SetField(mt, "new_element", L.NewFunction(newElementNode))
	L.SetField(mt, "new_comment", L.NewFunction(newCommentNode))
	L.SetField(mt, "new_doctype", L.NewFunction(newDoctypeNode))
	L.SetField(mt, "new_raw", L.NewFunction(newRawNode))
	L.SetField(mt, "__index", L.SetFuncs(L.NewTable(), nodeMethods))
	L.SetField(mt, "__tostring", L.NewFunction(nodeToString))
}

func addNodeToState(L *lua.LState, node *html.Node) int {
	ud := L.NewUserData()
	ud.Value = &Node{Node: node}

	L.SetMetatable(ud, L.GetTypeMetatable(luaNodeTypeName))
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

func checkNode(L *lua.LState) *Node {
	ud := L.CheckUserData(1)
	if v, ok := ud.Value.(*Node); ok {
		return v
	}

	L.ArgError(1, "Node expected")

	return nil
}

var nodeMethods = map[string]lua.LGFunction{
	"tostring": nodeToString,
	"attr":     nodeGetSetAttr,
}

func nodeToString(L *lua.LState) int {
	node := checkNode(L)
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

func nodeGetSetAttr(L *lua.LState) int {
	node := checkNode(L)
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
