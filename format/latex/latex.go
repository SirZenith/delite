package latex

import (
	"fmt"
	"path"
	"strings"
	"sync"

	"github.com/SirZenith/delite/common/html_util"
	"github.com/SirZenith/delite/format/common"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
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

type ForbiddenRuleMap map[atom.Atom][]atom.Atom
type ForbiddenScope map[atom.Atom]int

func GetStandardFrobiddenRuleMap() ForbiddenRuleMap {
	return map[atom.Atom][]atom.Atom{
		atom.H1: {atom.Img},
	}
}

func ForbiddenNodeExtraction(node *html.Node, ruleMap ForbiddenRuleMap, scope ForbiddenScope) {
	forbiddenList := ruleMap[node.DataAtom]
	for _, tag := range forbiddenList {
		scope[tag] = scope[tag] + 1
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		ForbiddenNodeExtraction(child, ruleMap, scope)
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
