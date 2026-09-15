package epub_by_script

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/SirZenith/delite/format/epub"
	"github.com/SirZenith/delite/format/latex"
	luamodule "github.com/SirZenith/delite/lua_module"
	"github.com/urfave/cli/v3"
	lua "github.com/yuin/gopher-lua"
	"golang.org/x/net/html"
)

const defaultAssetDirName = "assets"

func Cmd() *cli.Command {
	var epubFile string

	cmd := &cli.Command{
		Name:  "epub-by-script",
		Usage: "convert EPUB book with convertor script",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:    "job",
				Aliases: []string{"j"},
				Value:   -1,
			},
			&cli.StringFlag{
				Name:  "library",
				Usage: "path to library info JSON file",
				Value: "./library.json",
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
				Usage:   "path to output directory, if no value is given, a directory with the same name as book file (without extension) will be created, and result will be written to that file",
			},
			&cli.BoolFlag{
				Name:  "overwrite-assets",
				Usage: "write ePub asset even if they already exist",
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
	epubFile         string
	outputDir        string
	assetDirName     string
	preprocessScript string

	jobCnt int

	preprocessScriptRelativePath string
	preprocessScriptImportPath   string
	converterScript              string
	overwriteAssets              bool
}

func getOptionsFromCmd(cmd *cli.Command, epubFile string) (options, error) {
	options := options{
		epubFile:         epubFile,
		outputDir:        cmd.String("output"),
		assetDirName:     defaultAssetDirName,
		preprocessScript: cmd.String("script"),

		jobCnt:                       runtime.NumCPU(),
		preprocessScriptRelativePath: cmd.String("preprocess"),
		preprocessScriptImportPath:   cmd.String("preprocess-import-dir"),
		converterScript:              cmd.String("converter"),
		overwriteAssets:              cmd.Bool("overwrite-assets"),
	}

	if options.outputDir == "" {
		ext := filepath.Ext(options.epubFile)
		basename := filepath.Base(options.epubFile)
		options.outputDir = basename[:len(basename)-len(ext)]
	}

	return options, nil
}

func cmdMain(options options) error {
	basename := filepath.Base(options.epubFile)

	ext := filepath.Ext(basename)
	bookTitle := basename[:len(basename)-len(ext)]

	ls, stateInfo, err := luamodule.MakeConverterLuaState(options.converterScript, luamodule.ConversionArgs{
		ScriptDir:  filepath.Dir(options.converterScript),
		ScriptPath: options.converterScript,

		BookRoot:       options.outputDir,
		SourceFileName: filepath.Base(options.epubFile),
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

	return epub.Merge(epub.EpubMergeOptions{
		ReaderOption: epub.EpubReaderOptions{
			EpubFile:     options.epubFile,
			OutputDir:    stateInfo.Meta.OutputDir,
			AssetDirName: stateInfo.Meta.AssetDirRelativePath,

			JobCnt:          runtime.NumCPU(),
			OverwriteAssets: options.overwriteAssets,
		},

		PreprocessFunc: func(nodes []*html.Node) ([]*html.Node, error) {
			// TODO: this preprocess is not specific to LaTeX format, extract it
			// as a common function.
			nodes = latex.FromEpubPreprocess(nodes, latex.FromEpubOptions{
				OutputDir: stateInfo.Meta.OutputDir,

				Title:  bookTitle,
				Author: "",
			})

			if options.preprocessScript != "" {
				meta := luamodule.PreprocessMeta{
					OutputDir:      stateInfo.Meta.OutputDir,
					OutputBaseName: stateInfo.Meta.OutputFileBasename,
					SourceFileName: filepath.Base(options.epubFile),

					Book:   bookTitle,
					Volume: "",
					Title:  bookTitle,
					Author: "",
					Artist: "",
				}

				if processed, err := luamodule.RunPreprocessScript(nodes, options.preprocessScript, []string{options.preprocessScriptImportPath}, meta); err == nil {
					nodes = processed
				} else {
					return nil, err
				}
			}

			return nodes, nil
		},
		SaveOutputFunc: func(nodes []*html.Node, _ string, _ string) error {
			return runConverterScript(ls, stateInfo, nodes)
		},
	})
}

func runConverterScript(ls *lua.LState, stateInfo *luamodule.ConverterStateInfo, nodes []*html.Node) error {
	result := luamodule.RunConverterScript(ls, *stateInfo, nodes)

	// write output file
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
