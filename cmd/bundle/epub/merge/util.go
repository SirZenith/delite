package merge

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	lua_html "github.com/SirZenith/delite/lua_module/html"
	lua_html_atom "github.com/SirZenith/delite/lua_module/html/atom"
	lua "github.com/yuin/gopher-lua"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func readZipContent(reader *zip.ReadCloser, path string) ([]byte, error) {
	fileReader, err := reader.Open(path)
	if err != nil {
		return nil, err
	}

	return io.ReadAll(fileReader)
}

func readXMLData[T any](reader *zip.ReadCloser, path string) (*T, error) {
	data, err := readZipContent(reader, path)

	container := new(T)
	err = xml.Unmarshal(data, container)
	if err != nil {
		return nil, err
	}

	return container, nil
}

func findHTMLBody(root *html.Node) *html.Node {
	if root.Type == html.ElementNode && root.DataAtom == atom.Body {
		return root
	}

	var result *html.Node
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		result = findHTMLBody(child)
		if result != nil {
			break
		}
	}

	return result
}

func findElementByID(root *html.Node, id string) *html.Node {
	if root.Type == html.ElementNode {
		for _, attr := range root.Attr {
			if attr.Key == "id" && attr.Val == id {
				return root
			}
		}
	}

	var result *html.Node
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		result = findElementByID(child, id)
		if result != nil {
			break
		}
	}

	return result
}

func getNodeAttr(node *html.Node, attrName string) *html.Attribute {
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

func replaceImageReference(attr *html.Attribute, contextFile, assetOutDir string) (string, string) {
	packDir := path.Dir(contextFile)
	fullPath := path.Join(packDir, attr.Val)
	basename := path.Base(attr.Val)
	attr.Val = path.Join(assetOutDir, basename)
	return fullPath, attr.Val
}

func imageReferenceRedirect(node *html.Node, contextFile, assetOutDir string, outNameMap map[string]string) string {
	childContextFile := contextFile
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		childContextFile = imageReferenceRedirect(child, childContextFile, assetOutDir, outNameMap)
		if childContextFile == "" {
			childContextFile = contextFile
		}
	}

	switch node.Type {
	case html.CommentNode:
		switch {
		case strings.HasPrefix(node.Data, fileStartCommentPrefix):
			contextFile = node.Data[len(fileStartCommentPrefix):]
		case strings.HasPrefix(node.Data, fileEndCommentPrefix):
			contextFile = ""
		}
	case html.ElementNode:
		switch node.DataAtom {
		case atom.Img:
			attr := getNodeAttr(node, "src")
			if attr != nil {
				srcPath, dstPath := replaceImageReference(attr, contextFile, assetOutDir)
				outNameMap[srcPath] = dstPath
			}
		case atom.Image:
			attr := getNodeAttr(node, "href")
			if attr != nil {
				srcPath, dstPath := replaceImageReference(attr, contextFile, assetOutDir)
				outNameMap[srcPath] = dstPath
			}
		}
	}

	return contextFile
}

func checkIsDisplayNone(node *html.Node) bool {
	style := getNodeAttr(node, "style")
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

func runPreprocessScript(nodes []*html.Node, scriptPath string) ([]*html.Node, error) {
	if _, err := os.Stat(scriptPath); err != nil {
		return nil, fmt.Errorf("failed to access script %s: %s", scriptPath, err)
	}

	L := lua.NewState()
	defer L.Close()

	lua_html.RegisterNodeType(L)

	L.PreloadModule("html", lua_html.Loader)
	L.PreloadModule("html-atom", lua_html_atom.Loader)

	luaNodes := L.NewTable()
	for i, node := range nodes {
		luaNode := lua_html.NewNode(L, node)
		L.RawSetInt(luaNodes, i+1, luaNode)
	}
	L.SetGlobal("nodes", luaNodes)

	if err := L.DoFile(scriptPath); err != nil {
		return nil, fmt.Errorf("preprocess script executation error:\n%s", err)
	}

	tbl, ok := L.Get(1).(*lua.LTable)
	if !ok {
		return nil, fmt.Errorf("preprocess script does not return a table")
	}

	totalCnt := tbl.Len()
	newNodes := []*html.Node{}
	for i := 1; i <= totalCnt; i++ {
		value := tbl.RawGetInt(i)

		ud, ok := value.(*lua.LUserData)
		if !ok {
			return nil, fmt.Errorf("invalid return value found at index %d, expecting userdata, found %s", i, value.Type().String())
		}

		wrapped, ok := ud.Value.(*lua_html.Node)
		if !ok {
			return nil, fmt.Errorf("invalid usertdata found at index %d", i)
		}

		newNodes = append(newNodes, wrapped.Node)
	}

	return newNodes, nil
}
