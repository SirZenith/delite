package base

import (
	"bufio"
	"os"
	"strings"

	"github.com/SirZenith/delite/common/html_util"
	format_common "github.com/SirZenith/delite/format/common"
	format_latex "github.com/SirZenith/delite/format/latex"
	lua_html "github.com/SirZenith/delite/lua_module/html"
	"github.com/charmbracelet/log"
	lua "github.com/yuin/gopher-lua"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func Loader(L *lua.LState) int {
	mod := L.SetFuncs(L.NewTable(), exports)

	L.Push(mod)

	return 1
}

var exports = map[string]lua.LGFunction{
	"group_children_by_file":        groupChildrenByFile,
	"delete_between_nodes":          deleteBetweenNodes,
	"replace_between_nodes":         replaceBetweenNodes,
	"replace_file_content":          replaceFileContent,
	"replace_file_content_with_toc": replaceFileContentWithToc,
	"delete_file_content":           deleteFileContent,

	"get_file_of_node":            getFileOfNode,
	"get_file_delimiting_nodes":   getFileDelimitingNodes,
	"find_node_in_file":           findNodeInFile,
	"find_all_nodes_in_file":      findAllNodesInFile,
	"iter_matching_nodes_in_file": iterMatchingNodesInFile,

	"render_node":   renderNode,
	"node_to_latex": nodeToLatex,

	"switch_handler":         switchHandler,
	"forbidden_node_cleanup": forbiddenNodeCleanup,

	"add_pagebreak_before_node": addPageBreakBeforeNode,
	"add_pagebreak_before_file": addPageBreakBeforeFile,
}

type FileRange struct {
	FileName string

	st_comment, ed_comment *html.Node
}

func getFileRange(node *html.Node, filename string) FileRange {
	result := FileRange{}

outter:
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.CommentNode {
			continue
		}

		switch {
		case strings.HasPrefix(child.Data, format_common.MetaCommentFileStart):
			name := child.Data[len(format_common.MetaCommentFileStart):]
			if name == filename {
				result.st_comment = child
			}
		case strings.HasPrefix(child.Data, format_common.MetaCommentFileEnd):
			name := child.Data[len(format_common.MetaCommentFileEnd):]
			if name == filename {
				result.ed_comment = child
				break outter
			}
		}
	}

	return result
}

func getAllFileRanges(node *html.Node) []FileRange {
	ranges := []FileRange{}

	curRange := FileRange{}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.CommentNode {
			continue
		}

		switch {
		case strings.HasPrefix(child.Data, format_common.MetaCommentFileStart):
			curRange.FileName = child.Data[len(format_common.MetaCommentFileStart):]
			curRange.st_comment = child
		case strings.HasPrefix(child.Data, format_common.MetaCommentFileEnd):
			fileName := child.Data[len(format_common.MetaCommentFileEnd):]

			if curRange.FileName != "" && curRange.FileName == fileName {
				curRange.ed_comment = child
				ranges = append(ranges, curRange)
				curRange = FileRange{}
			} else {
				curRange.FileName = ""
				curRange.st_comment = nil
			}
		}
	}

	return ranges
}

func (frange *FileRange) makeTable(L *lua.LState) *lua.LTable {
	rangeTbl := L.NewTable()
	rangeTbl.RawSet(lua.LString("filename"), lua.LString(frange.FileName))

	rangeTbl.RawSet(lua.LString("start_comment"), lua_html.NewNodeUserData(L, frange.st_comment))
	rangeTbl.RawSet(lua.LString("end_comment"), lua_html.NewNodeUserData(L, frange.ed_comment))

	return rangeTbl
}

// groupChildrenByFile groups a list of nodes into chunks by file name. A list of
// chunk table will be returned to Lua. Each chunk table has field filename,
// start_comment, end_comment, nodes.
func groupChildrenByFile(L *lua.LState) int {
	node := lua_html.CheckNode(L, 1)

	ranges := getAllFileRanges(node.Node)
	result := L.NewTable()
	for _, frange := range ranges {
		result.Append(frange.makeTable(L))
	}

	L.Push(result)

	return 1
}

// internalDeleteBetween removes all nodes in between `nodeSt` and `nodeEd`. All
// removed nodes will be wrapped with a container node and that container node
// will be returned.
func internalDeleteBetween(nodeSt, nodeEd *html.Node) *html.Node {
	container := &html.Node{
		Type: html.DocumentNode,
	}

	if parent := nodeSt.Parent; parent != nil {
		// remove existing
		sib := nodeSt.NextSibling
		for sib != nil && sib != nodeEd {
			nextSib := sib.NextSibling
			parent.RemoveChild(sib)
			container.AppendChild(sib)
			sib = nextSib
		}
	}

	return container
}

// internalReplaceBetween replace nodes between given node, and nodes given by Lua
// table, will be inserted in their place. Removed ndoe will be wrapped with a
// container node, and that container node will be returned.
func internalReplaceBetween(nodeSt, nodeEd *html.Node, replacementTbl *lua.LTable) *html.Node {
	container := internalDeleteBetween(nodeSt, nodeEd)

	// add new content
	if parent := nodeSt.Parent; parent != nil {
		totalCnt := replacementTbl.Len()
		for i := 1; i <= totalCnt; i++ {
			ud, ok := replacementTbl.RawGetInt(i).(*lua.LUserData)
			if !ok {
				continue
			}

			newNode, ok := ud.Value.(*lua_html.Node)
			if !ok {
				continue
			}

			parent.InsertBefore(newNode.Node, nodeEd)
		}
	}

	return container
}

// deleteBetweenNodes takes two nodes, and removes all nodes between them.
// Removed nodes will be wrapped with a container node and that container node
// will be returned.
func deleteBetweenNodes(L *lua.LState) int {
	nodeSt := lua_html.CheckNode(L, 1)
	nodeEd := lua_html.CheckNode(L, 2)

	container := internalDeleteBetween(nodeSt.Node, nodeEd.Node)

	return lua_html.AddNodeToState(L, container)
}

// replaceBetweenNodes takes two nodes and a list of node. Remove all nodes between
// two target nodes, and inserts content provided by node list in their place.
// Removed nodes will be wrapped with a container node, that container node will
// be returned.
func replaceBetweenNodes(L *lua.LState) int {
	nodeSt := lua_html.CheckNode(L, 1)
	nodeEd := lua_html.CheckNode(L, 2)
	replaceTbl := L.CheckTable(3)

	container := internalReplaceBetween(nodeSt.Node, nodeEd.Node, replaceTbl)

	return lua_html.AddNodeToState(L, container)
}

// replaceFileContent takes a node and a replacement table, each key in replacment
// table is a filename, and coressponding value is a list of HTML nodes.
// Node range encapusulate by specified file will wiped out and gets replaced by
// node list provided in replacement table.
// Removed nodes for each file will wrapped by a container node, and gets returned
// as a table in form of table<string, html.Node>
func replaceFileContent(L *lua.LState) int {
	node := lua_html.CheckNode(L, 1)
	replaceTbl := L.CheckTable(2)

	deletedTbl := L.NewTable()
	for _, frange := range getAllFileRanges(node.Node) {
		replacement, ok := replaceTbl.RawGetString(frange.FileName).(*lua.LTable)
		if !ok {
			continue
		}

		container := internalReplaceBetween(frange.st_comment, frange.ed_comment, replacement)
		deletedTbl.RawSetString(frange.FileName, lua_html.NewNodeUserData(L, container))
	}

	L.Push(deletedTbl)

	return 1
}

// replaceFileContentWithToc deletes content node in specified file range and
// replacing witn `\tableofcontents` and `\newpage` command.
func replaceFileContentWithToc(L *lua.LState) int {
	node := lua_html.CheckNode(L, 1)
	filename := L.CheckString(2)

	filerange := getFileRange(node.Node, filename)
	if filerange.st_comment == nil || filerange.ed_comment == nil {
		return 0
	}
	nodeEd := filerange.ed_comment
	parent := nodeEd.Parent
	if parent == nil {
		return 0
	}

	internalDeleteBetween(filerange.st_comment, filerange.ed_comment)

	parent.InsertBefore(
		&html.Node{
			Type: html.TextNode,
			Data: "\\n",
		},
		nodeEd,
	)
	parent.InsertBefore(
		&html.Node{
			Type: html.CommentNode,
			Data: format_common.MetaCommentRawText + "\\tableofcontents",
		},
		nodeEd,
	)
	parent.InsertBefore(
		&html.Node{
			Type: html.CommentNode,
			Data: format_common.MetaCommentRawText + "\\newpage",
		},
		nodeEd,
	)

	return 0
}

// deleteFileContent takes a list of filename, clear node content inside of each
// of these file ranges. Removed nodes of each file will wrapped by a container
// node, and gets returned as a table in form of table<string, html.Node>
func deleteFileContent(L *lua.LState) int {
	node := lua_html.CheckNode(L, 1)
	nameTbl := L.CheckTable(2)
	deletedTbl := L.NewTable()

	totalCnt := nameTbl.Len()
	if totalCnt <= 0 {
		L.Push(deletedTbl)
		return 1
	}

	nameSet := map[string]bool{}
	for i := 1; i <= totalCnt; i++ {
		str, ok := nameTbl.RawGetInt(i).(lua.LString)
		if !ok {
			continue
		}
		nameSet[str.String()] = true
	}

	for _, frange := range getAllFileRanges(node.Node) {
		isTarget := nameSet[frange.FileName]
		if !isTarget {
			continue
		}

		container := internalDeleteBetween(frange.st_comment, frange.ed_comment)
		deletedTbl.RawSetString(frange.FileName, lua_html.NewNodeUserData(L, container))
	}

	L.Push(deletedTbl)

	return 1

}

// getFileOfNode returns name of the file given node belongs to.
func getFileOfNode(L *lua.LState) int {
	node := lua_html.CheckNode(L, 1)

	result := lua.LNil
	walk := node.Node
	for walk != nil {
		if walk.Type == html.CommentNode && strings.HasPrefix(walk.Data, format_common.MetaCommentFileStart) {
			offset := len(format_common.MetaCommentFileStart)
			result = lua.LString(walk.Data[offset:])
			break
		} else {
			sib := walk.PrevSibling
			if sib == nil {
				walk = walk.Parent
			} else {
				walk = sib
			}
		}
	}

	L.Push(result)

	return 1
}

// getFileDelimitingNodes takes a node and target filename, tries to find starting
// and ending comment node of given file.
func getFileDelimitingNodes(L *lua.LState) int {
	node := lua_html.CheckNode(L, 1)
	filename := L.CheckString(2)

	result := getFileRange(node.Node, filename)

	if result.st_comment == nil {
		L.Push(lua.LNil)
	} else {
		L.Push(lua_html.NewNodeUserData(L, result.st_comment))
	}

	if result.ed_comment == nil {
		L.Push(lua.LNil)
	} else {
		L.Push(lua_html.NewNodeUserData(L, result.ed_comment))
	}

	return 2
}

// findNodeInFile searchs for first matching node in range of specified file.
func findNodeInFile(L *lua.LState) int {
	node := lua_html.CheckNode(L, 1)
	filename := L.CheckString(2)
	argTbl := L.CheckTable(3)

	rangeResult := getFileRange(node.Node, filename)
	if rangeResult.st_comment == nil || rangeResult.ed_comment == nil {
		return 0
	}

	args := &html_util.NodeMatchArgs{}
	lua_html.UpdateMatchingArgsFromTable(L, args, argTbl)

	sib := rangeResult.st_comment.NextSibling
	nodeEd := rangeResult.ed_comment

	var result *html.Node
	for sib != nil && sib != nodeEd {
		if html_util.CheckNodeIsMatch(sib, args) {
			result = sib
			break
		}

		args.Root = sib
		result = html_util.FindMatchingNodeDFS(sib, args)
		if result != nil {
			break
		}

		sib = sib.NextSibling
	}

	return lua_html.AddNodeToState(L, result)
}

// findAllNodesInFile searchs for all matching node in range of specified file.
func findAllNodesInFile(L *lua.LState) int {
	node := lua_html.CheckNode(L, 1)
	filename := L.CheckString(2)
	argTbl := L.CheckTable(3)

	matchTbl := L.NewTable()

	rangeResult := getFileRange(node.Node, filename)
	if rangeResult.st_comment == nil || rangeResult.ed_comment == nil {
		L.Push(matchTbl)
		return 1
	}

	args := &html_util.NodeMatchArgs{}
	lua_html.UpdateMatchingArgsFromTable(L, args, argTbl)

	sib := rangeResult.st_comment.NextSibling
	nodeEd := rangeResult.ed_comment

	for sib != nil && sib != nodeEd {
		if html_util.CheckNodeIsMatch(sib, args) {
			matchTbl.Append(lua_html.NewNodeUserData(L, sib))
		}

		args.Root = sib
		matches := html_util.FindAllMatchingNodes(sib, args)
		for _, match := range matches {
			matchTbl.Append(lua_html.NewNodeUserData(L, match))
		}

		sib = sib.NextSibling
	}

	L.Push(matchTbl)

	return 1
}

// iterMatchingNodesInFile returns iterator for iterating over all matching nodes
// in range of specified file.
func iterMatchingNodesInFile(L *lua.LState) int {
	node := lua_html.CheckNode(L, 1)
	filename := L.CheckString(2)
	argTbl := L.CheckTable(3)

	rangeResult := getFileRange(node.Node, filename)
	if rangeResult.st_comment == nil || rangeResult.ed_comment == nil {
		L.Push(L.NewFunction(func(_L *lua.LState) int {
			L.Push(lua.LNil)
			return 1
		}))
		return 1
	}

	args := &html_util.NodeMatchArgs{}
	lua_html.UpdateMatchingArgsFromTable(L, args, argTbl)

	walk := rangeResult.st_comment.NextSibling
	nodeEd := rangeResult.ed_comment
	if walk == nil || walk == nodeEd {
		L.Push(L.NewFunction(func(_L *lua.LState) int {
			L.Push(lua.LNil)
			return 1
		}))
		return 1
	}

	L.Push(L.NewFunction(func(L *lua.LState) int {
		for walk != nil && walk != nodeEd {
			var searchStart *html.Node

			if args.Root == nil {
				isMatch := html_util.CheckNodeIsMatch(walk, args)
				args.Root = walk

				if isMatch {
					return lua_html.AddNodeToState(L, walk)
				}

				searchStart = walk
			} else {
				searchStart = lua_html.CheckNode(L, 2).Node
			}

			match := html_util.FindNextMatchingNode(searchStart, args)
			if match != nil {
				args.LastMatch = match
				return lua_html.AddNodeToState(L, match)
			}

			walk = walk.NextSibling
			args.Root = nil
		}

		L.Push(lua.LNil)

		return 1
	}))
	L.Push(lua.LNil)
	L.Push(lua.LNil)

	return 3
}

// renderNode takes a file path and a Node, write content of node to file as HTML.
func renderNode(L *lua.LState) int {
	fileName := L.CheckString(1)
	if fileName == "" {
		L.ArgError(1, "file name can't not be empty string")
		return 0
	}

	node := lua_html.CheckNode(L, 2)

	file, err := os.Create(fileName)
	if err != nil {
		L.RaiseError("failed to create file %s: %s", fileName, err)
		return 0
	}
	defer file.Close()

	buf := bufio.NewWriter(file)
	defer buf.Flush()

	html.Render(buf, node.Node)

	return 0
}

// switchHandler takes a value as key and a table with handler function as its
// value. It looks for matching handler in handler table with key, and calls
// found handler with that key.
// User can use stirng `_` as key for default handler when no explict match is
// found. When no matching handler is found, it will print an warning message.
func switchHandler(L *lua.LState) int {
	value := L.Get(1)
	handlerTbl := L.CheckTable(2)

	handler, ok := handlerTbl.RawGet(value).(*lua.LFunction)
	if !ok {
		handler, ok = handlerTbl.RawGetString("_").(*lua.LFunction)
	}
	if !ok {
		log.Warnf("can't find handler for value %q", value)
		return 0
	}

	L.CallByParam(lua.P{Fn: handler}, value)

	return 0
}

// forbiddenNodeCleanup is a wrapper for ForbiddenNodeExtraction
func forbiddenNodeCleanup(L *lua.LState) int {
	node := lua_html.CheckNode(L, 1)

	forbiddenRuleMap := html_util.GetLatexStandardFrobiddenRuleMap()
	html_util.ForbiddenNodeExtraction(node.Node, forbiddenRuleMap, map[atom.Atom]int{})

	return 0
}

// nodeToLatex converts a HTML node to its latex representation.
func nodeToLatex(L *lua.LState) int {
	node := lua_html.CheckNode(L, 1)

	converterMap := format_latex.GetLatexTategakiConverter()
	content, _ := format_latex.ConvertHTML2Latex(node.Node, "", converterMap)

	var builder strings.Builder
	for ele := content.Front(); ele != nil; ele = ele.Next() {
		builder.WriteString(ele.Value.(string))
	}

	L.Push(lua.LString(builder.String()))

	return 1
}

// internalAddPageBreakBeforeNode adds newpage command raw text comment before given node
func internalAddPageBreakBeforeNode(node *html.Node) {
	parent := node.Parent
	if parent == nil {
		return
	}

	newNode := &html.Node{
		Type: html.CommentNode,
		Data: format_common.MetaCommentRawText + "\\newpage",
	}

	parent.InsertBefore(newNode, node)
}

// addPageBreakBeforeNode adds newpage command raw text comment before given node
func addPageBreakBeforeNode(L *lua.LState) int {
	node := lua_html.CheckNode(L, 1)
	internalAddPageBreakBeforeNode(node.Node)
	return 0
}

// addPageBreakBeforeFile adds newpage command raw text comment before beginning
// node of a file.
func addPageBreakBeforeFile(L *lua.LState) int {
	node := lua_html.CheckNode(L, 1)
	filename := L.CheckString(2)

	filerange := getFileRange(node.Node, filename)
	if filerange.st_comment != nil {
		internalAddPageBreakBeforeNode(filerange.st_comment)
	}

	return 0
}
