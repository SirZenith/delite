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

func removeInvalidImageTags(node *html.Node, outputDir string) bool {
	child := node.FirstChild
	for child != nil {
		nextChild := child.NextSibling

		childOk := removeInvalidImageTags(child, outputDir)
		if !childOk {
			node.RemoveChild(child)
		}

		child = nextChild
	}

	if node.Type != html.ElementNode {
		return true
	}

	var src string

	switch node.DataAtom {
	case atom.Img:
		src, _ = html_util.GetNodeAttrVal(node, "src", "")
	case atom.Image:
		src, _ = html_util.GetNodeAttrVal(node, "href", "")
	default:
		return true
	}

	if src == "" {
		return false
	}

	filename := filepath.Join(outputDir, src)
	_, err := os.Stat(filename)

	return err == nil
}

func FromEpubPreprocess(nodes []*html.Node, options FromEpubOptions) []*html.Node {
	container := &html.Node{
		Type:     html.ElementNode,
		DataAtom: atom.Body,
		Data:     atom.Body.String(),
	}
	for _, node := range nodes {
		container.AppendChild(node)
	}

	removeInvalidImageTags(container, options.OutputDir)

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

	for ele := content.Front(); ele != nil; ele = ele.Next() {
		outWriter.WriteString(ele.Value.(string))
	}

	fmt.Fprintln(outWriter, "\\end{document}")

	err = outWriter.Flush()
	if err != nil {
		return fmt.Errorf("failed to flush output file buffer: %s", err)
	}

	return nil
}
