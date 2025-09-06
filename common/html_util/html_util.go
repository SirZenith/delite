package html_util

import (
	"bufio"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func FindHTMLTag(root *html.Node, nodeType html.NodeType, tagName atom.Atom) *html.Node {
	if root.Type == nodeType && root.DataAtom == tagName {
		return root
	}

	var result *html.Node
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		result = FindHTMLTag(child, nodeType, tagName)
		if result != nil {
			break
		}
	}

	return result
}

func FindElementByID(root *html.Node, id string) *html.Node {
	if root.Type == html.ElementNode {
		for _, attr := range root.Attr {
			if attr.Key == "id" && attr.Val == id {
				return root
			}
		}
	}

	var result *html.Node
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		result = FindElementByID(child, id)
		if result != nil {
			break
		}
	}

	return result
}

func GetNodeAttr(node *html.Node, attrName string) *html.Attribute {
	var result *html.Attribute

	for i := range node.Attr {
		attr := &node.Attr[i]
		if attr.Key == attrName {
			result = attr
			break
		}
	}

	return result
}

// GetNodeAttrVal returns value of specified attreibute. If such attribute cannot
// be found, this function will return `defaultValue` instead.
func GetNodeAttrVal(node *html.Node, attrName string, defaultValue string) (string, bool) {
	if attr := GetNodeAttr(node, attrName); attr != nil {
		return attr.Val, true
	} else {
		return defaultValue, false
	}
}

func CheckIsDisplayNone(node *html.Node) bool {
	style := GetNodeAttr(node, "style")
	if style == nil {
		return false
	}

	isDisplayNone := false
	statements := strings.Split(style.Val, ";")
	for _, statement := range statements {
		parts := strings.SplitN(statement, ":", 2)
		if len(parts) != 2 {
			continue
		} else if strings.ToLower(parts[0]) != "display" {
			continue
		} else {
			isDisplayNone = strings.TrimSpace(parts[1]) == "none"
		}
	}

	return isDisplayNone
}

type ForbiddenRuleMap map[atom.Atom][]atom.Atom
type ForbiddenScope map[atom.Atom]int

func GetLatexStandardFrobiddenRuleMap() ForbiddenRuleMap {
	return map[atom.Atom][]atom.Atom{
		atom.H1: {atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6, atom.Img},
		atom.H2: {atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6, atom.Img},
		atom.H3: {atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6, atom.Img},
		atom.H4: {atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6, atom.Img},
		atom.H5: {atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6, atom.Img},
		atom.H6: {atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6, atom.Img},
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

type NodeMatchArgs struct {
	Type  map[html.NodeType]bool
	Tag   map[atom.Atom]bool // a list of allowed tag type
	Id    map[string]bool    // node should have specified ID
	Class map[string]bool    // node should contain specified classes
	Attr  map[string]bool    // node should have specified attributes

	MatchFunc func(*html.Node, *NodeMatchArgs) bool // custom matching function to use in addition to tag meta data rules.

	Root      *html.Node // starting point of this match argument, this node won't be included in search result.
	LastMatch *html.Node // the result of last match, this node will be excluded from new matching process.
}

func CheckNodeIsMatch(node *html.Node, args *NodeMatchArgs) bool {
	if node == args.LastMatch || node == args.Root {
		return false
	}

	if args.Type != nil {
		if _, ok := args.Type[node.Type]; !ok {
			return false
		}
	}

	if args.Tag != nil {
		if _, ok := args.Tag[node.DataAtom]; !ok {
			return false
		}
	}

	if args.Id != nil {
		id, _ := GetNodeAttrVal(node, "id", "")
		if _, ok := args.Id[id]; !ok {
			return false
		}
	}

	if args.Class != nil {
		classStr, _ := GetNodeAttrVal(node, "class", "")

		class := map[string]bool{}
		scan := bufio.NewScanner(strings.NewReader(classStr))
		scan.Split(bufio.ScanWords)

		for scan.Scan() {
			name := scan.Text()
			class[name] = true
		}

		for k := range args.Class {
			if !class[k] {
				return false
			}
		}
	}

	if args.Attr != nil {
		attrSet := map[string]bool{}
		for _, attr := range node.Attr {
			attrSet[attr.Key] = true
		}

		for name := range args.Attr {
			if !attrSet[name] {
				return false
			}
		}
	}

	if args.MatchFunc != nil {
		if !args.MatchFunc(node, args) {
			return false
		}
	}

	return true
}

func FindMatchingNodeDFS(root *html.Node, args *NodeMatchArgs) *html.Node {
	if CheckNodeIsMatch(root, args) {
		return root
	}

	var match *html.Node
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		match = FindMatchingNodeDFS(child, args)
		if match != nil {
			break
		}
	}

	return match
}

func FindNextMatchingNode(node *html.Node, args *NodeMatchArgs) *html.Node {
	// searching under current node
	match := FindMatchingNodeDFS(node, args)
	if match != nil {
		return match
	}

	if node == args.Root {
		return nil
	}

	// move on to siblings
	for sibling := node; sibling != nil; sibling = sibling.NextSibling {
		match = FindMatchingNodeDFS(sibling, args)
		if match != nil {
			return match
		}
	}

	// step back to parent's siblings
	parent := node.Parent
	for parent != nil && parent != args.Root {
		for sibling := parent.NextSibling; sibling != nil; sibling = sibling.NextSibling {
			match = FindMatchingNodeDFS(sibling, args)
			if match != nil {
				return match
			}
		}
		parent = parent.Parent
	}

	return nil
}

func FindAllMatchingNodes(node *html.Node, args *NodeMatchArgs) []*html.Node {
	matches := []*html.Node{}
	match := FindNextMatchingNode(node, args)
	args.LastMatch = match

	for match != nil {
		matches = append(matches, match)
		match = FindNextMatchingNode(match, args)
		args.LastMatch = match
	}

	return matches
}

// ExtractText extracts all text node under given node as a slice.
func ExtractText(node *html.Node) []string {
	content := []string{}

	child := node.FirstChild
	for child != nil {
		if child.FirstChild != nil {
			child = child.FirstChild
			continue
		}

		if child.Type == html.TextNode {
			content = append(content, child.Data)
		}

		if child.NextSibling != nil {
			child = child.NextSibling
		} else {
			parent := child.Parent
			child = nil

			for parent != nil && parent != node {
				if parent.NextSibling != nil {
					child = parent.NextSibling
					break
				}

				parent = parent.Parent
			}
		}
	}

	return content
}
