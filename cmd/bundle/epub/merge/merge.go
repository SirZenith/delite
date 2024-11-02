package merge

import (
	"context"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/SirZenith/delite/format/epub"
	format_html "github.com/SirZenith/delite/format/html"
	lua_html "github.com/SirZenith/delite/lua_module/html"
	lua_html_atom "github.com/SirZenith/delite/lua_module/html/atom"
	"github.com/charmbracelet/log"
	"github.com/urfave/cli/v3"
	lua "github.com/yuin/gopher-lua"
	"golang.org/x/net/html"
)

func Cmd() *cli.Command {
	var epubFile string

	cmd := &cli.Command{
		Name:  "merge",
		Usage: "merge HTML content of EPUB book into a single HTML file.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "template-file",
				Aliases: []string{"T"},
				Usage:   "path to file containing template string, ignored when `template` flag has non-empty value.",
			},
			&cli.StringFlag{
				Name:    "template",
				Aliases: []string{"t"},
				Usage: strings.Join([]string{
					"output template string.",
					"    1. For HTML format, this should be HTML text.",
					"       By default book content will be filled into `body` tag of template,",
					"       User can specify container element by ID attribute and `html-id` flag",
				}, "\n"),
			},
			&cli.StringFlag{
				Name:  "html-id",
				Usage: "id of target HTML tag to fill book content to",
			},
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "path to output directory, if no value is given, a directory with the same name as book file (without extension) will be created, and result will be written to that file",
			},
			&cli.StringFlag{
				Name:    "format",
				Aliases: []string{"f"},
				Usage: "output format, valid values are: " + strings.Join([]string{
					outputFormatHTML,
					outputFormatLatex,
				}, ", "),
				Value: outputFormatHTML,
			},
			&cli.StringFlag{
				Name:  "script",
				Usage: "path to preprocess script",
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "input",
				UsageText:   "<epub-file>",
				Destination: &epubFile,
				Min:         1,
				Max:         1,
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			options, err := getOptionsFromCmd(cmd, epubFile)
			if err != nil {
				return err
			}

			return cmdMain(options)
		},
	}

	return cmd
}

type options struct {
	template        string
	htmlContainerID string

	epubFile         string
	outputDir        string
	assetDirName     string
	outputFormat     string
	preprocessScript string

	jobCnt int
}

func getOptionsFromCmd(cmd *cli.Command, epubFile string) (options, error) {
	options := options{
		template:        cmd.String("template"),
		htmlContainerID: cmd.String("html-id"),

		epubFile:         epubFile,
		outputDir:        cmd.String("output"),
		assetDirName:     defaultAssetDirName,
		outputFormat:     cmd.String("format"),
		preprocessScript: cmd.String("script"),

		jobCnt: runtime.NumCPU(),
	}

	if options.outputDir == "" {
		ext := filepath.Ext(options.epubFile)
		basename := filepath.Base(options.epubFile)
		options.outputDir = basename[:len(basename)-len(ext)]
	}

	switch options.outputFormat {
	case outputFormatHTML, outputFormatLatex:
		// pass
	default:
		return options, fmt.Errorf("invalid output format: %q", options.outputFormat)
	}

	templateFile := cmd.String("template-file")
	if options.template != "" {
		// pass
	} else if templateFile == "" {
		options.template = getDefaultTemplate(options.outputFormat)
	} else {
		data, err := os.ReadFile(templateFile)
		if err != nil {
			return options, fmt.Errorf("failed to read template file %s: %s", templateFile, err)
		}

		options.template = string(data)
	}

	return options, nil
}

func getDefaultTemplate(format string) string {
	switch format {
	case outputFormatHTML:
		return defaultHTMLTemplate
	case outputFormatLatex:
		return defaultLatexTemplte
	default:
		return ""
	}
}

func cmdMain(options options) error {
	if _, err := os.Stat(options.epubFile); err != nil {
		return fmt.Errorf("can't access target file %s: %s", options.epubFile, err)
	}

	if err := os.MkdirAll(options.outputDir, 0o755); err != nil {
		return fmt.Errorf("failed to create output directory %s: %s", options.outputDir, err)
	}

	assetOutDir := filepath.Join(options.outputDir, options.assetDirName)
	if err := os.MkdirAll(assetOutDir, 0o755); err != nil {
		return fmt.Errorf("failed to create asset directory %s: %s", assetOutDir, err)
	}

	merger := new(epub.EpubReader)
	if err := merger.Init(epub.EpubReaderOptions{
		EpubFile:     options.epubFile,
		OutputDir:    options.outputDir,
		AssetDirName: options.assetDirName,
		JobCnt:       options.jobCnt,
	}); err != nil {
		return err
	}

	defer merger.Close()

	nodes, errList := merger.Merge()
	for _, err := range errList {
		log.Warnf("%s", err)
	}

	nodePreprocess(options, merger, nodes)

	var err error
	outputBasename := merger.GetMergeOutputBasename()
	switch options.outputFormat {
	case outputFormatHTML:
		err = saveHTMLOutput(options, nodes, outputBasename)
	case outputFormatLatex:
		err = saveLatexOutput(options, nodes, outputBasename)
	}

	return err
}

func nodePreprocess(options options, merger *epub.EpubReader, nodes []*html.Node) {
	// image reference handling
	nameMap := map[string]string{}
	contextFile := ""
	for _, node := range nodes {
		contextFile = format_html.ImageReferenceRedirect(node, contextFile, options.assetDirName, nameMap)
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

	// format specific preprocessing
	switch options.outputFormat {
	case outputFormatHTML:
		nodes = htmlOutputPreprocess(options, nodes)
	case outputFormatLatex:
		nodes = latexOutputPreprocess(options, nodes)
	}

	// user script
	if options.preprocessScript != "" {
		if processed, err := runPreprocessScript(nodes, options.preprocessScript); err == nil {
			nodes = processed
		} else {
			log.Warnf("failed to run preprocess script %s:\n%s", options.preprocessScript, err)
		}
	}
}

func runPreprocessScript(nodes []*html.Node, scriptPath string) ([]*html.Node, error) {
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
