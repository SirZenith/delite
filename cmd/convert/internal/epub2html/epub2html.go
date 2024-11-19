package epub2html

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/SirZenith/delite/common/html_util"
	"github.com/SirZenith/delite/format/epub"
	format_html "github.com/SirZenith/delite/format/html"
	"github.com/SirZenith/delite/format/latex"
	"github.com/urfave/cli/v3"
	"golang.org/x/net/html"
)

const defaultAssetDirName = "assets"

const defaultHTMLTemplate = `
<html>
<head>
    <meta charset="UTF-8">
    <meta http-equiv="X-UA-Compatible" content="IE=edge">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Document</title>
</head>
<body>
</body>
</html>
`

func Cmd() *cli.Command {
	var epubFile string

	cmd := &cli.Command{
		Name:  "epub2html",
		Usage: "convert EPUB file into a single HTML file",
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
					"output template string, this should be HTML text.",
					"       By default book content will be filled into `body` tag of template,",
					"       User can specify container element by ID attribute and `html-id` flag",
				}, "\n"),
			},
			&cli.StringFlag{
				Name:  "id",
				Usage: "id of target HTML tag to fill book content to",
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
	template        string
	htmlContainerID string

	epubFile         string
	outputDir        string
	assetDirName     string
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
		options.template = defaultHTMLTemplate
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
	return epub.Merge(epub.EpubMergeOptions{
		EpubFile:     options.epubFile,
		OutputDir:    options.outputDir,
		AssetDirName: options.assetDirName,

		JobCnt: options.jobCnt,

		PreprocessFunc: func(nodes []*html.Node) ([]*html.Node, error) {
			nodes = outputPreprocess(options, nodes)

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
		SaveOutputFunc: func(nodes []*html.Node, fileBasename string, _ string) error {
			return saveOutput(options, nodes, fileBasename)
		},
	})
}

func outputPreprocess(_ options, nodes []*html.Node) []*html.Node {
	for _, node := range nodes {
		format_html.SetImageTypeMeta(node)
	}

	return nodes
}

func saveOutput(options options, nodes []*html.Node, fileBasename string) error {
	doc, container, err := parseHTMLTemplate(options.template, options.htmlContainerID)
	if err != nil {
		return err
	}

	for _, node := range nodes {
		container.AppendChild(node)
	}

	outputName := filepath.Join(options.outputDir, fileBasename+".html")
	outFile, err := os.Create(outputName)
	if err != nil {
		return fmt.Errorf("failed to write create file %s: %s", outputName, err)
	}
	defer outFile.Close()

	outWriter := bufio.NewWriter(outFile)
	html.Render(outWriter, doc)

	err = outWriter.Flush()
	if err != nil {
		return fmt.Errorf("failed to flush output file buffer: %s", err)
	}

	return nil
}

// parseHTMLTemplate parses template string into HTML tree and tries to find container
// node in it.
// When `containerID` is empty string, body tag in template will be used. If a
// container node cannot be fond, a error will be returned.
// This function returns template HTML tree, pointer to container node, and error
// happened during operation.
func parseHTMLTemplate(template string, containerID string) (*html.Node, *html.Node, error) {
	templateReader := strings.NewReader(template)
	templateDoc, err := html.Parse(templateReader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse template string: %s", err)
	}

	var container *html.Node
	if containerID == "" {
		container = html_util.FindHTMLBody(templateDoc)
	} else {
		container = html_util.FindElementByID(templateDoc, containerID)
	}

	if container == nil {
		return nil, nil, fmt.Errorf("can't find HTML body tag in template")
	}

	return templateDoc, container, nil
}
