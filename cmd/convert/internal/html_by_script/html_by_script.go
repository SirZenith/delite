package html_by_script

import (
	"bufio"
	"context"
	"fmt"
	"image"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/SirZenith/delite/common"
	"github.com/SirZenith/delite/common/html_util"
	"github.com/SirZenith/delite/format/epub"
	format_html "github.com/SirZenith/delite/format/html"
	luamodule "github.com/SirZenith/delite/lua_module"
	"github.com/charmbracelet/log"
	"github.com/urfave/cli/v3"
	lua "github.com/yuin/gopher-lua"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

const defaultAssetOutputExt = ".png"

func Cmd() *cli.Command {
	var htmlFile string

	cmd := &cli.Command{
		Name:  "html-by-script",
		Usage: "convert HTML file with convertor script",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:    "job",
				Aliases: []string{"j"},
				Value:   -1,
			},
			&cli.StringFlag{
				Name:  "preprocess",
				Usage: "path to preprocess Lua script",
			},
			&cli.StringFlag{
				Name:  "preprocess-import-dir",
				Usage: "path to extra preprocess script import directory",
			},
			&cli.StringFlag{
				Name:     "converter",
				Usage:    "path to converter Lua script",
				Required: true,
			},
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "path to output directory, if no value is given, a directory with the same name as HTML file (without extension) will be created, and result will be written to that directory",
			},
			&cli.BoolFlag{
				Name:  "overwrite-assets",
				Usage: "write asset files even if they already exist",
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "input",
				UsageText:   "<html-file>",
				Destination: &htmlFile,
				Min:         1,
				Max:         1,
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			options, err := getOptionsFromCmd(cmd, htmlFile)
			if err != nil {
				return err
			}

			return cmdMain(options)
		},
	}

	return cmd
}

type options struct {
	htmlFile  string
	outputDir string
	jobCnt    int

	preprocessScript      string
	preprocessImportPaths []string
	converterScript       string
	overwriteAssets       bool
}

func getOptionsFromCmd(cmd *cli.Command, htmlFile string) (options, error) {
	options := options{
		htmlFile:         htmlFile,
		outputDir:        cmd.String("output"),
		preprocessScript: cmd.String("preprocess"),

		jobCnt:          int(cmd.Int("job")),
		converterScript: cmd.String("converter"),
		overwriteAssets: cmd.Bool("overwrite-assets"),
	}

	if options.jobCnt <= 0 {
		options.jobCnt = runtime.NumCPU()
	}

	if importPath := cmd.String("preprocess-import-dir"); importPath != "" {
		if absPath, err := filepath.Abs(importPath); err == nil {
			options.preprocessImportPaths = []string{absPath}
		}
	}

	if options.outputDir == "" {
		ext := filepath.Ext(options.htmlFile)
		basename := filepath.Base(options.htmlFile)
		options.outputDir = basename[:len(basename)-len(ext)]
	}

	return options, nil
}

func cmdMain(options options) error {
	basename := filepath.Base(options.htmlFile)

	ext := filepath.Ext(basename)
	bookTitle := basename[:len(basename)-len(ext)]

	ls, stateInfo, err := luamodule.MakeConverterLuaState(options.converterScript, luamodule.ConversionArgs{
		ScriptDir:  filepath.Dir(options.converterScript),
		ScriptPath: options.converterScript,

		BookRoot:       options.outputDir,
		SourceFileName: basename,
		Book:           bookTitle,
		Volume:         "",
		FullTitle:      "",
		Author:         "",
		Artist:         "",

		BundleOptions: nil,
	})
	if ls != nil {
		defer ls.Close()
	}

	if err != nil {
		return fmt.Errorf("failed to prepare Lua state for converter script: %s", err)
	}

	err = os.MkdirAll(stateInfo.Meta.OutputDir, 0o777)
	if err != nil {
		return fmt.Errorf("failed to create output directory %s: %s", stateInfo.Meta.OutputDir, err)
	}

	nodes, err := readHTMLFile(options.htmlFile)
	if err != nil {
		return err
	}

	errList := processImageAssets(options, nodes, stateInfo.Meta.OutputDir, stateInfo.Meta.AssetDirRelativePath)
	for _, err := range errList {
		log.Warnf("%s", err)
	}

	nodes = epub.FromEpubPreprocess(nodes, epub.FromEpubOptions{
		OutputDir: stateInfo.Meta.OutputDir,

		Title:  bookTitle,
		Author: "",
	})

	if options.preprocessScript != "" {
		meta := luamodule.PreprocessMeta{
			OutputDir:      stateInfo.Meta.OutputDir,
			OutputBaseName: stateInfo.Meta.OutputFileBasename,
			SourceFileName: basename,

			Book:   bookTitle,
			Volume: "",
			Title:  bookTitle,
			Author: "",
			Artist: "",
		}

		if processed, err := luamodule.RunPreprocessScript(nodes, options.preprocessScript, options.preprocessImportPaths, meta); err == nil {
			nodes = processed
		} else {
			return err
		}
	}

	return runConverterScript(ls, stateInfo, nodes)
}

func readHTMLFile(fileName string) ([]*html.Node, error) {
	file, err := os.Open(fileName)
	if err != nil {
		return nil, fmt.Errorf("failed to open HTML file %s: %s", fileName, err)
	}
	defer file.Close()

	doc, err := html.Parse(file)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML content of %s: %s", fileName, err)
	}

	body := html_util.FindHTMLTag(doc, html.ElementNode, atom.Body)
	if body == nil {
		return nil, fmt.Errorf("failed to find body tag in HTML content of %s", fileName)
	}

	nodes := []*html.Node{}
	child := body.FirstChild
	for child != nil {
		nextChild := child.NextSibling

		body.RemoveChild(child)
		nodes = append(nodes, child)

		child = nextChild
	}

	return nodes, nil
}

type assetTask struct {
	src string
	dst string
}

func processImageAssets(options options, nodes []*html.Node, outputDir, assetDirName string) []error {
	srcDir := filepath.Dir(options.htmlFile)

	imageNameMap := map[string]string{}
	contextFile := filepath.Base(options.htmlFile)
	for _, node := range nodes {
		contextFile = format_html.ImageReferenceRedirect(node, contextFile, assetDirName, defaultAssetOutputExt, imageNameMap)
	}

	var errList []error
	sizeMap := map[string]*image.Point{}
	for srcPath, dstPath := range imageNameMap {
		filePath, err := url.PathUnescape(filepath.Join(srcDir, srcPath))
		if err != nil {
			errList = append(errList, fmt.Errorf("invalid image path %s: %s", srcPath, err))
			continue
		}

		size, err := getImageSize(filePath)
		if err != nil {
			errList = append(errList, err)
			continue
		}

		sizeMap[dstPath] = size
	}

	for _, node := range nodes {
		format_html.SetImageSizeMeta(node, sizeMap)
	}

	tasks := make([]assetTask, 0, len(imageNameMap))
	for srcPath, dstPath := range imageNameMap {
		tasks = append(tasks, assetTask{
			src: filepath.Join(srcDir, srcPath),
			dst: filepath.Join(outputDir, dstPath),
		})
	}

	errList = append(errList, copyImageAssets(tasks, options.jobCnt, options.overwriteAssets)...)

	return errList
}

func copyImageAssets(tasks []assetTask, jobCnt int, overwrite bool) []error {
	taskChan := make(chan assetTask)
	errChan := make(chan error, len(tasks))

	var group sync.WaitGroup
	for i := 0; i < jobCnt; i++ {
		group.Add(1)
		go func() {
			defer group.Done()

			for task := range taskChan {
				if err := copyImageAsset(task.src, task.dst, overwrite); err != nil {
					errChan <- err
				}
			}
		}()
	}

	for _, task := range tasks {
		taskChan <- task
	}
	close(taskChan)

	group.Wait()
	close(errChan)

	errList := []error{}
	for err := range errChan {
		errList = append(errList, err)
	}

	return errList
}

func copyImageAsset(srcPath, dstPath string, overwrite bool) error {
	if !overwrite {
		if info, err := os.Stat(dstPath); err == nil && !info.IsDir() {
			return nil
		}
	}

	filePath, err := url.PathUnescape(srcPath)
	if err != nil {
		return fmt.Errorf("invalid image path %s: %s", srcPath, err)
	}

	srcFile, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open image %s: %s", filePath, err)
	}
	defer srcFile.Close()

	if err := os.MkdirAll(filepath.Dir(dstPath), 0o777); err != nil {
		return fmt.Errorf("failed to create asset directory for %s: %s", dstPath, err)
	}

	dstFile, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("failed to create asset file %s: %s", dstPath, err)
	}
	defer dstFile.Close()

	srcBuf := bufio.NewReader(srcFile)
	dstBuf := bufio.NewWriter(dstFile)
	if _, err := common.ConvertImageTo(srcBuf, dstBuf, common.ImageFormatPng); err != nil {
		dstFile.Close()
		os.Remove(dstPath)

		return fmt.Errorf("failed to convert image %s: %s", filePath, err)
	}

	if err := dstBuf.Flush(); err != nil {
		return fmt.Errorf("failed to flush asset file %s: %s", dstPath, err)
	}

	return nil
}

func getImageSize(filePath string) (*image.Point, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %s", filePath, err)
	}
	defer file.Close()

	fileReader := bufio.NewReader(file)
	img, _, err := image.Decode(fileReader)
	if err != nil {
		return nil, fmt.Errorf("can't decode image %s: %s", filePath, err)
	}

	size := img.Bounds().Size()

	return &size, nil
}

func runConverterScript(ls *lua.LState, stateInfo *luamodule.ConverterStateInfo, nodes []*html.Node) error {
	result := luamodule.RunConverterScript(ls, *stateInfo, nodes)

	outputName := filepath.Join(stateInfo.Meta.OutputDir, stateInfo.Meta.OutputFileBasename)
	outFile, err := os.Create(outputName)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %s", outputName, err)
	}
	defer outFile.Close()

	outWriter := bufio.NewWriter(outFile)

	for ele := result.Front(); ele != nil; ele = ele.Next() {
		switch v := ele.Value.(type) {
		case lua.LValue:
			outWriter.WriteString(v.String())
		case string:
			outWriter.WriteString(v)
		default:
		}
	}

	err = outWriter.Flush()
	if err != nil {
		return fmt.Errorf("failed to flush output file buffer: %s", err)
	}

	return nil
}
