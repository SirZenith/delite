package base

import (
	"bufio"
	"os"
	"strings"

	format_common "github.com/SirZenith/delite/format/common"
	lua_html "github.com/SirZenith/delite/lua_module/html"
	"github.com/charmbracelet/log"
	lua "github.com/yuin/gopher-lua"
	"golang.org/x/net/html"
)

func Loader(L *lua.LState) int {
	mod := L.SetFuncs(L.NewTable(), exports)

	L.Push(mod)

	return 1
}

var exports = map[string]lua.LGFunction{
	"group_nodes_by_file":     groupNodesByFile,
	"concate_file_range_list": concateFileRangeList,
	"replace_file_content":    replaceFileContent,
	"render_nodes":            renderNodes,
	"switch_handler":          switchHandler,
}

type FileRange struct {
	FileName string
	st, ed   int // 1-base inclusive index of node range
}

func GetFileRanges(nodeTbl *lua.LTable) []FileRange {
	totalCnt := nodeTbl.Len()

	ranges := []FileRange{}

	curRange := FileRange{}

	for i := 1; i <= totalCnt; i++ {
		value := nodeTbl.RawGetInt(i)

		ud, ok := value.(*lua.LUserData)
		if !ok {
			continue
		}

		wrapped, ok := ud.Value.(*lua_html.Node)
		if !ok {
			continue
		}

		node := wrapped.Node
		if node.Type != html.CommentNode {
			continue
		}

		switch {
		case strings.HasPrefix(node.Data, format_common.MetaCommentFileStart):
			curRange.FileName = node.Data[len(format_common.MetaCommentFileStart):]
			curRange.st = i
		case strings.HasPrefix(node.Data, format_common.MetaCommentFileEnd):
			fileName := node.Data[len(format_common.MetaCommentFileEnd):]
			if curRange.FileName != "" && curRange.FileName == fileName {
				curRange.ed = i
				ranges = append(ranges, curRange)
				curRange = FileRange{}
			} else {
				curRange.FileName = ""
				curRange.st = 0
			}
		}
	}

	return ranges
}

func (frange *FileRange) makeTable(L *lua.LState, srcTbl *lua.LTable) *lua.LTable {
	rangeTbl := L.NewTable()
	rangeTbl.RawSet(lua.LString("filename"), lua.LString(frange.FileName))

	rangeTbl.RawSet(lua.LString("start_comment"), srcTbl.RawGetInt(frange.st))
	rangeTbl.RawSet(lua.LString("end_comment"), srcTbl.RawGetInt(frange.ed))

	nodes := L.NewTable()
	rangeTbl.RawSet(lua.LString("nodes"), nodes)
	for i := frange.st + 1; i <= frange.ed-1; i++ {
		nodes.Append(srcTbl.RawGetInt(i))
	}

	return rangeTbl
}

// groupNodesByFile groups a list of nodes into chunks by file name. A list of
// chunk table will be returned to Lua. Each chunk table has field filename,
// start_comment, end_comment, nodes.
func groupNodesByFile(L *lua.LState) int {
	nodeTbl := L.CheckTable(1)

	allNodes := []*lua.LUserData{}
	totalCnt := nodeTbl.Len()
	for i := 1; i <= totalCnt; i++ {
		value := nodeTbl.RawGetInt(i)

		ud, ok := value.(*lua.LUserData)
		if !ok {
			L.RaiseError("invalid element at index #%d, expecting userdata get %q", i, value.Type())
			return 0
		}

		_, ok = ud.Value.(*lua_html.Node)
		if !ok {
			L.RaiseError("invalid userdata at index #%d, expecting a Node", i)
			return 0
		}

		allNodes = append(allNodes, ud)
	}

	ranges := GetFileRanges(nodeTbl)
	result := L.NewTable()
	for _, frange := range ranges {
		result.Append(frange.makeTable(L, nodeTbl))
	}

	L.Push(result)

	return 1
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

// replaceFileContent takes a list of nodes and a replacement table. Replacement
// table use file names as keys, and list of replacement nodes as value.
// This function will modify original list of nodes in place, replace span of
// nodes with new nodes provided in replacement table.
func replaceFileContent(L *lua.LState) int {
	nodeTbl := L.CheckTable(1)
	replaceTbl := L.CheckTable(2)

	ranges := GetFileRanges(nodeTbl)
	for i := len(ranges) - 1; i >= 0; i-- {
		frange := ranges[i]

		replacement, ok := replaceTbl.RawGetString(frange.FileName).(*lua.LTable)
		if !ok {
			continue
		}

		replaceTblRange(nodeTbl, replacement, frange.st+1, frange.ed)
	}

	return 0
}

// replaceTblRange modifies `tbl` in place, remote all elements in range [st, ed),
// and insert array elements in `replacement` in replace of deleted elements.
// Both `st` and `ed` are 1-base Lua table index.
// Particularly, if `st` is greater than or equal to `ed`, then no element will
// be deleted, this function will go ahead to inserting step.
func replaceTblRange(tbl *lua.LTable, replacement *lua.LTable, st, ed int) {
	totalLen := tbl.Len()
	if ed > totalLen {
		ed = totalLen + 1
	}
	if ed < st {
		ed = st
	}

	deleteLen := ed - st
	insertLen := replacement.Len()

	delta := insertLen - deleteLen
	if delta > 0 {
		// expanding
		for i := totalLen; i >= ed; i-- {
			tbl.RawSetInt(i+delta, tbl.RawGetInt(i))
		}
	} else {
		// shrinking
		for i := ed; i <= totalLen; i++ {
			tbl.RawSetInt(i+delta, tbl.RawGetInt(i))
		}

		for i := 0; i > delta; i-- {
			tbl.RawSetInt(totalLen+i, lua.LNil)
		}
	}

	for i := 0; i < insertLen; i++ {
		tbl.RawSetInt(st+i, replacement.RawGetInt(i+1))
	}
}

// renderNodes takes a file path and a list of node, write content of all nodes
// into specified file as HTML.
func renderNodes(L *lua.LState) int {
	fileName := L.CheckString(1)
	if fileName == "" {
		return 0
	}

	nodes := []*lua_html.Node{}

	list := L.CheckTable(2)
	totalCnt := list.Len()
	for i := 1; i <= totalCnt; i++ {
		value := list.RawGetInt(i)

		ud, ok := value.(*lua.LUserData)
		if !ok {
			L.RaiseError("invalid element at #%d, expecting userdata, found %q", i, value.Type())
			return 0
		}

		node, ok := ud.Value.(*lua_html.Node)
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

// switchHandler takes a value as key and a table with handler function as its
// value. It looks for matching handler in handler table with key, and calls
// found handler with that key.
// When no matching handler is found, it will print an warning message.
func switchHandler(L *lua.LState) int {
	value := L.Get(1)
	handlerTbl := L.CheckTable(2)

	handler, ok := handlerTbl.RawGet(value).(*lua.LFunction)
	if !ok {
		log.Warnf("can't find handler for value %q", value)
		return 0
	}

	L.CallByParam(lua.P{Fn: handler}, value)

	return 0
}
