package html_util

import (
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func FindHTMLBody(root *html.Node) *html.Node {
	if root.Type == html.ElementNode && root.DataAtom == atom.Body {
		return root
	}

	var result *html.Node
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		result = FindHTMLBody(child)
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
