package latex

import (
	"fmt"
	"path"
	"strings"
	"sync"

	"github.com/SirZenith/delite/common/html_util"
	"github.com/SirZenith/delite/format/common"
	"golang.org/x/net/html"
)

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
			"\\", `\textbackslash{}`,
			"$", `\$`,
			"~", `\~{}`,
			"^", `\^{}`,
			"&", `\&`,
			"_", `\_{}`,
		)
	})

	return latexEscaper.Replace(text)
}

func AddReferenceLabel(node *html.Node, contextFile string) string {
	childContextFile := contextFile
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		childContextFile = AddReferenceLabel(child, childContextFile)

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
		case strings.HasPrefix(node.Data, common.MetaCommentFileStart):
			contextFile = node.Data[len(common.MetaCommentFileStart):]
		case strings.HasPrefix(node.Data, common.MetaCommentFileEnd):
			contextFile = ""
		}
	case html.ElementNode:
		if attr := html_util.GetNodeAttr(node, "id"); attr != nil {
			refNode := &html.Node{
				Type: html.CommentNode,
				Data: fmt.Sprintf("%s%s#%s", common.MetaCommentRefAnchor, path.Base(contextFile), attr.Val),
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
