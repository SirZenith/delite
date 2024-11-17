package latex

import (
	"container/list"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode"

	"github.com/SirZenith/delite/common"
	"github.com/SirZenith/delite/common/html_util"
	format_common "github.com/SirZenith/delite/format/common"
	lua_base "github.com/SirZenith/delite/lua_module/base"
	lua_fs "github.com/SirZenith/delite/lua_module/fs"
	lua_html "github.com/SirZenith/delite/lua_module/html"
	lua_html_atom "github.com/SirZenith/delite/lua_module/html/atom"
	"github.com/charmbracelet/log"
	lua "github.com/yuin/gopher-lua"
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
			baseList = append(baseList, child.Data)
		case html.ElementNode:
			switch child.DataAtom {
			case atom.Rp:
				// ignore
			case atom.Rb:
				text := html_util.ExtractText(child)
				baseList = append(baseList, strings.Join(text, ""))
			case atom.Rt:
				text := html_util.ExtractText(child)
				annotationList = append(annotationList, strings.Join(text, ""))
			default:
				// ignore
			}
		default:
			// ignore
		}
	}

	content.Init()

	annotationCnt := len(annotationList)
	for i, text := range baseList {
		text = strings.TrimSpace(text)
		text = latexStrEscape(text)

		var anno string
		if i < annotationCnt {
			anno := strings.TrimSpace(annotationList[i])
			anno = latexStrEscape(anno)
		}

		if anno == "" {
			content.PushBack(text)
		} else {
			content.PushBack("\\ruby{")
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
						content.PushBack(strings.TrimSpace(walk.Data))
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

type PreprocessMeta struct {
	OutputBaseName string
	SourceFileName string
	Book           string
	Volume         string
	Title          string
	Author         string
}

func (meta *PreprocessMeta) toLuaTable(L *lua.LState) *lua.LTable {
	tbl := L.NewTable()

	tbl.RawSetString("output_basename", lua.LString(meta.OutputBaseName))
	tbl.RawSetString("source_filename", lua.LString(meta.SourceFileName))
	tbl.RawSetString("book", lua.LString(meta.Book))
	tbl.RawSetString("volume", lua.LString(meta.Volume))
	tbl.RawSetString("title", lua.LString(meta.Title))
	tbl.RawSetString("author", lua.LString(meta.Author))

	return tbl
}

func RunPreprocessScript(nodes []*html.Node, scriptPath string, meta PreprocessMeta) ([]*html.Node, error) {
	if _, err := os.Stat(scriptPath); err != nil {
		return nil, fmt.Errorf("failed to access script %s: %s", scriptPath, err)
	}

	L := lua.NewState()
	defer L.Close()

	// setup modules
	updateScriptImportPath(L, scriptPath)

	lua_html.RegisterNodeType(L)

	L.PreloadModule("delite", lua_base.Loader)
	L.PreloadModule("fs", lua_fs.Loader)
	L.PreloadModule("html", lua_html.Loader)
	L.PreloadModule("html-atom", lua_html_atom.Loader)

	// setup global variables
	container := &html.Node{
		Type: html.DocumentNode,
	}
	for _, node := range nodes {
		container.AppendChild(node)
	}
	L.SetGlobal("doc_node", lua_html.NewNodeUserData(L, container))

	L.SetGlobal("meta", meta.toLuaTable(L))
	L.SetGlobal("fnil", L.NewFunction(func(_ *lua.LState) int { return 0 }))

	// executation
	if err := L.DoFile(scriptPath); err != nil {
		return nil, fmt.Errorf("preprocess script executation error:\n%s", err)
	}

	// return value handling
	ud, ok := L.Get(1).(*lua.LUserData)
	if !ok {
		return nil, fmt.Errorf("preprocess script does not return a userdata")
	}

	wrapped, ok := ud.Value.(*lua_html.Node)
	if !ok {
		return nil, fmt.Errorf("preprocess script returns invalid userdata, expecting Node object")
	}

	newNodes := []*html.Node{}
	docNode := wrapped.Node
	child := docNode.FirstChild
	for child != nil {
		nextChild := child.NextSibling
		docNode.RemoveChild(child)
		newNodes = append(newNodes, child)
		child = nextChild
	}

	return newNodes, nil
}

func updateScriptImportPath(L *lua.LState, scriptPath string) error {
	pack, ok := L.GetGlobal("package").(*lua.LTable)
	if !ok {
		return fmt.Errorf("failed to retrive global variable `package`")
	}

	pathVal, ok := L.GetField(pack, "path").(lua.LString)
	if !ok {
		return fmt.Errorf("`path` field of `package` table is not a string")
	}

	path := string(pathVal)
	scriptDir := filepath.Dir(scriptPath)

	path += fmt.Sprintf(";%s/?.lua;%s/?/init.lua", scriptDir, scriptDir)
	L.SetField(pack, "path", lua.LString(path))

	return nil
}
