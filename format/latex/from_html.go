package latex

import (
	"container/list"
	"net/url"
	"path"
	"regexp"
	"strings"
	"sync"
	"unicode"

	"github.com/SirZenith/delite/common"
	"github.com/SirZenith/delite/common/html_util"
	format_common "github.com/SirZenith/delite/format/common"
	"github.com/charmbracelet/log"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type HTMLConverterFunc = func(node *html.Node, contextFile string, content *list.List) *list.List
type HTMLConverterMap = map[atom.Atom]HTMLConverterFunc

var (
	htmlCrossRefEscaper     *strings.Replacer
	onceHtmlCrossRefEscaper sync.Once
)

func htmlCrossRefStrEscape(text string) string {
	onceHtmlCrossRefEscaper.Do(func() {
		htmlCrossRefEscaper = strings.NewReplacer(
			"#", `:`,
		)
	})

	return htmlCrossRefEscaper.Replace(text)
}

func convertCommentNode(node *html.Node, contextFile string, content *list.List) *list.List {
	switch {
	case strings.HasPrefix(node.Data, format_common.MetaCommentFileStart):
		contextFile = node.Data[len(format_common.MetaCommentFileStart):]
		common.ListBatchPushFront(content, "% ", node.Data, "\n")
	case strings.HasPrefix(node.Data, format_common.MetaCommentFileEnd):
		contextFile = ""
		common.ListBatchPushFront(content, "% ", node.Data, "\n")
	case strings.HasPrefix(node.Data, format_common.MetaCommentRefAnchor):
		label := node.Data[len(format_common.MetaCommentRefAnchor):]
		label = htmlCrossRefStrEscape(label)
		common.ListBatchPushFront(content, "\\label{", label, "}")
	case strings.HasPrefix(node.Data, format_common.MetaCommentRawText):
		common.ListBatchPushFront(content, node.Data[len(format_common.MetaCommentRawText):])
	}

	return content
}

func noOptLatexConverter(_ *html.Node, _ string, content *list.List) *list.List {
	return content
}

func dropLatexConverter(_ *html.Node, _ string, content *list.List) *list.List {
	return content.Init()
}

func surroundLatexConverter(_ *html.Node, _ string, content *list.List, left, right string) *list.List {
	if left != "" {
		content.PushFront(left)
	}
	if right != "" {
		content.PushBack(right)
	}
	return content
}

func surroundEachLineLatexConverter(_ *html.Node, _ string, content *list.List, left, right string) *list.List {
	skipRight := false

	for elem := content.Front(); elem != nil; elem = elem.Next() {
		segment := elem.Value.(string)
		if segment == "" {
			continue
		}

		parts := strings.Split(segment, "\n")
		elem.Value = parts[0]

		for i := 1; i < len(parts); i++ {
			if right != "" && !skipRight {
				elem = content.InsertAfter(right, elem)
			}

			elem = content.InsertAfter("\n", elem)

			if strings.TrimSpace(parts[i]) == "" {
				skipRight = true
				continue
			}

			skipRight = false

			if left != "" {
				elem = content.InsertAfter(left, elem)
			}

			elem = content.InsertAfter(parts[i], elem)
		}
	}

	if left != "" {
		content.PushFront(left)
	}
	if right != "" {
		content.PushBack(right)
	}

	return content
}

func trimSpaceLatexConverter(_ *html.Node, _ string, content *list.List) *list.List {
	if front := content.Front(); front != nil {
		front.Value = strings.TrimLeftFunc(front.Value.(string), unicode.IsSpace)
	}
	if back := content.Back(); back != nil {
		back.Value = strings.TrimRightFunc(back.Value.(string), unicode.IsSpace)
	}
	return content
}

func trimSpaceEachElementLatexConverter(_ *html.Node, _ string, content *list.List) *list.List {
	for elem := content.Front(); elem != nil; elem = elem.Next() {
		elem.Value = strings.TrimSpace(elem.Value.(string))
	}
	return content
}

var (
	patternMultipleWhitespace     *regexp.Regexp
	oncePatternMultipleWhitespace sync.Once
)

// replaceMultipleSpaceConverter replaces multiple white spaces and new line with single space.
func replaceMultipleSpaceConverter(_ *html.Node, _ string, content *list.List) *list.List {
	oncePatternMultipleWhitespace.Do(func() {
		patternMultipleWhitespace = regexp.MustCompile(`\s+`)
	})

	flagPostPone := false
	for elem := content.Front(); elem != nil; elem = elem.Next() {
		segment := elem.Value.(string)
		if segment == "" {
			continue
		}

		parts := patternMultipleWhitespace.Split(segment, -1)
		totalCnt := len(parts)

		if flagPostPone {
			content.InsertBefore(" ", elem)
			flagPostPone = false
		}

		elem.Value = parts[0]

		for i := 1; i < totalCnt; i++ {
			if parts[i] == "" {
				flagPostPone = true
				continue
			}

			elem = content.InsertAfter(" ", elem)
			flagPostPone = false

			elem = content.InsertAfter(parts[i], elem)
		}
	}

	return content
}

func headingNodeConverter(_ *html.Node, _ string, content *list.List, tocLevel string) *list.List {
	buffer := make([]string, 0, content.Len())
	for ele := content.Front(); ele != nil; ele = ele.Next() {
		buffer = append(buffer, ele.Value.(string))
	}

	title := strings.Join(buffer, "")
	title = strings.TrimSpace(title)
	title = strings.ReplaceAll(title, "\n", "")

	content.Init()

	common.ListBatchPushBack(
		content,
		"\n\n\\", tocLevel, "*{", title, "}",
		"\\addcontentsline{toc}{", tocLevel, "}{", title, "}",
	)

	return content
}

func imageNodeConverter(node *html.Node, srcPath string, graphicOptions ...string) *list.List {
	srcPath, err := url.PathUnescape(srcPath)
	if err != nil {
		return nil
	}

	if path.Ext(srcPath) == ".gif" {
		return nil
	}

	attr := html_util.GetNodeAttr(node, format_common.MetaAttrImageGraphicOption)
	if attr != nil {
		graphicOptions = append(graphicOptions, attr.Val)
	}

	imgType, _ := html_util.GetNodeAttrVal(node, format_common.MetaAttrImageType, format_common.ImageTypeUnknown)

	content := list.New()

	switch imgType {
	case format_common.ImageTypeInline:
		graphicOptions = append(graphicOptions, "height = 0.66\\baselineskip")
		common.ListBatchPushBack(content, "\\raisebox{-0.5\\height}{\\includegraphics[", strings.Join(graphicOptions, ", "), "]{", srcPath, "}}")
	case format_common.ImageTypeStandalone:
		common.ListBatchPushBack(content, "\\afterpage{\\includepdf{", srcPath, "}}")
	case format_common.ImageTypeWidthOverflow:
		graphicOptions = append(graphicOptions, "width = \\textwidth")
		common.ListBatchPushBack(content, "\\includegraphics[", strings.Join(graphicOptions, ", "), "]{", srcPath, "}")
	case format_common.ImageTypeHeightOverflow:
		graphicOptions = append(graphicOptions, "height = \\textheight")
		common.ListBatchPushBack(content, "\\includegraphics[", strings.Join(graphicOptions, ", "), "]{", srcPath, "}")
	case format_common.ImageTypeHere:
		common.ListBatchPushBack(content, "\\includegraphics[", strings.Join(graphicOptions, ", "), "]{", srcPath, "}")
	default:
		common.ListBatchPushBack(content,
			"\\begin{figure}[htp]\n",
			"    \\includegraphics[", strings.Join(graphicOptions, ", "), "]{", srcPath, "}\n",
			"\\end{figure}",
		)
	}

	return content
}

func rubyNodeConverter(node *html.Node, _ string, content *list.List) *list.List {
	baseList := []string{}
	annotationList := []string{}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		switch child.Type {
		case html.TextNode:
			if child.Data != "" {
				baseList = append(baseList, latexStrEscape(child.Data))
			}
		case html.ElementNode:
			switch child.DataAtom {
			case atom.Rp:
				// ignore
			case atom.Rb:
				textList := html_util.ExtractText(child)
				text := latexStrEscape(strings.Join(textList, ""))
				baseList = append(baseList, text)
			case atom.Rt:
				textList := html_util.ExtractText(child)
				text := latexStrEscape(strings.Join(textList, ""))
				annotationList = append(annotationList, text)
			default:
				textList := html_util.ExtractText(child)
				text := latexStrEscape(strings.Join(textList, ""))
				baseList = append(baseList, text)
			}
		default:
			// ignore
		}
	}

	content.Init()

	baseCnt := len(baseList)
	annotationCnt := len(annotationList)
	partCnt := max(annotationCnt, baseCnt)

	rubyType := html_util.GetNodeAttr(node, format_common.MetaRubyType)

	for i := range partCnt {
		var text string
		if i < baseCnt {
			text = strings.TrimSpace(baseList[i])
			text = latexStrEscape(text)
		}

		var anno string
		if i < annotationCnt {
			anno = strings.TrimSpace(annotationList[i])
			anno = latexStrEscape(anno)
		}

		if anno == "" {
			content.PushBack(text)
		} else {
			if rubyType != nil {
				content.PushBack("\\ruby[")
				content.PushBack(rubyType.Val)
				content.PushBack("]{")
			} else {
				content.PushBack("\\ruby{")
			}
			content.PushBack(text)
			content.PushBack("}")

			content.PushBack("{")
			content.PushBack(anno)
			content.PushBack("}")
		}

	}

	return content
}

func chainConverter(converters ...HTMLConverterFunc) HTMLConverterFunc {
	return func(node *html.Node, contextFile string, content *list.List) *list.List {
		for _, converter := range converters {
			content = converter(node, contextFile, content)
		}
		return content
	}
}

func makeReplaceLatexConverter(text string) HTMLConverterFunc {
	return func(_ *html.Node, _ string, content *list.List) *list.List {
		content.Init().PushBack(text)
		return content
	}
}

func makeSurroundLatexConverter(left, right string) HTMLConverterFunc {
	return func(node *html.Node, contextFile string, content *list.List) *list.List {
		return surroundLatexConverter(node, contextFile, content, left, right)
	}
}

func makeSurroundEachLineLatexConverter(left, right string) HTMLConverterFunc {
	return func(node *html.Node, contextFile string, content *list.List) *list.List {
		return surroundEachLineLatexConverter(node, contextFile, content, left, right)
	}
}

func makeHeadingNodeConverter(tocLevel string) HTMLConverterFunc {
	return func(node *html.Node, contextFile string, content *list.List) *list.List {
		return headingNodeConverter(node, contextFile, content, tocLevel)
	}
}

func makeWithAttrLatexConverter(attrName string, action func(node *html.Node, contextFile string, content *list.List, attrVal string) *list.List) HTMLConverterFunc {
	return func(node *html.Node, contextFile string, content *list.List) *list.List {
		attr := html_util.GetNodeAttr(node, attrName)
		if attr == nil {
			return nil
		}
		return action(node, contextFile, content, attr.Val)
	}
}

func GetLatexStandardConverter() HTMLConverterMap {
	return map[atom.Atom]HTMLConverterFunc{
		atom.Aside: chainConverter(
			func(node *html.Node, contextFile string, content *list.List) *list.List {
				content.Init()

				walk := node.FirstChild
				for walk != nil {
					if walk.FirstChild != nil {
						walk = walk.FirstChild
						continue
					}

					switch walk.Type {
					case html.TextNode:
						text := strings.TrimSpace(walk.Data)
						text = latexStrEscape(text)
						content.PushBack(text)
					case html.CommentNode:
						// ignore comment found in aside tag
						// content = convertCommentNode(walk, contextFile, content)
					}

					if walk.NextSibling != nil {
						walk = walk.NextSibling
					} else if walk.Parent != nil && walk.Parent != node {
						walk = walk.Parent.NextSibling
					} else {
						break
					}
				}

				return content
			},
			replaceMultipleSpaceConverter,
			makeSurroundLatexConverter("\\footnote{", "}"),
		),
		atom.A: makeWithAttrLatexConverter("href", func(_ *html.Node, _ string, content *list.List, val string) *list.List {
			parsedVal, _ := url.Parse(val)

			if content.Len() == 0 {
				val = htmlCrossRefStrEscape(val)
				common.ListBatchPushBack(content, "\\url{", val, "}")
			} else if parsedVal.Scheme == "" {
				val = htmlCrossRefStrEscape(val)
				common.ListBatchPushFront(content, "\\hyperref[", val, "]{")
				content.PushBack("}")
			} else {
				// external reference
				common.ListBatchPushFront(content, "\\href{", val, "}{")
				content.PushBack("}")
			}

			return content
		}),
		atom.B: chainConverter(
			replaceMultipleSpaceConverter,
			makeSurroundLatexConverter("\\textbf{", "}"),
		),
		atom.Blockquote: makeSurroundLatexConverter("\\begin{quote}\n", "\n\\end{quote}"),
		atom.Big: chainConverter(
			replaceMultipleSpaceConverter,
			makeSurroundLatexConverter("{\\large ", "}"),
		),
		atom.Body:   noOptLatexConverter,
		atom.Br:     makeReplaceLatexConverter("\n\n"),
		atom.Center: makeSurroundLatexConverter("\n\\begin{center}\n", "\n\\end{center}"),
		atom.Dl:     noOptLatexConverter,
		atom.Dt: chainConverter(
			replaceMultipleSpaceConverter,
			makeSurroundLatexConverter("\\textbf{", "}"),
		),
		atom.Dd: chainConverter(
			replaceMultipleSpaceConverter,
			makeSurroundLatexConverter("\\textit{", "}"),
		),
		atom.Div:  makeSurroundLatexConverter("\n\n", ""),
		atom.Em:   makeSurroundLatexConverter("\\emph{", "}"),
		atom.Font: noOptLatexConverter,
		atom.H1:   makeHeadingNodeConverter("chapter"),
		atom.H2:   makeHeadingNodeConverter("section"),
		atom.H3:   makeHeadingNodeConverter("subsection"),
		atom.H4:   makeHeadingNodeConverter("subsubsection"),
		atom.H5:   makeHeadingNodeConverter("paragraph"),
		atom.H6:   makeHeadingNodeConverter("subparagraph"),
		atom.Head: dropLatexConverter,
		atom.Hr:   makeReplaceLatexConverter("\n\n"),
		atom.Html: noOptLatexConverter,
		atom.I: chainConverter(
			replaceMultipleSpaceConverter,
			makeSurroundLatexConverter("\\textit{", "}"),
		),
		atom.Image: makeWithAttrLatexConverter("href", func(node *html.Node, _ string, _ *list.List, val string) *list.List {
			return imageNodeConverter(node, val, "")
		}),
		atom.Img: makeWithAttrLatexConverter("src", func(node *html.Node, _ string, _ *list.List, val string) *list.List {
			return imageNodeConverter(node, val, "")
		}),
		atom.Li: chainConverter(
			replaceMultipleSpaceConverter,
			makeSurroundLatexConverter("\n\\item ", ""),
		),
		atom.Link: dropLatexConverter,
		atom.Meta: dropLatexConverter,
		atom.Ol:   makeSurroundLatexConverter("\n\\begin{enumerate}\n", "\n\\end{enumerate}"),
		atom.P: chainConverter(
			replaceMultipleSpaceConverter,
			makeSurroundLatexConverter("\n\n", ""),
		),
		atom.Rb:     noOptLatexConverter,
		atom.Rp:     noOptLatexConverter,
		atom.Rt:     noOptLatexConverter,
		atom.Ruby:   rubyNodeConverter,
		atom.Script: dropLatexConverter,
		atom.Small:  makeSurroundLatexConverter("{\\small ", "}"),
		atom.Span:   noOptLatexConverter,
		atom.Strong: chainConverter(
			replaceMultipleSpaceConverter,
			makeSurroundLatexConverter("\\textbf{", "}"),
		),
		atom.Sub: chainConverter(
			replaceMultipleSpaceConverter,
			makeSurroundLatexConverter("$_{", "}$"),
		),
		atom.Sup: chainConverter(
			replaceMultipleSpaceConverter,
			makeSurroundLatexConverter("$^{", "}$"),
		),
		atom.Svg:   noOptLatexConverter,
		atom.Table: noOptLatexConverter,
		atom.Tbody: noOptLatexConverter,
		atom.Td:    noOptLatexConverter,
		atom.Title: dropLatexConverter,
		atom.Tr:    noOptLatexConverter,
		atom.Ul:    makeSurroundLatexConverter("\n\\begin{itemize}\n", "\n\\end{itemize}"),
	}
}

func GetLatexTategakiConverter() HTMLConverterMap {
	cvMap := GetLatexStandardConverter()

	cvMap[atom.Center] = noOptLatexConverter
	cvMap[atom.Em] = chainConverter(
		replaceMultipleSpaceConverter,
		makeSurroundLatexConverter("\\kenten{", "}"),
	)
	cvMap[atom.Image] = makeWithAttrLatexConverter("href", func(node *html.Node, _ string, _ *list.List, val string) *list.List {
		return imageNodeConverter(node, val, "angle = 90")
	})
	cvMap[atom.Img] = makeWithAttrLatexConverter("src", func(node *html.Node, _ string, _ *list.List, val string) *list.List {
		return imageNodeConverter(node, val, "angle = 90")
	})

	return cvMap
}

func ConvertHTML2Latex(node *html.Node, contextFile string, converterMap HTMLConverterMap) (*list.List, string) {
	content := list.New()

	childContextFile := contextFile
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		childContent, childContextFile := ConvertHTML2Latex(child, childContextFile, converterMap)

		if childContent != nil {
			content.PushBackList(childContent)
		}

		if childContextFile == "" {
			childContextFile = contextFile
		}
	}

	switch node.Type {
	case html.ErrorNode, html.DocumentNode, html.DoctypeNode:
		// pass
	case html.TextNode:
		content.PushBack(latexStrEscape(node.Data))
	case html.CommentNode:
		content = convertCommentNode(node, contextFile, content)
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
