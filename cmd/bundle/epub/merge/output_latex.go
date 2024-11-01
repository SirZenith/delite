package merge

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/charmbracelet/log"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type LatexConverterFunc = func(node *html.Node, contextFile string, content []string) []string
type LatexConverterMap = map[atom.Atom]LatexConverterFunc

var (
	latexEscaper     *strings.Replacer
	onceLatexEscaper sync.Once
)

func latexStrEscape(text string) string {
	onceLatexEscaper.Do(func() {
		latexEscaper = strings.NewReplacer(
			"#", `\#`,
			"%", `\%`,
			"{", `\{`,
			"}", `\}`,
			// TODO: make better back slash escaping support
			// "\\", `\\textbackslash`,
			"$", `\$`,
			"~", `\~{}`,
			"^", `\^{}`,
			"&", `\&`,
			"_", `\_{}`,
		)
	})

	return latexEscaper.Replace(text)
}

func noOptLatexConverter(_ *html.Node, _ string, content []string) []string {
	return content
}

func dropLatexConverter(_ *html.Node, _ string, content []string) []string {
	return nil
}

func surroundLatexConverter(_ *html.Node, _ string, content []string, left, right string) []string {
	if left != "" {
		content = slices.Insert(content, 0, left)
	}
	if right != "" {
		content = append(content, right)
	}
	return content
}

func makeReplaceLatexConverter(text string) LatexConverterFunc {
	return func(_ *html.Node, _ string, content []string) []string {
		return []string{text}
	}
}

func makeSurroundLatexConverter(left string, right string) LatexConverterFunc {
	return func(node *html.Node, contextFile string, content []string) []string {
		return surroundLatexConverter(node, contextFile, content, left, right)
	}
}

func makeWithAttrLatexConverter(attrName string, action func(*html.Node, string, []string, string) []string) LatexConverterFunc {
	return func(node *html.Node, contextFile string, content []string) []string {
		attr := getNodeAttr(node, attrName)
		if attr == nil {
			return nil
		}
		return action(node, contextFile, content, attr.Val)
	}
}

func getLatexStandardConverter() LatexConverterMap {
	return map[atom.Atom]LatexConverterFunc{
		atom.A: makeWithAttrLatexConverter("href", func(_ *html.Node, _ string, content []string, val string) []string {
			val = strings.ReplaceAll(val, "#", ":")
			val = latexStrEscape(val)
			if len(content) == 0 {
				return []string{"\\url{", val, "}"}
			}
			content = slices.Insert(content, 0, "\\hyperref[", val, "]{")
			content = append(content, "}")
			return content
		}),
		atom.B:          makeSurroundLatexConverter("\\textbf{", "}"),
		atom.Blockquote: makeSurroundLatexConverter("\\begin{quote}\n", "\n\\end{quote}"),
		atom.Body:       noOptLatexConverter,
		atom.Br:         makeReplaceLatexConverter("\n\n"),
		atom.Center:     makeSurroundLatexConverter("\n\\begin{center}\n", "\n\\end{center}"),
		atom.Div:        makeSurroundLatexConverter("\n\n", ""),
		atom.H1:         makeSurroundLatexConverter("\n\n\\chapter{", "}"),
		atom.H2:         makeSurroundLatexConverter("\n\n\\section{", "}"),
		atom.H3:         makeSurroundLatexConverter("\n\n\\subsection{", "}"),
		atom.H4:         makeSurroundLatexConverter("\n\n\\subsubsection{", "}"),
		atom.H5:         makeSurroundLatexConverter("\n\n\\paragraph{", "}"),
		atom.H6:         makeSurroundLatexConverter("\n\n\\subparagraph{", "}"),
		atom.Head:       dropLatexConverter,
		atom.Hr:         makeReplaceLatexConverter("\n\n"),
		atom.Html:       noOptLatexConverter,
		atom.I:          makeSurroundLatexConverter("\\textit{", "}"),
		atom.Image: makeWithAttrLatexConverter("href", func(_ *html.Node, _ string, _ []string, val string) []string {
			return []string{"\n\\includepdf{", val, "}"}
		}),
		atom.Img: makeWithAttrLatexConverter("src", func(_ *html.Node, _ string, _ []string, val string) []string {
			return []string{"\n\\includepdf{", val, "}"}
		}),
		// 'img': '\n\\includegraphics[width = \\textwidth]{ {{- tag["src"] -}} }',
		// 'img': '\n\\begin{figure}[htp]\n\\includegraphics[width = \\textwidth]{ {{- tag["src"] -}} }\n\\end{figure}',
		atom.Li:     makeSurroundLatexConverter("\n\\item ", ""),
		atom.Link:   dropLatexConverter,
		atom.Meta:   dropLatexConverter,
		atom.Ol:     makeSurroundLatexConverter("\n\\begin{enumerate}\n", "\n\\end{enumerate}"),
		atom.P:      makeSurroundLatexConverter("\n\n", ""),
		atom.Rb:     noOptLatexConverter,
		atom.Rt:     makeSurroundLatexConverter("}{", ""),
		atom.Ruby:   makeSurroundLatexConverter("\\ruby{", "}"),
		atom.Span:   noOptLatexConverter,
		atom.Strong: makeSurroundLatexConverter("\\textbf{", "}"),
		atom.Sub:    makeSurroundLatexConverter("_{", "}"),
		atom.Sup:    makeSurroundLatexConverter("^{", "}"),
		atom.Svg:    noOptLatexConverter,
		atom.Table:  noOptLatexConverter,
		atom.Tbody:  noOptLatexConverter,
		atom.Td:     noOptLatexConverter,
		atom.Title:  dropLatexConverter,
		atom.Tr:     noOptLatexConverter,
		atom.Ul:     makeSurroundLatexConverter("\n\\begin{itemize}\n", "\n\\end{itemize}"),
	}
}

func convertHTML2Latex(node *html.Node, contextFile string, converterMap LatexConverterMap) ([]string, string) {
	var content []string

	childContextFile := contextFile
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		childContent, childContextFile := convertHTML2Latex(child, childContextFile, converterMap)

		if childContent != nil {
			content = append(content, childContent...)
		}

		if childContextFile == "" {
			childContextFile = contextFile
		}
	}

	switch node.Type {
	case html.ErrorNode, html.DocumentNode, html.DoctypeNode:
		// pass
	case html.TextNode:
		content = append(content, latexStrEscape(node.Data))
	case html.CommentNode:
		switch {
		case strings.HasPrefix(node.Data, metaCommentFileStart):
			contextFile = node.Data[len(metaCommentFileStart):]
		case strings.HasPrefix(node.Data, metaCommentFileEnd):
			contextFile = ""
		case strings.HasPrefix(node.Data, metaCommentRefAnchor):
			label := node.Data[len(metaCommentRefAnchor):]
			label = strings.ReplaceAll(label, "#", ":")
			content = slices.Insert(content, 0, "\\label{", latexStrEscape(label), "}")
		}
		content = slices.Insert(content, 0, "% ", node.Data, "\n")
	case html.ElementNode:
		if checkIsDisplayNone(node) {
			content = nil
		} else if converter := converterMap[node.DataAtom]; converter == nil {
			log.Warnf("not supported tag: %q", node.Data)
		} else {
			content = converter(node, contextFile, content)
		}
	}

	return content, contextFile
}

type ForbiddenRuleMap map[atom.Atom][]atom.Atom
type ForbiddenScope map[atom.Atom]int

func getStandardFrobiddenRuleMap() ForbiddenRuleMap {
	return map[atom.Atom][]atom.Atom{
		atom.H1: {atom.Img},
	}
}

func forbiddenNodeExtraction(node *html.Node, ruleMap ForbiddenRuleMap, scope ForbiddenScope) {
	forbiddenList := ruleMap[node.DataAtom]
	for _, tag := range forbiddenList {
		scope[tag] = scope[tag] + 1
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		forbiddenNodeExtraction(child, ruleMap, scope)
	}

	if node.Type != html.ElementNode {
		return
	}

	parent := node.Parent
	if parent == nil {
		return
	}

	nextSibling := node.NextSibling
	child := node.FirstChild
	for child != nil {
		nextChild := child.NextSibling

		if scope[child.DataAtom] > 0 {
			// extract forbidden node one level upwards.
			node.RemoveChild(child)
			if nextSibling != nil {
				parent.InsertBefore(child, nextSibling)
			} else {
				parent.AppendChild(child)
			}
		}

		child = nextChild
	}

	for _, tag := range forbiddenList {
		newCnt := scope[tag] - 1
		if newCnt > 0 {
			scope[tag] = newCnt
		} else {
			delete(scope, tag)
		}
	}
}

func addReferenceLabel(node *html.Node, contextFile string) string {
	childContextFile := contextFile
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		childContextFile = addReferenceLabel(child, childContextFile)

		if childContextFile == "" {
			childContextFile = contextFile
		}
	}

	parent := node.Parent
	if parent == nil {
		return contextFile
	}

	switch node.Type {
	case html.ErrorNode, html.DocumentNode, html.DoctypeNode, html.TextNode:
	case html.CommentNode:
		switch {
		case strings.HasPrefix(node.Data, metaCommentFileStart):
			contextFile = node.Data[len(metaCommentFileStart):]
		case strings.HasPrefix(node.Data, metaCommentFileEnd):
			contextFile = ""
		}
	case html.ElementNode:
		if attr := getNodeAttr(node, "id"); attr != nil {
			refNode := &html.Node{
				Type: html.CommentNode,
				Data: fmt.Sprintf("%s%s#%s", metaCommentRefAnchor, path.Base(contextFile), attr.Val),
			}

			if node.NextSibling == nil {
				parent.AppendChild(refNode)
			} else {
				parent.InsertBefore(refNode, node.NextSibling)
			}
		}
	}

	return contextFile
}

func saveLatexOutput(options options, nodes []*html.Node, fileBasename string) (map[string]string, error) {
	container := &html.Node{
		Type:     html.ElementNode,
		DataAtom: atom.Body,
		Data:     atom.Body.String(),
	}
	for _, node := range nodes {
		container.AppendChild(node)
	}

	nameMap := make(map[string]string)
	imageReferenceRedirect(container, "", options.assetDirName, nameMap)

	forbiddenRuleMap := getStandardFrobiddenRuleMap()
	forbiddenNodeExtraction(container, forbiddenRuleMap, map[atom.Atom]int{})

	addReferenceLabel(container, "")

	outputName := filepath.Join(options.outputDir, fileBasename+".tex")
	outFile, err := os.Create(outputName)
	if err != nil {
		return nil, fmt.Errorf("failed to write create file %s: %s", outputName, err)
	}
	defer outFile.Close()

	converterMap := getLatexStandardConverter()
	content, _ := convertHTML2Latex(container, "", converterMap)

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
		return nil, fmt.Errorf("failed to flush output file buffer: %s", err)
	}

	return nameMap, nil
}
