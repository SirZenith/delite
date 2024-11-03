package epub2latex

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/SirZenith/delite/cmd/convert/common/epub_merge"
	format_html "github.com/SirZenith/delite/format/html"
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
	return epub_merge.Merge(epub_merge.EpubMergeOptions{
		EpubFile:         options.epubFile,
		OutputDir:        options.outputDir,
		AssetDirName:     options.assetDirName,
		PreprocessScript: options.preprocessScript,

		JobCnt: options.jobCnt,

		PreprocessFunc: func(nodes []*html.Node) []*html.Node {
			return outputPreprocess(options, nodes)
		},
		SaveOutputFunc: func(nodes []*html.Node, fileBasename string) error {
			return saveOutput(options, nodes, fileBasename)
		},
	})
}

func outputPreprocess(_ options, nodes []*html.Node) []*html.Node {
	container := &html.Node{
		Type:     html.ElementNode,
		DataAtom: atom.Body,
		Data:     atom.Body.String(),
	}
	for _, node := range nodes {
		container.AppendChild(node)
	}

	forbiddenRuleMap := latex.GetStandardFrobiddenRuleMap()
	latex.ForbiddenNodeExtraction(container, forbiddenRuleMap, map[atom.Atom]int{})

	latex.AddReferenceLabel(container, "")

	format_html.SetImageTypeMeta(container)

	newNodes := make([]*html.Node, 0, len(nodes))
	child := container.FirstChild
	for child != nil {
		nextChild := child.NextSibling
		newNodes = append(newNodes, child)
		container.RemoveChild(child)
		child = nextChild
	}

	return newNodes
}

func saveOutput(options options, nodes []*html.Node, fileBasename string) error {
	container := &html.Node{
		Type:     html.ElementNode,
		DataAtom: atom.Body,
		Data:     atom.Body.String(),
	}
	for _, node := range nodes {
		container.AppendChild(node)
	}

	converterMap := latex.GetLatexTategakiConverter()
	content, _ := latex.ConvertHTML2Latex(container, "", converterMap)

	// write output file
	outputName := filepath.Join(options.outputDir, fileBasename+".tex")
	outFile, err := os.Create(outputName)
	if err != nil {
		return fmt.Errorf("failed to write create file %s: %s", outputName, err)
	}
	defer outFile.Close()

	outWriter := bufio.NewWriter(outFile)
	fmt.Fprintln(outWriter, options.template)
	fmt.Fprintf(outWriter, "\\title{%s}\n", "")
	fmt.Fprintf(outWriter, "\\author{%s}\n", "")
	fmt.Fprintf(outWriter, "\\date{%s}\n", "")
	fmt.Fprint(outWriter, "\n")
	fmt.Fprintln(outWriter, "\\begin{document}")
	fmt.Fprintln(outWriter, "\\maketitle")
	fmt.Fprintln(outWriter, "\\large")

	for _, segment := range content {
		outWriter.WriteString(segment)
	}

	fmt.Fprintln(outWriter, "\\end{document}")

	err = outWriter.Flush()
	if err != nil {
		return fmt.Errorf("failed to flush output file buffer: %s", err)
	}

	return nil
}
