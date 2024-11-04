package html

import (
	"bufio"
	"os"
	"strings"
	"sync"

	format_common "github.com/SirZenith/delite/format/common"
	lua "github.com/yuin/gopher-lua"
	"golang.org/x/net/html"
)

func Loader(L *lua.LState) int {
	mod := L.SetFuncs(L.NewTable(), exports)

	nodeTypeTbl := MakeNodeTypeEnum(L, mod)
	L.SetField(mod, "NodeType", nodeTypeTbl)

	nodeMt := RegisterNodeType(L)
	L.SetField(mod, "Node", nodeMt)

	L.Push(mod)

	return 1
}

var exports = map[string]lua.LGFunction{
	"group_nodes_by_file":     groupNodesByFile,
	"concate_file_range_list": concateFileRangeList,
	"render_nodes":            renderNodes,
}

var (
	nodeTypeMap     map[string]html.NodeType
	onceNodeTypeMap sync.Once
)

func MakeNodeTypeEnum(L *lua.LState, mod *lua.LTable) *lua.LTable {
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

	return tbl
}

// groupNodesByFile groups a list of nodes into chunks by file name. A list of
// chunk table will be returned to Lua. Each chunk table has field filename,
// start_comment, end_comment, nodes.
func groupNodesByFile(L *lua.LState) int {
	tbl := L.CheckTable(1)

	allNodes := []*lua.LUserData{}
	totalCnt := tbl.Len()
	for i := 1; i <= totalCnt; i++ {
		value := tbl.RawGetInt(i)

		ud, ok := value.(*lua.LUserData)
		if !ok {
			L.RaiseError("invalid element at index #%d, expecting userdata get %q", i, value.Type())
			return 0
		}

		_, ok = ud.Value.(*Node)
		if !ok {
			L.RaiseError("invalid userdata at index #%d, expecting a Node", i)
			return 0
		}

		allNodes = append(allNodes, ud)
	}

	result := groupingFileRanges(L, allNodes)

	L.Push(result)

	return 1
}

func groupingFileRanges(L *lua.LState, nodeUdList []*lua.LUserData) *lua.LTable {
	ranges := L.NewTable()

	var curFilename string
	curRange, curNodes := newFileRangeTable(L)

	for _, ud := range nodeUdList {
		node, ok := ud.Value.(*Node)
		if !ok {
			continue
		}

		switch node.Type {
		case html.CommentNode:
			switch {
			case strings.HasPrefix(node.Data, format_common.MetaCommentFileStart):
				curFilename = node.Data[len(format_common.MetaCommentFileStart):]
				curRange.RawSet(lua.LString("filename"), lua.LString(curFilename))
				curRange.RawSet(lua.LString("start_comment"), ud)
			case strings.HasPrefix(node.Data, format_common.MetaCommentFileEnd):
				curFilename = ""
				curRange.RawSet(lua.LString("end_comment"), ud)
				ranges.Append(curRange)

				curRange, curNodes = newFileRangeTable(L)
			default:
				if curFilename != "" {
					curNodes.Append(ud)
				}
			}
		default:
			if curFilename != "" {
				curNodes.Append(ud)
			}
		}
	}

	return ranges
}

func newFileRangeTable(L *lua.LState) (*lua.LTable, *lua.LTable) {
	curRange := L.NewTable()

	curNodes := L.NewTable()
	curRange.RawSet(lua.LString("nodes"), curNodes)

	return curRange, curNodes
}

// concateFileRangeList joins a list of file range table into one single list of
// nodes.
func concateFileRangeList(L *lua.LState) int {
	tbl := L.CheckTable(1)

	nodes := L.NewTable()

	totalCnt := tbl.Len()
	for i := 1; i <= totalCnt; i++ {
		value := tbl.RawGetInt(i)

		rangeTbl, ok := value.(*lua.LTable)
		if !ok {
			L.RaiseError("invalid element at index #%d, expecting table, get %q", i, value.Type())
			return 0
		}

		startComment := rangeTbl.RawGet(lua.LString("start_comment"))
		if startComment != lua.LNil {
			nodes.Append(startComment)
		}

		if children, ok := rangeTbl.RawGet(lua.LString("nodes")).(*lua.LTable); ok {
			childCnt := children.Len()
			for cIndex := 1; cIndex <= childCnt; cIndex++ {
				nodes.Append(children.RawGetInt(cIndex))
			}
		}

		endComment := rangeTbl.RawGet(lua.LString("end_comment"))
		if endComment != lua.LNil {
			nodes.Append(endComment)
		}
	}

	L.Push(nodes)

	return 1
}

func renderNodes(L *lua.LState) int {
	fileName := L.CheckString(1)
	if fileName == "" {
		return 0
	}

	nodes := []*Node{}

	list := L.CheckTable(2)
	totalCnt := list.Len()
	for i := 1; i <= totalCnt; i++ {
		value := list.RawGetInt(i)

		ud, ok := value.(*lua.LUserData)
		if !ok {
			L.RaiseError("invalid element at #%d, expecting userdata, found %q", i, value.Type())
			return 0
		}

		node, ok := ud.Value.(*Node)
		if !ok {
			L.RaiseError("invalid userdata, expecting Node")
			return 0
		}

		nodes = append(nodes, node)
	}

	file, err := os.Create(fileName)
	if err != nil {
		L.RaiseError("failed to create file %s: %s", fileName, err)
		return 0
	}
	defer file.Close()

	buf := bufio.NewWriter(file)
	defer buf.Flush()

	for _, node := range nodes {
		html.Render(buf, node.Node)
	}

	return 0
}
