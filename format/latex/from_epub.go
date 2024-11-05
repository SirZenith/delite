package latex

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"

	"github.com/SirZenith/delite/common/html_util"
	format_html "github.com/SirZenith/delite/format/html"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type FromEpubOptions struct {
	Template  string
	OutputDir string

	Title  string
	Author string
	Date   string
}

func FromEpubPreprocess(nodes []*html.Node, _ FromEpubOptions) []*html.Node {
	container := &html.Node{
		Type:     html.ElementNode,
		DataAtom: atom.Body,
		Data:     atom.Body.String(),
	}
	for _, node := range nodes {
		container.AppendChild(node)
	}

	forbiddenRuleMap := html_util.GetLatexStandardFrobiddenRuleMap()
	html_util.ForbiddenNodeExtraction(container, forbiddenRuleMap, map[atom.Atom]int{})

	AddReferenceLabel(container, "")

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

func FromEpubSaveOutput(nodes []*html.Node, fileBasename string, options FromEpubOptions) error {
	container := &html.Node{
		Type:     html.ElementNode,
		DataAtom: atom.Body,
		Data:     atom.Body.String(),
	}
	for _, node := range nodes {
		container.AppendChild(node)
	}

	converterMap := GetLatexTategakiConverter()
	content, _ := ConvertHTML2Latex(container, "", converterMap)

	// write output file
	outputName := filepath.Join(options.OutputDir, fileBasename+".tex")
	outFile, err := os.Create(outputName)
	if err != nil {
		return fmt.Errorf("failed to write create file %s: %s", outputName, err)
	}
	defer outFile.Close()

	outWriter := bufio.NewWriter(outFile)
	fmt.Fprintln(outWriter, options.Template)
	fmt.Fprintf(outWriter, "\\title{%s}\n", options.Title)
	fmt.Fprintf(outWriter, "\\author{%s}\n", options.Author)
	fmt.Fprintf(outWriter, "\\date{%s}\n", options.Date)
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
