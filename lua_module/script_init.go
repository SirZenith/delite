package luamodule

import (
	"fmt"
	"os"
	"path/filepath"

	lua_base "github.com/SirZenith/delite/lua_module/base"
	lua_fs "github.com/SirZenith/delite/lua_module/fs"
	lua_html "github.com/SirZenith/delite/lua_module/html"
	lua_html_atom "github.com/SirZenith/delite/lua_module/html/atom"
	lua "github.com/yuin/gopher-lua"
	"golang.org/x/net/html"
)

type PreprocessMeta struct {
	OutputDir      string
	OutputBaseName string
	SourceFileName string
	Book           string
	Volume         string
	Title          string
	Author         string
}

func (meta *PreprocessMeta) toLuaTable(L *lua.LState) *lua.LTable {
	tbl := L.NewTable()

	tbl.RawSetString("output_dir", lua.LString(meta.OutputDir))
	tbl.RawSetString("output_basename", lua.LString(meta.OutputBaseName))
	tbl.RawSetString("source_filename", lua.LString(meta.SourceFileName))
	tbl.RawSetString("book", lua.LString(meta.Book))
	tbl.RawSetString("volume", lua.LString(meta.Volume))
	tbl.RawSetString("title", lua.LString(meta.Title))
	tbl.RawSetString("author", lua.LString(meta.Author))

	return tbl
}

func RunPreprocessScript(nodes []*html.Node, scriptPath string, meta PreprocessMeta) ([]*html.Node, error) {
	if _, err := os.Stat(scriptPath); err != nil {
		return nil, fmt.Errorf("failed to access script %s: %s", scriptPath, err)
	}

	L := lua.NewState()
	defer L.Close()

	// setup modules
	updateScriptImportPath(L, scriptPath)

	lua_html.RegisterNodeType(L)

	L.PreloadModule("delite", lua_base.Loader)
	L.PreloadModule("fs", lua_fs.Loader)
	L.PreloadModule("html", lua_html.Loader)
	L.PreloadModule("html-atom", lua_html_atom.Loader)

	// setup global variables
	container := &html.Node{
		Type: html.DocumentNode,
	}
	for _, node := range nodes {
		container.AppendChild(node)
	}
	L.SetGlobal("doc_node", lua_html.NewNodeUserData(L, container))

	L.SetGlobal("meta", meta.toLuaTable(L))
	L.SetGlobal("fnil", L.NewFunction(func(_ *lua.LState) int { return 0 }))

	// executation
	if err := L.DoFile(scriptPath); err != nil {
		return nil, fmt.Errorf("preprocess script executation error:\n%s", err)
	}

	// return value handling
	ud, ok := L.Get(1).(*lua.LUserData)
	if !ok {
		return nil, fmt.Errorf("preprocess script does not return a userdata")
	}

	wrapped, ok := ud.Value.(*lua_html.Node)
	if !ok {
		return nil, fmt.Errorf("preprocess script returns invalid userdata, expecting Node object")
	}

	newNodes := []*html.Node{}
	docNode := wrapped.Node
	child := docNode.FirstChild
	for child != nil {
		nextChild := child.NextSibling
		docNode.RemoveChild(child)
		newNodes = append(newNodes, child)
		child = nextChild
	}

	return newNodes, nil
}

func updateScriptImportPath(L *lua.LState, scriptPath string) error {
	pack, ok := L.GetGlobal("package").(*lua.LTable)
	if !ok {
		return fmt.Errorf("failed to retrive global variable `package`")
	}

	pathVal, ok := L.GetField(pack, "path").(lua.LString)
	if !ok {
		return fmt.Errorf("`path` field of `package` table is not a string")
	}

	path := string(pathVal)
	scriptDir := filepath.Dir(scriptPath)

	path += fmt.Sprintf(";%s/?.lua;%s/?/init.lua", scriptDir, scriptDir)
	L.SetField(pack, "path", lua.LString(path))

	return nil
}
