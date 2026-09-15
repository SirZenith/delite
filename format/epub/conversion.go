package epub

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/SirZenith/delite/common/html_util"
	"github.com/SirZenith/delite/format/common"
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
	format_html.SetListLevelMeta(container, 0, false)
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
