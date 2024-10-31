package merge

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func saveHTMLOutput(options options, nodes []*html.Node, fileBasename string) (map[string]string, error) {
	doc, container, err := parseHTMLTemplate(options.template, options.htmlContainerID)
	if err != nil {
		return nil, err
	}

	for _, node := range nodes {
		container.AppendChild(node)
	}

	nameMap := make(map[string]string)
	resourceReferenceRedirect(doc, "", options.assetDirName, nameMap)

	outputName := filepath.Join(options.outputDir, fileBasename+".html")
	outFile, err := os.Create(outputName)
	if err != nil {
		return nil, fmt.Errorf("failed to write create file %s: %s", outputName, err)
	}
	defer outFile.Close()

	outWriter := bufio.NewWriter(outFile)
	html.Render(outWriter, doc)

	err = outWriter.Flush()
	if err != nil {
		return nil, fmt.Errorf("failed to flush output file buffer: %s", err)
	}

	return nameMap, nil
}

func resourceReferenceRedirect(node *html.Node, contextFile, assetOutDir string, outNameMap map[string]string) string {
	childContextFile := contextFile
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		childContextFile = resourceReferenceRedirect(child, childContextFile, assetOutDir, outNameMap)
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
			for i := range node.Attr {
				attr := &node.Attr[i]
				if attr.Key == "src" {
					packDir := path.Dir(contextFile)
					fullPath := path.Join(packDir, attr.Val)
					basename := path.Base(attr.Val)
					attr.Val = path.Join(assetOutDir, basename)
					outNameMap[fullPath] = attr.Val
				}
			}
		case atom.Image:
			for i := range node.Attr {
				attr := &node.Attr[i]
				if attr.Key == "href" {
					packDir := path.Dir(contextFile)
					fullPath := path.Join(packDir, attr.Val)
					basename := path.Base(attr.Val)
					attr.Val = path.Join(assetOutDir, basename)
					outNameMap[fullPath] = attr.Val
				}
			}
		}
	}

	return contextFile
}

// parseHTMLTemplate parses template string into HTML tree and tries to find container
// node in it.
// When `containerID` is empty string, body tag in template will be used. If a
// container node cannot be fond, a error will be returned.
// This function returns template HTML tree, pointer to container node, and error
// happened during operation.
func parseHTMLTemplate(template string, containerID string) (*html.Node, *html.Node, error) {
	templateReader := strings.NewReader(template)
	templateDoc, err := html.Parse(templateReader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse template string: %s", err)
	}

	var container *html.Node
	if containerID == "" {
		container = findHTMLBody(templateDoc)
	} else {
		container = findElementByID(templateDoc, containerID)
	}

	if container == nil {
		return nil, nil, fmt.Errorf("can't find HTML body tag in template")
	}

	return templateDoc, container, nil
}
