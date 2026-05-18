package docconv

import (
	"container/list"
	"fmt"
	"strings"

	"github.com/SirZenith/delite/common/html_util"
	format_common "github.com/SirZenith/delite/format/common"
	lua_docconv_converter "github.com/SirZenith/delite/lua_module/docconv/converter"
	"github.com/SirZenith/delite/lua_module/docconv/linked_list"
	lua_html "github.com/SirZenith/delite/lua_module/html"
	"github.com/charmbracelet/log"
	lua "github.com/yuin/gopher-lua"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func Loader(L *lua.LState) int {
	mod := L.SetFuncs(L.NewTable(), exports)

	replacerMt := RegisterReplacerType(L)
	L.SetField(mod, "Replacer", replacerMt)

	L.Push(mod)

	return 1
}

var exports = map[string]lua.LGFunction{
	"extract_delite_comment": extractDeliteMetaComment,
	"extract_ruby_content":   extractRubyContent,
	"convert_html":           ConvertHtmlWithLuaConverterExport,
}

func extractDeliteMetaComment(L *lua.LState) int {
	str := L.CheckString(1)

	content := ""

	switch {
	case strings.HasPrefix(str, format_common.MetaCommentFileStart):
		content = str[len(format_common.MetaCommentFileStart):]
	case strings.HasPrefix(str, format_common.MetaCommentFileEnd):
		content = str[len(format_common.MetaCommentFileEnd):]
	case strings.HasPrefix(str, format_common.MetaCommentRefAnchor):
		content = str[len(format_common.MetaCommentRefAnchor):]
	case strings.HasPrefix(str, format_common.MetaCommentRawText):
		content = str[len(format_common.MetaCommentRawText):]
	}

	L.Push(lua.LString(content))

	return 1
}

func extractRubyContent(L *lua.LState) int {
	node := lua_html.CheckNode(L, 1)

	baseList := []string{}
	annotationList := []string{}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		switch child.Type {
		case html.TextNode:
			if child.Data != "" {
				baseList = append(baseList, child.Data)
			}
		case html.ElementNode:
			switch child.DataAtom {
			case atom.Rp:
				// ignore
			case atom.Rb:
				textList := html_util.ExtractText(child)
				text := strings.Join(textList, "")
				baseList = append(baseList, text)
			case atom.Rt:
				textList := html_util.ExtractText(child)
				text := strings.Join(textList, "")
				annotationList = append(annotationList, text)
			default:
				textList := html_util.ExtractText(child)
				text := strings.Join(textList, "")
				baseList = append(baseList, text)
			}
		default:
			// ignore
		}
	}

	baseTbl := L.NewTable()
	for index, base := range baseList {
		baseTbl.RawSetInt(index+1, lua.LString(base))
	}

	annotationTbl := L.NewTable()
	for index, annotation := range annotationList {
		annotationTbl.RawSetInt(index+1, lua.LString(annotation))
	}

	L.Push(baseTbl)
	L.Push(annotationTbl)

	return 2
}

type ConversionHandler struct {
	TextHandler              *lua.LFunction
	CommentHandler           *lua.LFunction
	ElementHandler           *lua.LTable
	ElementPreprocessHandler *lua.LTable
}

func NewConversionHandlerFromTable(L *lua.LState, tbl *lua.LTable) (*ConversionHandler, error) {
	elementHandler, _ := tbl.RawGetString("element_handler").(*lua.LTable)
	if elementHandler == nil {
		return nil, fmt.Errorf("no converter map is provided")
	}

	textHandler, _ := tbl.RawGetString("text_handler").(*lua.LFunction)
	commentHandler, _ := tbl.RawGetString("comment_handler").(*lua.LFunction)
	elementPreprocessHandler, _ := tbl.RawGetString("element_preprocess_handler").(*lua.LTable)

	result := &ConversionHandler{
		TextHandler:              textHandler,
		CommentHandler:           commentHandler,
		ElementHandler:           elementHandler,
		ElementPreprocessHandler: elementPreprocessHandler,
	}

	return result, nil
}

func ConvertHtmlWithLuaConverter(L *lua.LState, node *html.Node, contextFile string, content *list.List, meta *ConversionHandler) (*list.List, string) {
	if node.Type == html.ElementNode && html_util.CheckIsDisplayNone(node) {
		return nil, contextFile
	}

	if content == nil {
		content = list.New()
	}
	var childContent *list.List

	if node.Type == html.ElementNode {
		value := meta.ElementPreprocessHandler.RawGet(lua.LNumber(node.DataAtom))
		if lua.LVIsFalse(value) {
			value = meta.ElementPreprocessHandler.RawGet(lua.LNumber(-1))
		}

		if lua.LVIsFalse(value) {

		} else if preprocessor, ok := value.(*lua.LFunction); ok {
			err := L.CallByParam(
				lua.P{
					Fn:      preprocessor,
					NRet:    0,
					Protect: true,
				},
				lua_html.NewNodeUserData(L, node),
				lua.LString(contextFile),
			)
			if err != nil {
				log.Warnf("failed to run element preprocessor for tag %q: %s", node.DataAtom, err)
			}
		} else {
			log.Warnf("preprocessor for tag %q is not a function", node.DataAtom)
		}
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		childContent, contextFile = ConvertHtmlWithLuaConverter(L, child, contextFile, nil, meta)

		if childContent != nil {
			content.PushBackList(childContent)
		}
	}

	switch node.Type {
	case html.ErrorNode, html.DocumentNode, html.DoctypeNode:
		// pass
	case html.TextNode:
		result := node.Data
		if meta.TextHandler != nil {
			err := L.CallByParam(
				lua.P{
					Fn:      meta.TextHandler,
					NRet:    1,
					Protect: true,
				},
				lua.LString(result),
			)

			if err != nil {
				log.Warnf("failed to run text handler: %s\n\tinput: %s", err, node.Data)
			} else {
				result = L.Get(-1).String()
				L.Pop(1)
			}
		}
		content.PushBack(result)
	case html.CommentNode:
		contextFile = UpdateContextFileByCommentNode(node, contextFile)
		if meta.CommentHandler != nil {
			err := L.CallByParam(
				lua.P{
					Fn:      meta.CommentHandler,
					NRet:    1,
					Protect: true,
				},
				lua_html.NewNodeUserData(L, node),
				lua.LString(contextFile),
				linked_list.WrapList(L, content),
			)

			if err == nil {
				ret := L.Get(-1)
				L.Pop(1)

				if wrapped, ok := ret.(*lua.LUserData); ok {
					lst, ok := wrapped.Value.(*list.List)
					if ok {
						content = lst
					} else {
						log.Warnf("value returned from comment handler is not a LinkedList")
					}
				} else {
					log.Warnf("value returned from comment handler is not a userdata")
				}
			} else {
				log.Warnf("failed to run comment handler: %s\n\tinput: %s", err, node.Data)
			}
		}
	case html.ElementNode:
		converter, ok := meta.ElementHandler.RawGet(lua.LNumber(node.DataAtom)).(*lua.LFunction)
		if ok {
			newContent, updateContextFile, newContenxtFile, err := lua_docconv_converter.CallConverterFunc(L, converter, node, contextFile, content)
			if err == nil {
				if newContent != nil {
					content = newContent
				}

				if updateContextFile {
					contextFile = newContenxtFile
				}
			} else {
				log.Warnf("failed to run converter for tag %q: %s", node.DataAtom, err)
			}
		} else {
			log.Warnf("not supported tag: %q", node.Data)
		}
	}

	return content, contextFile
}

func ConvertHtmlWithLuaConverterExport(L *lua.LState) int {
	node := lua_html.CheckNode(L, 1)
	contextFile := L.CheckString(2)
	content := linked_list.CheckListOptional(L, 3)
	handler_tbl := L.CheckTable(4)

	handler, err := NewConversionHandlerFromTable(L, handler_tbl)
	if err != nil {
		L.RaiseError("invalid conversion handler: %s", err)
		return 0
	}

	content, contextFile = ConvertHtmlWithLuaConverter(L, node.Node, contextFile, content, handler)

	linked_list.AddListToState(L, content)
	L.Push(lua.LString(contextFile))

	return 2
}

func UpdateContextFileByCommentNode(node *html.Node, contextFile string) string {
	switch {
	case strings.HasPrefix(node.Data, format_common.MetaCommentFileStart):
		contextFile = node.Data[len(format_common.MetaCommentFileStart):]
	case strings.HasPrefix(node.Data, format_common.MetaCommentFileEnd):
		contextFile = ""
	default:
	}
	return contextFile
}
