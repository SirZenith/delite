package merge

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"

	format_html "github.com/SirZenith/delite/format/html"
	"github.com/SirZenith/delite/format/latex"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func latexOutputPreprocess(_ options, nodes []*html.Node) []*html.Node {
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

func saveLatexOutput(options options, nodes []*html.Node, fileBasename string) error {
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
	fmt.Fprintln(outWriter, "\\pagenumbering{gobble}")
	fmt.Fprintln(outWriter, "\\maketitle")

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
