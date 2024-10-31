package merge

import (
	"archive/zip"
	"encoding/xml"
	"io"

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
