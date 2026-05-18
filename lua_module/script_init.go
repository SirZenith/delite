package luamodule

import (
	"container/list"
	"fmt"
	"os"
	"path/filepath"

	lua_base "github.com/SirZenith/delite/lua_module/base"
	lua_base_utils "github.com/SirZenith/delite/lua_module/base/utils"
	lua_docconv "github.com/SirZenith/delite/lua_module/docconv"
	lua_docconv_converter "github.com/SirZenith/delite/lua_module/docconv/converter"
	lua_docconv_linked_list "github.com/SirZenith/delite/lua_module/docconv/linked_list"
	lua_fs "github.com/SirZenith/delite/lua_module/fs"
	lua_html "github.com/SirZenith/delite/lua_module/html"
	lua_html_atom "github.com/SirZenith/delite/lua_module/html/atom"
	lua "github.com/yuin/gopher-lua"
	"golang.org/x/net/html"
)

// ----------------------------------------------------------------------------

func setupScripImportPath(L *lua.LState, scriptPath string) error {
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
func setupCommonConst(L *lua.LState) {
	L.SetGlobal("fnil", L.NewFunction(func(_ *lua.LState) int { return 0 }))
}

func setupGlobalDocNode(L *lua.LState, nodes []*html.Node) *html.Node {
	container := &html.Node{
		Type: html.DocumentNode,
	}
	for _, node := range nodes {
		container.AppendChild(node)
	}
	L.SetGlobal("doc_node", lua_html.NewNodeUserData(L, container))
	return container
}

// ----------------------------------------------------------------------------

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

	L.PreloadModule("delite", lua_base.Loader)
	L.PreloadModule("fs", lua_fs.Loader)
	L.PreloadModule("html", lua_html.Loader)
	L.PreloadModule("html.atom", lua_html_atom.Loader)

	lua_html.RegisterNodeType(L)

	// setup modules
	setupScripImportPath(L, scriptPath)
	setupCommonConst(L)
	setupGlobalDocNode(L, nodes)

	// setup global variables
	L.SetGlobal("meta", meta.toLuaTable(L))

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

// ----------------------------------------------------------------------------

type ConversionArgs struct {
	SourceFileName string
	Book           string
	Volume         string
	Title          string
	Author         string
}

func (args *ConversionArgs) toLuaTable(L *lua.LState) *lua.LTable {
	tbl := L.NewTable()

	tbl.RawSetString("source_filename", lua.LString(args.SourceFileName))
	tbl.RawSetString("book", lua.LString(args.Book))
	tbl.RawSetString("volume", lua.LString(args.Volume))
	tbl.RawSetString("title", lua.LString(args.Title))
	tbl.RawSetString("author", lua.LString(args.Author))

	return tbl
}

type ConversionResult struct {
	Content   *list.List
	Basename  string
	Extension string
}

type ConverterOutputMeta struct {
	OutputDirBasename string
	Basename          string
	Extension         string
}

func initConverterScriptEnv(L *lua.LState, scriptPath string, args ConversionArgs) {
	L.PreloadModule("delite", lua_base.Loader)
	L.PreloadModule("delite.utils", lua_base_utils.Loader)

	L.PreloadModule("fs", lua_fs.Loader)
	L.PreloadModule("html", lua_html.Loader)
	L.PreloadModule("html.atom", lua_html_atom.Loader)

	L.PreloadModule("docconv", lua_docconv.Loader)
	L.PreloadModule("docconv.converter", lua_docconv_converter.Loader)
	L.PreloadModule("docconv.linked_list", lua_docconv_linked_list.Loader)

	lua_base_utils.RegisterUrlType(L)
	lua_docconv_linked_list.RegisterListType(L)
	lua_html.RegisterNodeType(L)

	setupScripImportPath(L, scriptPath)
	setupCommonConst(L)

	L.SetGlobal("conversion_args", args.toLuaTable(L))
}

func GetConverterScripOutputMeta(scriptPath string) (*ConverterOutputMeta, error) {
	if _, err := os.Stat(scriptPath); err != nil {
		return nil, fmt.Errorf("failed to access script %s: %s", scriptPath, err)
	}

	L := lua.NewState()
	defer L.Close()

	initConverterScriptEnv(L, scriptPath, ConversionArgs{})

	// executation
	if err := L.DoFile(scriptPath); err != nil {
		return nil, fmt.Errorf("converter script executation error:\n%s", err)
	}

	// return value handling
	tbl, ok := L.Get(1).(*lua.LTable)
	if !ok {
		return nil, fmt.Errorf("converter script is expected to return a table")
	}

	outputDirBaseame, _ := tbl.RawGetString("output_dir_basename").(lua.LString)
	if outputDirBaseame == "" {
		return nil, fmt.Errorf("output directory basename is empty")
	}

	// output basename is allowed to be settle when conversion is run.
	basename, _ := tbl.RawGetString("output_basename").(lua.LString)

	extension, ok := tbl.RawGetString("output_ext").(lua.LString)
	if !ok {
		return nil, fmt.Errorf("output extension value is not a string")
	}

	result := &ConverterOutputMeta{
		OutputDirBasename: string(outputDirBaseame),
		Basename:          string(basename),
		Extension:         string(extension),
	}

	return result, nil
}

func RunConverterScript(nodes []*html.Node, scriptPath string, args ConversionArgs) (*ConversionResult, error) {
	if _, err := os.Stat(scriptPath); err != nil {
		return nil, fmt.Errorf("failed to access script %s: %s", scriptPath, err)
	}

	L := lua.NewState()
	defer L.Close()

	initConverterScriptEnv(L, scriptPath, args)
	nodeContainer := setupGlobalDocNode(L, nodes)

	// executation
	if err := L.DoFile(scriptPath); err != nil {
		return nil, fmt.Errorf("converter script executation error:\n%s", err)
	}

	// return value handling
	tbl, ok := L.Get(1).(*lua.LTable)
	if !ok {
		return nil, fmt.Errorf("converter script is expected to return a table")
	}

	result, err := doHtmlConversionWithLuaReturn(L, tbl, nodeContainer)
	if err != nil {
		return nil, fmt.Errorf("failed to convert HTML with Lua metadata: %s", err)
	}

	return result, nil
}

func doHtmlConversionWithLuaReturn(L *lua.LState, tbl *lua.LTable, node *html.Node) (*ConversionResult, error) {
	basename, _ := tbl.RawGetString("output_basename").(lua.LString)
	if basename == "" {
		return nil, fmt.Errorf("output basename is empty")
	}

	extension, ok := tbl.RawGetString("output_ext").(lua.LString)
	if !ok {
		return nil, fmt.Errorf("output extension value is not a string")
	}

	handler, err := lua_docconv.NewConversionHandlerFromTable(L, tbl)
	if err != nil {
		return nil, err
	}

	content, _ := lua_docconv.ConvertHtmlWithLuaConverter(L, node, "", nil, handler)

	if frontMatter, ok := tbl.RawGetString("front_matter").(lua.LString); ok {
		content.PushFront(string(frontMatter))
	}

	if backMatter, ok := tbl.RawGetString("back_matter").(lua.LString); ok {
		content.PushBack(string(backMatter))
	}

	result := &ConversionResult{
		Content:   content,
		Basename:  string(basename),
		Extension: string(extension),
	}

	return result, nil
}
