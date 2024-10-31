package merge

import (
	"bufio"
	"fmt"
	"os"
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
		atom.A: makeWithAttrLatexConverter("href", func(_ *html.Node, _ string, _ []string, val string) []string {
			return []string{"\\url{", val, "}"}
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
		content = slices.Insert(content, 0, "% "+node.Data+"\n")

		switch {
		case strings.HasPrefix(node.Data, fileStartCommentPrefix):
			contextFile = node.Data[len(fileStartCommentPrefix):]
		case strings.HasPrefix(node.Data, fileEndCommentPrefix):
			contextFile = ""
		}
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

	outputName := filepath.Join(options.outputDir, fileBasename+".tex")
	outFile, err := os.Create(outputName)
	if err != nil {
		return nil, fmt.Errorf("failed to write create file %s: %s", outputName, err)
	}
	defer outFile.Close()

	converterMap := getLatexStandardConverter()
	content, _ := convertHTML2Latex(container, "", converterMap)

	outWriter := bufio.NewWriter(outFile)
	templateParts := strings.SplitN(options.template, latextTemplatePlaceHolder, 2)

	if len(templateParts) > 0 {
		outWriter.WriteString(templateParts[0])
	}

	for _, segment := range content {
		outWriter.WriteString(segment)
	}

	if len(templateParts) > 1 {
		outWriter.WriteString(templateParts[1])
	}

	err = outWriter.Flush()
	if err != nil {
		return nil, fmt.Errorf("failed to flush output file buffer: %s", err)
	}

	return nameMap, nil
}
