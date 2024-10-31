package merge

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/SirZenith/delite/common"
	"github.com/charmbracelet/log"
	"github.com/urfave/cli/v3"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

const containerDocumentPath = "META-INF/container.xml"
const defaultAssetDirName = "assets"

var errFinish = errors.New("task finished")

const defaultTemplate = `
<html>
<head>
    <meta charset="UTF-8">
    <meta http-equiv="X-UA-Compatible" content="IE=edge">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Document</title>
</head>
<body>
</body>
</html>
`

func Cmd() *cli.Command {
	var epubFile string

	cmd := &cli.Command{
		Name:  "merge",
		Usage: "merge HTML content of EPUB book into a single HTML file.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "template-file",
				Usage: "path to HTML template file, ignored when `template` flag has non-empty value.",
			},
			&cli.StringFlag{
				Name:  "template",
				Usage: "output template string.",
			},
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "path to output directory, if no value is given, a directory with the same name as book file (without extensioin) will be created, and result will be written to that file",
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "input",
				UsageText:   "<epub-file>",
				Destination: &epubFile,
				Min:         1,
				Max:         1,
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			options, err := getOptionsFromCmd(cmd, epubFile)
			if err != nil {
				return err
			}

			return cmdMain(options)
		},
	}

	return cmd
}

type options struct {
	template  string
	outputDir string
	epubFile  string
}

func getOptionsFromCmd(cmd *cli.Command, epubFile string) (options, error) {
	options := options{
		template:  cmd.String("template"),
		outputDir: cmd.String("output"),
		epubFile:  epubFile,
	}

	if options.outputDir == "" {
		ext := filepath.Ext(options.epubFile)
		basename := filepath.Base(options.epubFile)
		options.outputDir = basename[:len(basename)-len(ext)]
	}

	templateFile := cmd.String("template-file")
	if options.template != "" {
		// pass
	} else if templateFile == "" {
		options.template = defaultTemplate
	} else {
		data, err := os.ReadFile(templateFile)
		if err != nil {
			return options, fmt.Errorf("failed to read template file %s: %s", templateFile, err)
		}

		options.template = string(data)
	}

	return options, nil
}

func cmdMain(options options) error {
	if _, err := os.Stat(options.epubFile); err != nil {
		return fmt.Errorf("can't access target file %s: %s", options.epubFile, err)
	}

	if err := os.MkdirAll(options.outputDir, 0o755); err != nil {
		return fmt.Errorf("failed to create output directory %s: %s", options.outputDir, err)
	}

	merger := new(EpubMerger)
	err := merger.Init(options.epubFile, options.outputDir, runtime.NumCPU())
	if err != nil {
		return err
	}
	defer merger.Close()

	err = merger.Merge(options.template)
	if err != nil {
		return fmt.Errorf("merge failed: %s", err)
	}

	return nil
}

// ----------------------------------------------------------------------------

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

type ResourceFetcher = func(path string) ([]byte, error)

type EpubContainer struct {
	XMLName       xml.Name       `xml:"urn:oasis:names:tc:opendocument:xmlns:container container"`
	RootFileInfos []RootFileInfo `xml:"rootfiles>rootfile"`
}

type RootFileInfo struct {
	FullPath  string `xml:"full-path,attr"`
	MediaType string `xml:"media-type,attr"`
}

type PackageXML struct {
	FullPath string
	XMLName  xml.Name              `xml:"package"`
	Metadata PackageMeta           `xml:"metadata"`
	Manifest []PackageManifestItem `xml:"manifest>item"`
	Spine    []PackageSpineItem    `xml:"spine>itemref"`
	Guide    []PackageGuideItem    `xml:"guide>reference"`
}

type PackageMeta struct {
	Identifier string `xml:"identifier"`
	Title      string `xml:"title"`
	Language   string `xml:"language"`
	Creator    string `xml:"creator"`
}

type PackageManifestItem struct {
	Id        string `xml:"id,attr"`
	Href      string `xml:"href,attr"`
	MediaType string `xml:"media-type,attr"`
}

type PackageSpineItem struct {
	IdRef string `xml:"idref,attr"`
}

type PackageGuideItem struct {
	Type  string `xml:"type,attr"`
	Title string `xml:"title,attr"`
	Href  string `xml:"href,attr"`
}

type XHTMLContent struct {
	Body XHTMLBody `xml:"body"`
}

type XHTMLBody struct {
	Content string `xml:",innerxml"`
}

// ----------------------------------------------------------------------------

type EpubMerger struct {
	filePath string
	reader   *zip.ReadCloser

	outputDir    string // path to output directory
	assetDirName string // base name of asset output directory under output directory

	jobCnt int
}

func (merger *EpubMerger) Init(filePath, outputDir string, jobCnt int) error {
	merger.Close()

	reader, err := zip.OpenReader(filePath)
	if err != nil {
		return fmt.Errorf("can't open ZIP archive %s: %s", filePath, err)
	}

	merger.filePath = filePath
	merger.reader = reader

	merger.outputDir = outputDir
	merger.assetDirName = defaultAssetDirName

	merger.jobCnt = jobCnt

	return nil
}

// Close closes underlying EPUB file of merger. Further access to EPUB file may
// result in panic.
func (merger *EpubMerger) Close() error {
	if merger.reader != nil {
		if err := merger.reader.Close(); err != nil {
			return err
		}

		merger.reader = nil
	}

	return nil
}

// loadPackages reads EPUB's container document and find all XML package document,
// reads and parses their data.
func (merger *EpubMerger) loadPackages() ([]*PackageXML, error) {
	container, err := readXMLData[EpubContainer](merger.reader, containerDocumentPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read container document: %s", err)
	}

	packages := []*PackageXML{}
	for _, info := range container.RootFileInfos {
		if info.MediaType == "application/oebps-package+xml" {
			pack, err := readXMLData[PackageXML](merger.reader, info.FullPath)
			if err == nil {
				pack.FullPath = info.FullPath
				packages = append(packages, pack)
			}
		}
	}

	return packages, nil
}

// readXMLResourceBody extracts body tag from XML/XHTML/HTML content, and parse
// it as HTML text. Returns parsed result as a slice of HTML node.
func (merger *EpubMerger) readXMLResourceBody(path string) ([]*html.Node, error) {
	file, err := merger.reader.Open(path)
	if err != nil {
		return nil, fmt.Errorf("can't open resource %s: %s", path, err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("can't read resource %s: %s", path, err)
	}

	content := new(XHTMLContent)
	err = xml.Unmarshal(data, content)
	if err != nil {
		return nil, fmt.Errorf("can't parse resource %s: %s", path, err)
	}

	reader := strings.NewReader(content.Body.Content)
	doc, err := html.Parse(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to parse XHTML body: %s", err)
	}

	body := findHTMLBody(doc)
	result := []*html.Node{}
	node := body.FirstChild
	for node != nil {
		nextNode := node.NextSibling

		body.RemoveChild(node)
		result = append(result, node)

		node = nextNode
	}

	return result, nil
}

func (merger *EpubMerger) redirectImageReference(root *html.Node, packageDir, assetOutDir string, outNameMap map[string]string) {
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		merger.redirectImageReference(child, packageDir, assetOutDir, outNameMap)
	}

	if root.Type != html.ElementNode {
		return
	}

	switch root.DataAtom {
	case atom.Img:
		for i := range root.Attr {
			attr := &root.Attr[i]
			if attr.Key == "src" {
				fullPath := path.Join(packageDir, attr.Val)
				basename := path.Base(attr.Val)
				attr.Val = path.Join(assetOutDir, basename)
				outNameMap[fullPath] = attr.Val
			}
		}
	case atom.Image:
		for i := range root.Attr {
			attr := &root.Attr[i]
			if attr.Key == "href" {
				fullPath := path.Join(packageDir, attr.Val)
				basename := path.Base(attr.Val)
				attr.Val = path.Join(assetOutDir, basename)
				outNameMap[fullPath] = attr.Val
			}
		}
	}
}

// DumpAsset saves asset file in archive to disk. `srcPath` is resource path
// relative to archive root, `dstPath` is path of output file relative to
// merger's output directory.
func (merger *EpubMerger) DumpAsset(srcPath, dstPath string) error {
	outputPath := filepath.Join(merger.outputDir, dstPath)

	srcFile, err := merger.reader.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to read resource %s: %s", srcPath, err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create resource output %s: %s", outputPath, err)
	}
	defer dstFile.Close()

	srcBuf := bufio.NewReader(srcFile)
	dstBuf := bufio.NewWriter(dstFile)
	io.Copy(dstBuf, srcBuf)

	err = dstBuf.Flush()
	if err != nil {
		return fmt.Errorf("failed to flush output buffer %s: %s", outputPath, err)
	}

	return nil
}

// BatchDumpAsset takes a map with key as resource path and value as dump path,
// dump all key value pairs to disk.
func (merger *EpubMerger) BatchDumpAsset(pathMap map[string]string) []error {
	var errorList []error
	jobCnt := merger.jobCnt
	taskChan := make(chan [2]string, jobCnt)
	errChan := make(chan error, jobCnt)

	go func() {
		for srcPath, dstPath := range pathMap {
			taskChan <- [2]string{srcPath, dstPath}
		}
		close(taskChan)
	}()

	for i := 0; i < jobCnt; i++ {
		go func() {
			for task := range taskChan {
				err := merger.DumpAsset(task[0], task[1])
				if err != nil {
					errChan <- fmt.Errorf("dump failed %s -> %s: %s", task[0], task[1], err)
				}
			}

			errChan <- errFinish
		}()
	}

	finishedCnt := 0
	for err := range errChan {
		if err == nil {
			// pass
		} else if errors.Is(err, errFinish) {
			finishedCnt++
			if finishedCnt >= jobCnt {
				break
			}
		} else {
			errorList = append(errorList, err)
		}
	}

	return errorList
}

// MergerPackageContent reads all item listed in `spine` section of package document.
// Merge the content of all items into a slice of HTML node.
// All resources referenced by `img` and `image` tag will be redirect to output asset
// directory.
func (merger *EpubMerger) MergerPackageContent(pack *PackageXML) ([]*html.Node, map[string]string, error) {
	idMap := map[string]*PackageManifestItem{}
	for i := range pack.Manifest {
		item := &pack.Manifest[i]
		idMap[item.Id] = item
	}

	packageDir := path.Dir(pack.FullPath)
	result := []*html.Node{}
	resourceNameMap := map[string]string{}

	for _, item := range pack.Spine {
		resource, ok := idMap[item.IdRef]
		if !ok {
			log.Warnf("invalid manifest ID in %s: %s", pack.FullPath, item.IdRef)
			continue
		}

		resourcePath := path.Join(packageDir, resource.Href)

		var (
			nodes []*html.Node
			err   error
		)
		switch resource.MediaType {
		case "text/html", "application/xhtml+xml":
			nodes, err = merger.readXMLResourceBody(resourcePath)
		}

		if err != nil {
			log.Warnf("(packge %s) %s", pack.FullPath, err)
		} else if nodes != nil {
			result = append(result, &html.Node{
				Type: html.CommentNode,
				Data: resourcePath,
			})
			result = append(result, &html.Node{
				Type: html.TextNode,
				Data: "\n",
			})

			result = append(result, nodes...)

			result = append(result, &html.Node{
				Type: html.TextNode,
				Data: "\n",
			})

			for _, node := range nodes {
				merger.redirectImageReference(node, packageDir, merger.assetDirName, resourceNameMap)
			}
		}
	}

	return result, resourceNameMap, nil
}

type mergePackageTask struct {
	index int
	pack  *PackageXML
}

type mergePackageResult struct {
	index        int
	nodes        []*html.Node
	assetNameMap map[string]string
	err          error
}

func (merger *EpubMerger) BatchMergePackageContent(packs []*PackageXML) ([]*html.Node, map[string]string, []error) {
	jobCnt := merger.jobCnt
	taskChan := make(chan mergePackageTask, jobCnt)
	resultChan := make(chan mergePackageResult, jobCnt)

	go func() {
		for i, pack := range packs {
			taskChan <- mergePackageTask{
				index: i,
				pack:  pack,
			}
		}
		close(taskChan)
	}()

	for i := 0; i < jobCnt; i++ {
		go func() {
			for task := range taskChan {
				pack := task.pack
				nodes, nameMap, err := merger.MergerPackageContent(pack)
				if err != nil {
					resultChan <- mergePackageResult{
						index: task.index,
						err:   fmt.Errorf("failed to merge content of package %s: %s", pack.FullPath, err),
					}
					continue
				}

				resultChan <- mergePackageResult{
					index:        task.index,
					nodes:        nodes,
					assetNameMap: nameMap,
				}
			}

			resultChan <- mergePackageResult{
				err: errFinish,
			}
		}()
	}

	finishedCnt := 0
	resultList := make([]mergePackageResult, len(packs))
	for result := range resultChan {
		if errors.Is(result.err, errFinish) {
			finishedCnt++
			if finishedCnt >= jobCnt {
				break
			}
			continue
		}

		resultList[result.index] = result
	}

	var nodes []*html.Node
	nameMap := make(map[string]string)
	var errList []error
	for _, result := range resultList {
		nodes = append(nodes, result.nodes...)

		for srcPath, dstPath := range result.assetNameMap {
			nameMap[srcPath] = dstPath
		}

		if result.err != nil {
			errList = append(errList, result.err)
		}
	}

	return nodes, nameMap, errList
}

func (merger *EpubMerger) Merge(template string) error {
	templateReader := strings.NewReader(template)
	templateDoc, err := html.Parse(templateReader)
	if err != nil {
		return fmt.Errorf("failed to parse template string: %s", err)
	}

	templateBody := findHTMLBody(templateDoc)
	if templateBody == nil {
		return fmt.Errorf("can't find HTML body tag in template")
	}

	packages, err := merger.loadPackages()
	if err != nil {
		return err
	}

	assetOutDir := filepath.Join(merger.outputDir, merger.assetDirName)
	if err = os.MkdirAll(assetOutDir, 0o755); err != nil {
		return fmt.Errorf("failed to create asset directory %s: %s", assetOutDir, err)
	}

	nodes, assetNameMap, errList := merger.BatchMergePackageContent(packages)
	for _, node := range nodes {
		templateBody.AppendChild(node)
	}
	for _, err := range errList {
		log.Warnf("%s", err)
	}

	title := filepath.Base(merger.outputDir)
	for _, pack := range packages {
		if pack.Metadata.Title != "" {
			title = pack.Metadata.Title
			break
		}
	}

	title = common.InvalidPathCharReplace(title)
	outputName := filepath.Join(merger.outputDir, title+".html")
	outFile, err := os.Create(outputName)
	if err != nil {
		return fmt.Errorf("failed to write create file %s: %s", outputName, err)
	}
	defer outFile.Close()

	outWriter := bufio.NewWriter(outFile)
	html.Render(outWriter, templateDoc)

	outWriter.Flush()

	errList = merger.BatchDumpAsset(assetNameMap)
	for _, err := range errList {
		log.Warnf("%s", err)
	}

	return nil
}
