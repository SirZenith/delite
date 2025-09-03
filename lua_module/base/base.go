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
	"group_children_by_file": groupChildrenByFile,
	"replace_file_content":   replaceFileContent,
	"render_node":            renderNode,
	"switch_handler":         switchHandler,
	"forbidden_node_cleanup": forbiddenNodeCleanup,
	"node_to_latex":          nodeToLatex,
}

type FileRange struct {
	FileName string

	st_comment, ed_comment *html.Node
}

func GetFileRanges(node *html.Node) []FileRange {
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

	ranges := GetFileRanges(node.Node)
	result := L.NewTable()
	for _, frange := range ranges {
		result.Append(frange.makeTable(L))
	}

	L.Push(result)

	return 1
}

// replaceFileContent takes a node and a replacement table, each key in replacment
// table is a filename, and coressponding value is a list of HTML nodes.
// Node range encapusulate by specified file will wiped out and gets replaced by
// node list provided in replacement table.
// Removed children will be returned in form of table<string, html.Node[]>.
func replaceFileContent(L *lua.LState) int {
	node := lua_html.CheckNode(L, 1)
	replaceTbl := L.CheckTable(2)

	deletedTbl := L.NewTable()
	for _, frange := range GetFileRanges(node.Node) {
		replacement, ok := replaceTbl.RawGetString(frange.FileName).(*lua.LTable)
		if !ok {
			continue
		}

		container := &html.Node{
			Type: html.DocumentNode,
		}

		if parent := frange.st_comment.Parent; parent != nil {
			// remove existing
			sib := frange.st_comment.NextSibling
			for sib != nil && sib != frange.ed_comment {
				nextSib := sib.NextSibling
				parent.RemoveChild(sib)
				container.AppendChild(sib)
				sib = nextSib
			}

			// add new content
			totalCnt := replacement.Len()
			for i := 1; i <= totalCnt; i++ {
				ud, ok := replacement.RawGetInt(i).(*lua.LUserData)
				if !ok {
					continue
				}

				newNode, ok := ud.Value.(*lua_html.Node)
				if !ok {
					continue
				}

				parent.InsertBefore(newNode.Node, frange.ed_comment)
			}
		}

		deletedTbl.RawSetString(frange.FileName, lua_html.NewNodeUserData(L, container))
	}

	L.Push(deletedTbl)

	return 1
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
