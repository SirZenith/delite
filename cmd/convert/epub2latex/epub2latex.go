package epub2latex

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/SirZenith/delite/cmd/convert/common/epub_merge"
	"github.com/SirZenith/delite/format/latex"
	"github.com/urfave/cli/v3"
	"golang.org/x/net/html"
)

const defaultAssetDirName = "assets"

const defaultLatexTemplte = `
\documentclass{ltjtbook}

\usepackage{
    afterpage,
    geometry,
    graphicx,
    hyperref,
    luatexja-fontspec,
    pdfpages,
    pxrubrica,
    url,
}

\setmainjfont{SourceHanSerif-Medium}

\rubysetup{g}

\geometry{
	paperwidth = 12cm,
	paperheight = 16cm,
    top = 1.5cm,
    bottom = 1.5cm,
    left = 1.2cm,
    right = 1.2cm,
}
`

func Cmd() *cli.Command {
	var epubFile string

	cmd := &cli.Command{
		Name:  "epub2latex",
		Usage: "convert EPUB book into a LaTeX file",
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
					"output template string, should be preamble content of output file, e.g. content before `\\began{document}` command",
				}, "\n"),
			},
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "path to output directory, if no value is given, a directory with the same name as book file (without extension) will be created, and result will be written to that file",
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
	template string

	epubFile         string
	outputDir        string
	assetDirName     string
	preprocessScript string

	jobCnt int
}

func getOptionsFromCmd(cmd *cli.Command, epubFile string) (options, error) {
	options := options{
		template: cmd.String("template"),

		epubFile:         epubFile,
		outputDir:        cmd.String("output"),
		assetDirName:     defaultAssetDirName,
		preprocessScript: cmd.String("script"),

		jobCnt: runtime.NumCPU(),
	}

	if options.outputDir == "" {
		ext := filepath.Ext(options.epubFile)
		basename := filepath.Base(options.epubFile)
		options.outputDir = basename[:len(basename)-len(ext)]
	}

	templateFile := cmd.String("template-file")
	if options.template != "" {
		// pass
	} else if templateFile == "" {
		options.template = defaultLatexTemplte
	} else {
		data, err := os.ReadFile(templateFile)
		if err != nil {
			return options, fmt.Errorf("failed to read template file %s: %s", templateFile, err)
		}

		options.template = string(data)
	}

	return options, nil
}

func cmdMain(options options) error {
	convertOptions := latex.FromEpubOptions{
		Template:  options.template,
		OutputDir: options.outputDir,
	}

	return epub_merge.Merge(epub_merge.EpubMergeOptions{
		EpubFile:     options.epubFile,
		OutputDir:    options.outputDir,
		AssetDirName: options.assetDirName,

		JobCnt: options.jobCnt,

		PreprocessFunc: func(nodes []*html.Node) ([]*html.Node, error) {
			nodes = latex.FromEpubPreprocess(nodes, convertOptions)

			// user script
			if options.preprocessScript != "" {
				meta := latex.PreprocessMeta{}

				if processed, err := latex.RunPreprocessScript(nodes, options.preprocessScript, meta); err == nil {
					nodes = processed
				} else {
					return nil, err
				}
			}

			return nodes, nil
		},
		SaveOutputFunc: func(nodes []*html.Node, fileBasename string, author string) error {
			convertOptions.Author = author
			return latex.FromEpubSaveOutput(nodes, fileBasename, convertOptions)
		},
	})
}
