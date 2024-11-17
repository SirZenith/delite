package fs

import (
	"os"
	"path/filepath"

	lua "github.com/yuin/gopher-lua"
)

func Loader(L *lua.LState) int {
	mod := L.SetFuncs(L.NewTable(), exports)

	L.Push(mod)

	return 1
}

var exports = map[string]lua.LGFunction{
	"mkdir":     mkdir,
	"mkdir_all": mkdirAll,
	"link":      link,
	"symlink":   symlink,

	"join":      join,
	"split":     split,
	"split_ext": splitExt,
	"dirname":   dirname,
	"basename":  basename,
	"ext":       ext,
}

func mkdir(L *lua.LState) int {
	path := L.CheckString(1)

	if err := os.Mkdir(path, 0o777); err == nil {
		L.Push(lua.LNil)
	} else {
		L.Push(lua.LString(err.Error()))
	}

	return 1
}

func mkdirAll(L *lua.LState) int {
	path := L.CheckString(1)

	if err := os.MkdirAll(path, 0o777); err == nil {
		L.Push(lua.LNil)
	} else {
		L.Push(lua.LString(err.Error()))
	}

	return 1
}

func link(L *lua.LState) int {
	oldname := L.CheckString(1)
	newname := L.CheckString(2)

	if err := os.Link(oldname, newname); err == nil {
		L.Push(lua.LNil)
	} else {
		L.Push(lua.LString(err.Error()))
	}

	return 1
}

func symlink(L *lua.LState) int {
	oldname := L.CheckString(1)
	newname := L.CheckString(2)

	if err := os.Symlink(oldname, newname); err == nil {
		L.Push(lua.LNil)
	} else {
		L.Push(lua.LString(err.Error()))
	}

	return 1
}

func join(L *lua.LState) int {
	cnt := L.GetTop()
	parts := []string{}

	for i := 1; i <= cnt; i++ {
		parts = append(parts, L.CheckString(i))
	}

	result := filepath.Join(parts...)
	L.Push(lua.LString(result))

	return 1
}

func split(L *lua.LState) int {
	path := L.CheckString(1)
	dirname, basename := filepath.Split(path)
	L.Push(lua.LString(dirname))
	L.Push(lua.LString(basename))
	return 2
}

func splitExt(L *lua.LState) int {
	path := L.CheckString(1)
	ext := filepath.Ext(path)
	stem := path[:len(path)-len(ext)]
	L.Push(lua.LString(stem))
	L.Push(lua.LString(ext))
	return 2
}

func dirname(L *lua.LState) int {
	path := L.CheckString(1)
	result := filepath.Dir(path)
	L.Push(lua.LString(result))
	return 1
}

func basename(L *lua.LState) int {
	path := L.CheckString(1)
	result := filepath.Base(path)
	L.Push(lua.LString(result))
	return 1
}

func ext(L *lua.LState) int {
	path := L.CheckString(1)
	result := filepath.Ext(path)
	L.Push(lua.LString(result))
	return 1
}
