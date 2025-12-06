package latex

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/SirZenith/delite/common/html_util"
	format_html "github.com/SirZenith/delite/format/html"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type FromEpubOptions struct {
	Template     string
	OutputDir    string
	IsHorizontal bool

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

// replaceDashesWithPipe replace all horizontal dashes with tategaki friendly
// dashes.
func replaceDashesForTategaki(root *html.Node) {
	args := &html_util.NodeMatchArgs{
		Root: root,
		Type: map[html.NodeType]bool{
			html.TextNode: true,
		},
	}

	match := html_util.FindNextMatchingNode(root, args)
	args.LastMatch = match
	for match != nil {
		match.Data = strings.ReplaceAll(match.Data, "——", "──")

		match = html_util.FindNextMatchingNode(match, args)
		args.LastMatch = match
	}
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

	format_html.UnescapleAllTextNode(container)
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

	// distinguishing layout mode
	var converterMap HTMLConverterMap
	if options.IsHorizontal {
		converterMap = GetLatexStandardConverter()
	} else {
		replaceDashesForTategaki(container)
		converterMap = GetLatexTategakiConverter()
	}

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
	fmt.Fprintf(outWriter, "\\title{%s}\n", latexStrEscape(options.Title))
	fmt.Fprintf(outWriter, "\\author{%s}\n", latexStrEscape(options.Author))
	fmt.Fprintf(outWriter, "\\date{%s}\n", latexStrEscape(options.Date))
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
