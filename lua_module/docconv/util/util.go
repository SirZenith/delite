package util

import (
	"container/list"
	"strings"

	format_common "github.com/SirZenith/delite/format/common"
	lua "github.com/yuin/gopher-lua"
)

func Loader(L *lua.LState) int {
	mod := L.SetFuncs(L.NewTable(), exports)

	L.Push(mod)

	return 1
}

var exports = map[string]lua.LGFunction{
	"has_prefix":             hasPrefix,
	"extract_delite_comment": extractDeliteMetaComment,
}

func hasPrefix(L *lua.LState) int {
	str := L.CheckString(1)
	prefix := L.CheckString(2)

	if strings.HasPrefix(str, prefix) {
		L.Push(lua.LTrue)
	} else {
		L.Push(lua.LFalse)
	}

	return 1
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
