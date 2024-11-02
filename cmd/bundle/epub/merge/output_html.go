package merge

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/SirZenith/delite/common/html_util"
	format_html "github.com/SirZenith/delite/format/html"
	"golang.org/x/net/html"
)

func htmlOutputPreprocess(_ options, nodes []*html.Node) []*html.Node {
	for _, node := range nodes {
		format_html.SetImageTypeMeta(node)
	}

	return nodes
}

func saveHTMLOutput(options options, nodes []*html.Node, fileBasename string) error {
	doc, container, err := parseHTMLTemplate(options.template, options.htmlContainerID)
	if err != nil {
		return err
	}

	for _, node := range nodes {
		container.AppendChild(node)
	}

	outputName := filepath.Join(options.outputDir, fileBasename+".html")
	outFile, err := os.Create(outputName)
	if err != nil {
		return fmt.Errorf("failed to write create file %s: %s", outputName, err)
	}
	defer outFile.Close()

	outWriter := bufio.NewWriter(outFile)
	html.Render(outWriter, doc)

	err = outWriter.Flush()
	if err != nil {
		return fmt.Errorf("failed to flush output file buffer: %s", err)
	}

	return nil
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
		container = html_util.FindHTMLBody(templateDoc)
	} else {
		container = html_util.FindElementByID(templateDoc, containerID)
	}

	if container == nil {
		return nil, nil, fmt.Errorf("can't find HTML body tag in template")
	}

	return templateDoc, container, nil
}
