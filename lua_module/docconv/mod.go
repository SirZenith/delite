package docconv

import (
	"strings"

	format_common "github.com/SirZenith/delite/format/common"
	lua "github.com/yuin/gopher-lua"
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
