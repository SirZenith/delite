package html

import (
	"sync"

	lua "github.com/yuin/gopher-lua"
	"golang.org/x/net/html"
)

func Loader(L *lua.LState) int {
	mod := L.SetFuncs(L.NewTable(), exports)

	registerNodeTypeEnum(L, mod)
	registerNodeType(L, mod)

	L.Push(mod)

	return 1
}

var exports = map[string]lua.LGFunction{}

var (
	nodeTypeMap     map[string]html.NodeType
	onceNodeTypeMap sync.Once
)

func registerNodeTypeEnum(L *lua.LState, mod *lua.LTable) {
	onceNodeTypeMap.Do(func() {
		nodeTypeMap = map[string]html.NodeType{
			"ErrorNode":    html.ErrorNode,
			"TextNode":     html.TextNode,
			"DocumentNode": html.DocumentNode,
			"ElementNode":  html.ElementNode,
			"CommentNode":  html.CommentNode,
			"DoctypeNode":  html.DoctypeNode,
			"RawNode":      html.RawNode,
		}
	})

	tbl := L.NewTable()
	for name, value := range nodeTypeMap {
		L.SetField(tbl, name, lua.LNumber(value))
	}

	L.SetField(mod, "NodeType", tbl)
}
