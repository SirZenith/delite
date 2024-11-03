package latex

import (
	"path"
	"slices"
	"strings"

	"github.com/SirZenith/delite/common/html_util"
	"github.com/SirZenith/delite/format/common"
	"github.com/charmbracelet/log"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type HTMLConverterFunc = func(node *html.Node, contextFile string, content []string) []string
type HTMLConverterMap = map[atom.Atom]HTMLConverterFunc

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

func headingNodeConverter(node *html.Node, contextFile string, content []string, tocLevel string) []string {
	tocParts := []string{
		"\\addcontentsline{toc}{", tocLevel, "}{",
	}
	tocParts = append(tocParts, content...)
	tocParts = append(tocParts, "}")
	content = surroundLatexConverter(node, contextFile, content, "\n\n\\"+tocLevel+"*{", "}")
	content = append(content, tocParts...)
	return content
}

func imageNodeConverter(node *html.Node, srcPath string, grphicOptions string) []string {
	if path.Ext(srcPath) == ".gif" {
		return nil
	}

	if grphicOptions != "" && grphicOptions[0] != ',' {
		grphicOptions = ", " + grphicOptions
	}

	imgType := html_util.GetNodeAttrVal(node, common.MetaAttrImageType, common.ImageTypeUnknown)

	switch imgType {
	case common.ImageTypeInline:
		return []string{"\\raisebox{-0.5\\height}{\\includegraphics[height = 0.66\\baselineskip", grphicOptions, "]{", srcPath, "}}"}
	case common.ImageTypeStandalone:
		return []string{"\\afterpage{\\includepdf{", srcPath, "}}"}
	case common.ImageTypeWidthOverflow:
		return []string{"\\includegraphics[width = \\textwidth,", grphicOptions, "]{", srcPath, "}"}
	case common.ImageTypeHeightOverflow:
		return []string{"\\includegraphics[height = \\textheight", grphicOptions, "]{", srcPath, "}"}
	default:
		return []string{
			"\\begin{figure}[htp]\n",
			"    \\includegraphics[", grphicOptions, "]{", srcPath, "}\n",
			"\\end{figure}",
		}
	}
}

func makeReplaceLatexConverter(text string) HTMLConverterFunc {
	return func(_ *html.Node, _ string, content []string) []string {
		return []string{text}
	}
}

func makeSurroundLatexConverter(left string, right string) HTMLConverterFunc {
	return func(node *html.Node, contextFile string, content []string) []string {
		return surroundLatexConverter(node, contextFile, content, left, right)
	}
}

func makeHeadingNodeConverter(tocLevel string) HTMLConverterFunc {
	return func(node *html.Node, contextFile string, content []string) []string {
		return headingNodeConverter(node, contextFile, content, tocLevel)
	}
}

func makeWithAttrLatexConverter(attrName string, action func(*html.Node, string, []string, string) []string) HTMLConverterFunc {
	return func(node *html.Node, contextFile string, content []string) []string {
		attr := html_util.GetNodeAttr(node, attrName)
		if attr == nil {
			return nil
		}
		return action(node, contextFile, content, attr.Val)
	}
}

func GetLatexStandardConverter() HTMLConverterMap {
	return map[atom.Atom]HTMLConverterFunc{
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
		atom.H1:         makeHeadingNodeConverter("chapter"),
		atom.H2:         makeHeadingNodeConverter("section"),
		atom.H3:         makeHeadingNodeConverter("subsection"),
		atom.H4:         makeHeadingNodeConverter("subsubsection"),
		atom.H5:         makeHeadingNodeConverter("paragraph"),
		atom.H6:         makeHeadingNodeConverter("subparagraph"),
		atom.Head:       dropLatexConverter,
		atom.Hr:         makeReplaceLatexConverter("\n\n"),
		atom.Html:       noOptLatexConverter,
		atom.I:          makeSurroundLatexConverter("\\textit{", "}"),
		atom.Image: makeWithAttrLatexConverter("href", func(node *html.Node, _ string, _ []string, val string) []string {
			return imageNodeConverter(node, val, "")
		}),
		atom.Img: makeWithAttrLatexConverter("src", func(node *html.Node, _ string, _ []string, val string) []string {
			return imageNodeConverter(node, val, "")
		}),
		atom.Li:     makeSurroundLatexConverter("\n\\item ", ""),
		atom.Link:   dropLatexConverter,
		atom.Meta:   dropLatexConverter,
		atom.Ol:     makeSurroundLatexConverter("\n\\begin{enumerate}\n", "\n\\end{enumerate}"),
		atom.P:      makeSurroundLatexConverter("\n\n", ""),
		atom.Rb:     noOptLatexConverter,
		atom.Rp:     dropLatexConverter,
		atom.Rt:     makeSurroundLatexConverter("}{", ""),
		atom.Ruby:   makeSurroundLatexConverter("\\ruby{", "}"),
		atom.Small:  makeSurroundLatexConverter("{\\small", "}"),
		atom.Span:   noOptLatexConverter,
		atom.Strong: makeSurroundLatexConverter("\\textbf{", "}"),
		atom.Sub:    makeSurroundLatexConverter("$_{", "}$"),
		atom.Sup:    makeSurroundLatexConverter("$^{", "}$"),
		atom.Svg:    noOptLatexConverter,
		atom.Table:  noOptLatexConverter,
		atom.Tbody:  noOptLatexConverter,
		atom.Td:     noOptLatexConverter,
		atom.Title:  dropLatexConverter,
		atom.Tr:     noOptLatexConverter,
		atom.Ul:     makeSurroundLatexConverter("\n\\begin{itemize}\n", "\n\\end{itemize}"),
	}
}

func GetLatexTategakiConverter() HTMLConverterMap {
	cvMap := GetLatexStandardConverter()
	cvMap[atom.Image] = makeWithAttrLatexConverter("href", func(node *html.Node, _ string, _ []string, val string) []string {
		return imageNodeConverter(node, val, "angle = 90")
	})
	cvMap[atom.Img] = makeWithAttrLatexConverter("src", func(node *html.Node, _ string, _ []string, val string) []string {
		return imageNodeConverter(node, val, "angle = 90")
	})
	return cvMap
}

func ConvertHTML2Latex(node *html.Node, contextFile string, converterMap HTMLConverterMap) ([]string, string) {
	var content []string

	childContextFile := contextFile
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		childContent, childContextFile := ConvertHTML2Latex(child, childContextFile, converterMap)

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
		case strings.HasPrefix(node.Data, common.MetaCommentFileStart):
			contextFile = node.Data[len(common.MetaCommentFileStart):]
		case strings.HasPrefix(node.Data, common.MetaCommentFileEnd):
			contextFile = ""
		case strings.HasPrefix(node.Data, common.MetaCommentRefAnchor):
			label := node.Data[len(common.MetaCommentRefAnchor):]
			label = strings.ReplaceAll(label, "#", ":")
			content = slices.Insert(content, 0, "\\label{", latexStrEscape(label), "}")
		}
		content = slices.Insert(content, 0, "% ", node.Data, "\n")
	case html.ElementNode:
		if html_util.CheckIsDisplayNone(node) {
			content = nil
		} else if converter := converterMap[node.DataAtom]; converter == nil {
			log.Warnf("not supported tag: %q", node.Data)
		} else {
			content = converter(node, contextFile, content)
		}
	}

	return content, contextFile
}
