package log

import (
	"github.com/charmbracelet/log"
	lua "github.com/yuin/gopher-lua"
)

func Loader(L *lua.LState) int {
	mod := L.SetFuncs(L.NewTable(), exports)

	L.Push(mod)

	return 1
}

var exports = map[string]lua.LGFunction{
	"debug": logWithFunc(log.Debug),
	"info":  logWithFunc(log.Info),
	"warn":  logWithFunc(log.Warn),
	"error": logWithFunc(log.Error),
	"fatal": logWithFunc(log.Fatal),
}

// logWithFunc returns a closure that that writes log with wrapped function.
func logWithFunc(action func(msg interface{}, keyvals ...interface{})) func(L *lua.LState) int {
	return func(L *lua.LState) int {
		nArg := L.GetTop()

		msg := L.Get(1)

		pairList := make([]any, 0, nArg)
		for i := 2; i <= nArg; i++ {
			value := L.Get(i)
			pairList = append(pairList, value)
		}

		action(msg, pairList...)

		return 0
	}
}
