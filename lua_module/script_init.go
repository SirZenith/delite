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
	lua_log "github.com/SirZenith/delite/lua_module/log"
	lua "github.com/yuin/gopher-lua"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
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

	Book   string
	Volume string
	Title  string
	Author string
	Artist string
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
	tbl.RawSetString("artist", lua.LString(meta.Artist))

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
	L.PreloadModule("log", lua_log.Loader)

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
	ScriptDir  string
	ScriptPath string

	BookRoot       string
	SourceFileName string

	Book      string
	Volume    string
	FullTitle string
	Author    string
	Artist    string
}

func (args *ConversionArgs) toLuaTable(L *lua.LState) *lua.LTable {
	tbl := L.NewTable()

	tbl.RawSetString("script_dir", lua.LString(args.ScriptDir))
	tbl.RawSetString("script_path", lua.LString(args.ScriptPath))

	tbl.RawSetString("book_root", lua.LString(args.BookRoot))
	tbl.RawSetString("source_filename", lua.LString(args.SourceFileName))

	tbl.RawSetString("book", lua.LString(args.Book))
	tbl.RawSetString("volume", lua.LString(args.Volume))
	tbl.RawSetString("full_title", lua.LString(args.FullTitle))
	tbl.RawSetString("author", lua.LString(args.Author))
	tbl.RawSetString("artist", lua.LString(args.Artist))

	return tbl
}

type ConverterOutputMeta struct {
	OutputDir            string
	OutputFileBasename   string
	AssetDirRelativePath string

	FrontMatter string
	BackMatter  string
}

type ConverterStateInfo struct {
	Meta    ConverterOutputMeta
	Handler lua_docconv.ConversionHandler
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

	L.PreloadModule("log", lua_log.Loader)

	lua_base_utils.RegisterUrlType(L)
	lua_docconv_linked_list.RegisterListType(L)
	lua_html.RegisterNodeType(L)

	setupScripImportPath(L, scriptPath)
	setupCommonConst(L)

	L.SetGlobal("conversion_args", args.toLuaTable(L))
}

func MakeConverterLuaState(scriptPath string, args ConversionArgs) (*lua.LState, *ConverterStateInfo, error) {
	if _, err := os.Stat(scriptPath); err != nil {
		return nil, nil, fmt.Errorf("failed to access script %s: %s", scriptPath, err)
	}

	L := lua.NewState()

	initConverterScriptEnv(L, scriptPath, args)

	// executation
	if err := L.DoFile(scriptPath); err != nil {
		L.Close()
		return nil, nil, fmt.Errorf("converter script executation error:\n%s", err)
	}

	// return value handling
	tbl, ok := L.Get(-1).(*lua.LTable)
	L.Pop(1)
	if !ok {
		L.Close()
		return nil, nil, fmt.Errorf("converter script is expected to return a table")
	}

	meta, err := ReadConverterOutputMeta(L, tbl)
	if err != nil || meta == nil {
		L.Close()
		return nil, nil, err
	}

	handler, err := lua_docconv.NewConversionHandlerFromTable(L, tbl)
	if err != nil || handler == nil {
		L.Close()
		return nil, nil, err
	}

	info := &ConverterStateInfo{
		Meta:    *meta,
		Handler: *handler,
	}

	return L, info, nil
}

func ReadConverterOutputMeta(L *lua.LState, tbl *lua.LTable) (*ConverterOutputMeta, error) {
	outputDir, _ := tbl.RawGetString("output_dir").(lua.LString)
	if outputDir == "" {
		return nil, fmt.Errorf("output directory is empty string")
	}

	outputFileBasename, _ := tbl.RawGetString("output_file_basename").(lua.LString)
	if outputFileBasename == "" {
		return nil, fmt.Errorf("output file basename is empty string")
	}

	assetDirBasename, _ := tbl.RawGetString("asset_dir_relative_path").(lua.LString)
	if assetDirBasename == "" {
		return nil, fmt.Errorf("asset directory name is empty string")
	}

	frontMatter := ""
	luaFrontMatter := tbl.RawGetString("front_matter")
	if !lua.LVIsFalse(luaFrontMatter) {
		str, ok := luaFrontMatter.(lua.LString)
		if ok {
			frontMatter = string(str)
		} else {
			return nil, fmt.Errorf("front matter is specified but it's not a string")
		}
	}

	backMatter := ""
	luaBackMatter := tbl.RawGetString("back_matter")
	if !lua.LVIsFalse(luaBackMatter) {
		str, ok := luaBackMatter.(lua.LString)
		if ok {
			backMatter = string(str)
		} else {
			return nil, fmt.Errorf("front matter is specified but it's not a string")
		}
	}

	result := &ConverterOutputMeta{
		OutputDir:            string(outputDir),
		OutputFileBasename:   string(outputFileBasename),
		AssetDirRelativePath: string(assetDirBasename),

		FrontMatter: frontMatter,
		BackMatter:  backMatter,
	}

	return result, nil
}

func RunConverterScript(L *lua.LState, stateInfo ConverterStateInfo, nodes []*html.Node) *list.List {
	container := &html.Node{
		Type:     html.ElementNode,
		DataAtom: atom.Body,
		Data:     atom.Body.String(),
	}
	for _, node := range nodes {
		container.AppendChild(node)
	}

	lua_docconv.ConversionPreprocess(L, container, "", &stateInfo.Handler)

	content, _ := lua_docconv.ConvertHtmlWithLuaConverter(L, container, "", nil, &stateInfo.Handler)

	if stateInfo.Meta.FrontMatter != "" {
		content.PushFront(stateInfo.Meta.FrontMatter)
	}

	if stateInfo.Meta.BackMatter != "" {
		content.PushBack(stateInfo.Meta.BackMatter)
	}

	return content
}
