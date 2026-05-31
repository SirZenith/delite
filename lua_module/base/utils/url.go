package utils

import (
	"net/url"

	lua "github.com/yuin/gopher-lua"
)

const UrlTypeName = "delite.utils.url"

func RegisterUrlType(L *lua.LState) *lua.LTable {
	mt := L.NewTypeMetatable(UrlTypeName)

	L.SetFuncs(mt, urlStaticMethod)
	L.SetField(mt, "__index", L.SetFuncs(L.NewTable(), urlMethods))

	return mt
}

func CheckUrl(L *lua.LState, index int) *url.URL {
	ud := L.CheckUserData(index)
	if v, ok := ud.Value.(*url.URL); ok {
		return v
	}

	L.ArgError(index, "Url expected")

	return nil
}

func WrapUrl(L *lua.LState, value *url.URL) *lua.LUserData {
	ud := L.NewUserData()
	ud.Value = value

	L.SetMetatable(ud, L.GetTypeMetatable(UrlTypeName))

	return ud
}

func AddUrlToState(L *lua.LState, value *url.URL) int {
	if value == nil {
		L.Push(lua.LNil)
		return 1
	}

	ud := WrapUrl(L, value)
	L.Push(ud)

	return 1
}

// ----------------------------------------------------------------------------

var urlStaticMethod = map[string]lua.LGFunction{
	"parse":         urlParse,
	"path_unescape": urlPathUnescape,
	"__tostring":    urlMetaTostring,
}

// urlParse creates a new URL object by parsing string argument.
func urlParse(L *lua.LState) int {
	str := L.CheckString(1)

	value, error := url.Parse(str)
	if error == nil {
		AddUrlToState(L, value)
		L.Push(lua.LNil)
	} else {
		L.Push(lua.LNil)
		L.Push(lua.LString(error.Error()))
	}

	return 2
}

// urlPathUnescape unescapes given URL string.
func urlPathUnescape(L *lua.LState) int {
	str := L.CheckString(1)

	value, error := url.PathUnescape(str)
	if error == nil {
		L.Push(lua.LString(value))
		L.Push(lua.LNil)
	} else {
		L.Push(lua.LNil)
		L.Push(lua.LString(error.Error()))
	}

	return 2
}

// urlMetaTostring is meta method __tostring for URL object.
func urlMetaTostring(L *lua.LState) int {
	value := CheckUrl(L, 1)
	L.Push(lua.LString(value.String()))
	return 1
}

// ----------------------------------------------------------------------------

var urlMethods = map[string]lua.LGFunction{
	"scheme":    urlGetSetScheme,
	"host":      urlGetSetHost,
	"path":      urlGetSetPath,
	"raw_query": urlGetSetRawQuery,
	"fragment":  urlGetSetFragment,
}

// urlGetSetScheme is getter/setter for URL scheme.
func urlGetSetScheme(L *lua.LState) int {
	value := CheckUrl(L, 1)

	if L.GetTop() == 1 {
		L.Push(lua.LString(value.Scheme))
		return 1
	}

	val := L.CheckString(2)
	value.Scheme = val

	return 0
}

// urlGetSetHost is getter/setter for URL host.
func urlGetSetHost(L *lua.LState) int {
	value := CheckUrl(L, 1)

	if L.GetTop() == 1 {
		L.Push(lua.LString(value.Host))
		return 1
	}

	val := L.CheckString(2)
	value.Host = val

	return 0
}

// urlGetSetPath is getter/setter for URL path.
func urlGetSetPath(L *lua.LState) int {
	value := CheckUrl(L, 1)

	if L.GetTop() == 1 {
		L.Push(lua.LString(value.Path))
		return 1
	}

	val := L.CheckString(2)
	value.Path = val

	return 0
}

// urlGetSetRawQuery is getter/setter for URL raw query.
func urlGetSetRawQuery(L *lua.LState) int {
	value := CheckUrl(L, 1)

	if L.GetTop() == 1 {
		L.Push(lua.LString(value.RawQuery))
		return 1
	}

	val := L.CheckString(2)
	value.RawQuery = val

	return 0
}

// urlGetSetFragment is getter/setter for URL fragment.
func urlGetSetFragment(L *lua.LState) int {
	value := CheckUrl(L, 1)

	if L.GetTop() == 1 {
		L.Push(lua.LString(value.Fragment))
		return 1
	}

	val := L.CheckString(2)
	value.Fragment = val

	return 0
}
