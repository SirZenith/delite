package html2latex

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/SirZenith/delite/common/html_util"
	"github.com/SirZenith/delite/format/latex"
	"github.com/urfave/cli/v3"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
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
	var htmlFile string

	cmd := &cli.Command{
		Name:  "html2latex",
		Usage: "convert HTML file into a LaTeX file",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "path of output file",
			},
			&cli.StringFlag{
				Name:  "script",
				Usage: "path to preprocess script",
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
	htmlFile         string
	outputName       string
	preprocessScript string

	jobCnt int
}

func getOptionsFromCmd(cmd *cli.Command, htmlFile string) (options, error) {
	options := options{
		htmlFile:         htmlFile,
		outputName:       cmd.String("output"),
		preprocessScript: cmd.String("script"),

		jobCnt: runtime.NumCPU(),
	}

	if options.outputName == "" {
		ext := filepath.Ext(options.htmlFile)
		basename := filepath.Base(options.htmlFile)
		options.outputName = basename[:len(basename)-len(ext)] + ".html"
	}

	return options, nil
}

func cmdMain(options options) error {
	file, err := os.Open(options.htmlFile)
	if err != nil {
		return fmt.Errorf("failed to open HTML file %s: %s", options.htmlFile, err)
	}
	defer file.Close()

	root, err := html.Parse(file)
	if err != nil {
		return fmt.Errorf("failed to parse HTML content: %s", err)
	}

	body := html_util.FindHTMLTag(root, html.ElementNode, atom.Body)
	if body == nil {
		return fmt.Errorf("faield to find body tag in HTML content")
	}

	nodes := []*html.Node{}
	child := body.FirstChild
	for child != nil {
		next := child.NextSibling
		nodes = append(nodes, child)
		body.RemoveChild(child)
		child = next
	}

	fileBasename := filepath.Base(options.outputName)
	convertOptions := latex.FromEpubOptions{
		Template:  defaultLatexTemplte,
		OutputDir: filepath.Dir(options.outputName),
	}

	// user script
	if options.preprocessScript != "" {
		meta := latex.PreprocessMeta{
			SourceFileName: filepath.Base(options.htmlFile),
			OutputBaseName: fileBasename,
			OutputDir:      convertOptions.OutputDir,
		}

		if processed, err := latex.RunPreprocessScript(nodes, options.preprocessScript, meta); err == nil {
			nodes = processed
		} else {
			return err
		}
	}

	return latex.FromEpubSaveOutput(nodes, fileBasename, convertOptions)
}
