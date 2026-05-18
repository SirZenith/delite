package docconv

import (
	"strings"

	"github.com/SirZenith/delite/common/html_util"
	format_common "github.com/SirZenith/delite/format/common"
	lua_html "github.com/SirZenith/delite/lua_module/html"
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
