package epub_merge

import (
	"fmt"
	"image"
	"os"
	"path/filepath"

	"github.com/SirZenith/delite/format/epub"
	format_html "github.com/SirZenith/delite/format/html"
	lua_html "github.com/SirZenith/delite/lua_module/html"
	lua_html_atom "github.com/SirZenith/delite/lua_module/html/atom"
	"github.com/charmbracelet/log"
	lua "github.com/yuin/gopher-lua"
	"golang.org/x/net/html"
)

type EpubMergeOptions struct {
	EpubFile         string
	OutputDir        string
	AssetDirName     string
	PreprocessScript string

	JobCnt int

	PreprocessFunc func(nodes []*html.Node) []*html.Node
	SaveOutputFunc func(nodes []*html.Node, fileBasename string) error
}

func Merge(options EpubMergeOptions) error {
	if _, err := os.Stat(options.EpubFile); err != nil {
		return fmt.Errorf("can't access target file %s: %s", options.EpubFile, err)
	}

	if err := os.MkdirAll(options.OutputDir, 0o755); err != nil {
		return fmt.Errorf("failed to create output directory %s: %s", options.OutputDir, err)
	}

	assetOutDir := filepath.Join(options.OutputDir, options.AssetDirName)
	if err := os.MkdirAll(assetOutDir, 0o755); err != nil {
		return fmt.Errorf("failed to create asset directory %s: %s", assetOutDir, err)
	}

	merger := new(epub.EpubReader)
	if err := merger.Init(epub.EpubReaderOptions{
		EpubFile:     options.EpubFile,
		OutputDir:    options.OutputDir,
		AssetDirName: options.AssetDirName,
		JobCnt:       options.JobCnt,
	}); err != nil {
		return err
	}

	defer merger.Close()

	nodes, errList := merger.Merge()
	for _, err := range errList {
		log.Warnf("%s", err)
	}

	NodePreprocess(options, merger, nodes)

	outputBasename := merger.GetMergeOutputBasename()

	return options.SaveOutputFunc(nodes, outputBasename)
}

func NodePreprocess(options EpubMergeOptions, merger *epub.EpubReader, nodes []*html.Node) {
	// image reference handling
	nameMap := map[string]string{}
	contextFile := ""
	for _, node := range nodes {
		contextFile = format_html.ImageReferenceRedirect(node, contextFile, options.AssetDirName, nameMap)
	}

	if errList := merger.BatchDumpAsset(nameMap); errList != nil {
		for _, err := range errList {
			log.Warnf("%s", err)
		}
	}

	// image meta data injection
	sizeMap := map[string]*image.Point{}
	for srcPath, dstPath := range nameMap {
		if size, err := merger.GetImageSize(srcPath); err == nil {
			sizeMap[dstPath] = size
		} else {
			log.Warnf("failed to get image size: %s", err)
		}
	}
	for _, node := range nodes {
		format_html.SetImageSizeMeta(node, sizeMap)
	}

	nodes = options.PreprocessFunc(nodes)

	// user script
	if options.PreprocessScript != "" {
		if processed, err := RunPreprocessScript(nodes, options.PreprocessScript); err == nil {
			nodes = processed
		} else {
			log.Warnf("failed to run preprocess script %s:\n%s", options.PreprocessScript, err)
		}
	}
}

func RunPreprocessScript(nodes []*html.Node, scriptPath string) ([]*html.Node, error) {
	if _, err := os.Stat(scriptPath); err != nil {
		return nil, fmt.Errorf("failed to access script %s: %s", scriptPath, err)
	}

	L := lua.NewState()
	defer L.Close()

	lua_html.RegisterNodeType(L)

	L.PreloadModule("html", lua_html.Loader)
	L.PreloadModule("html-atom", lua_html_atom.Loader)

	luaNodes := L.NewTable()
	for i, node := range nodes {
		luaNode := lua_html.NewNode(L, node)
		L.RawSetInt(luaNodes, i+1, luaNode)
	}
	L.SetGlobal("nodes", luaNodes)

	if err := L.DoFile(scriptPath); err != nil {
		return nil, fmt.Errorf("preprocess script executation error:\n%s", err)
	}

	tbl, ok := L.Get(1).(*lua.LTable)
	if !ok {
		return nil, fmt.Errorf("preprocess script does not return a table")
	}

	totalCnt := tbl.Len()
	newNodes := []*html.Node{}
	for i := 1; i <= totalCnt; i++ {
		value := tbl.RawGetInt(i)

		ud, ok := value.(*lua.LUserData)
		if !ok {
			return nil, fmt.Errorf("invalid return value found at index %d, expecting userdata, found %s", i, value.Type().String())
		}

		wrapped, ok := ud.Value.(*lua_html.Node)
		if !ok {
			return nil, fmt.Errorf("invalid usertdata found at index %d", i)
		}

		newNodes = append(newNodes, wrapped.Node)
	}

	return newNodes, nil
}
