package epub

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"

	"github.com/SirZenith/delite/common"
	"github.com/SirZenith/delite/common/html_util"
	format_common "github.com/SirZenith/delite/format/common"
	"github.com/beevik/etree"
	"github.com/charmbracelet/log"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

const ContainerDocumentPath = "META-INF/container.xml"

var errFinish = errors.New("task finished")

type ContainerFile struct {
	XMLName       xml.Name       `xml:"urn:oasis:names:tc:opendocument:xmlns:container container"`
	RootFileInfos []RootFileInfo `xml:"rootfiles>rootfile"`
}

type RootFileInfo struct {
	FullPath  string `xml:"full-path,attr"`
	MediaType string `xml:"media-type,attr"`
}

type PackageDocument struct {
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

// ----------------------------------------------------------------------------

// EpubReader is an object used for handling EPUB content, fields are initialized
// with `Init` method, and being read-only through out operation.
type EpubReader struct {
	filePath  string
	zipReader *zip.ReadCloser

	outputDir    string // path to output directory
	assetDirName string // base name of asset output directory under output directory
	jobCnt       int

	packs []*PackageDocument
}

type EpubReaderOptions struct {
	EpubFile     string
	OutputDir    string
	AssetDirName string

	JobCnt int
}

func (merger *EpubReader) Init(options EpubReaderOptions) error {
	merger.Close()

	reader, err := zip.OpenReader(options.EpubFile)
	if err != nil {
		return fmt.Errorf("can't open ZIP archive %s: %s", options.EpubFile, err)
	}

	merger.filePath = options.EpubFile
	merger.zipReader = reader

	merger.outputDir = options.OutputDir
	merger.assetDirName = options.AssetDirName
	merger.jobCnt = options.JobCnt

	packs, err := merger.loadPackages()
	if err != nil {
		return err
	}
	merger.packs = packs

	return nil
}

// Close closes underlying EPUB file of merger. Further access to EPUB file may
// result in panic.
func (reader *EpubReader) Close() error {
	if reader.zipReader != nil {
		if err := reader.zipReader.Close(); err != nil {
			return err
		}

		reader.zipReader = nil
	}

	return nil
}

// loadPackages reads EPUB's container document and find all XML package document,
// reads and parses their data.
func (reader *EpubReader) loadPackages() ([]*PackageDocument, error) {
	container, err := readXMLData[ContainerFile](reader.zipReader, ContainerDocumentPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read container document: %s", err)
	}

	packages := []*PackageDocument{}
	for _, info := range container.RootFileInfos {
		if info.MediaType == "application/oebps-package+xml" {
			pack, err := readXMLData[PackageDocument](reader.zipReader, info.FullPath)
			if err == nil {
				pack.FullPath = info.FullPath
				packages = append(packages, pack)
			}
		}
	}

	return packages, nil
}

// readHTMLResourceBody extracts body tag from HTML content, and parse
// it as HTML text. Returns parsed result as a slice of HTML node.
func (merger *EpubReader) readHTMLResourceBody(path string) ([]*html.Node, error) {
	file, err := merger.zipReader.Open(path)
	if err != nil {
		return nil, fmt.Errorf("can't open resource %s: %s", path, err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("can't read resource %s: %s", path, err)
	}

	reader := bytes.NewReader(data)
	doc, err := html.Parse(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML body: %s", err)
	}

	body := html_util.FindHTMLTag(doc, html.ElementNode, atom.Body)
	if body == nil {
		return nil, fmt.Errorf("faield to find body tag from HTML: %s", path)
	}

	result := []*html.Node{}
	node := body.FirstChild
	if node == nil {
		return nil, fmt.Errorf("empty body: %s", path)
	}

	for node != nil {
		nextNode := node.NextSibling

		body.RemoveChild(node)
		result = append(result, node)

		node = nextNode
	}

	return result, nil
}

func findXHTMLTag(root *etree.Element, tag string) *etree.Element {
	if root.Tag == tag {
		return root
	}

	var result *etree.Element
	children := root.ChildElements()
	for _, child := range children {
		result = findXHTMLTag(child, tag)
		if result != nil {
			break
		}
	}

	return result
}

// readXHTMLResourceBody extracts body tag from XML/XHTML content, and parse
// it as HTML text. Returns parsed result as a slice of HTML node.
func (merger *EpubReader) readXHTMLResourceBody(path string) ([]*html.Node, error) {
	file, err := merger.zipReader.Open(path)
	if err != nil {
		return nil, fmt.Errorf("can't open resource %s: %s", path, err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("can't read resource %s: %s", path, err)
	}

	reader := bytes.NewReader(data)
	xmlDoc := etree.NewDocument()
	_, err = xmlDoc.ReadFrom(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to parse XHTML content: %s", err)
	}

	xmlRoot := xmlDoc.Root()
	xmlBody := findXHTMLTag(xmlRoot, "body")
	if xmlBody == nil {
		return nil, fmt.Errorf("failed to find body tag in XHTML content")
	}

	htmlBuf := bytes.NewBufferString("")
	xmlBody.WriteTo(htmlBuf, &xmlDoc.WriteSettings)
	doc, err := html.Parse(htmlBuf)

	body := html_util.FindHTMLTag(doc, html.ElementNode, atom.Body)
	if body == nil {
		return nil, fmt.Errorf("faield to find body tag from XHTML: %s", path)
	}

	result := []*html.Node{}
	node := body.FirstChild
	if node == nil {
		return nil, fmt.Errorf("empty body: %s", path)
	}

	for node != nil {
		nextNode := node.NextSibling

		body.RemoveChild(node)
		result = append(result, node)

		node = nextNode
	}

	return result, nil
}

// GetTitle finds book title through iterating meta data of all packages. If no
// such meta data is fond, epub file name (without extension) will be used.
func (reader *EpubReader) GetTitle() string {
	ext := filepath.Ext(reader.filePath)
	basename := filepath.Base(reader.filePath)
	title := basename[:len(basename)-len(ext)]

	for _, pack := range reader.packs {
		if pack.Metadata.Title != "" {
			title = pack.Metadata.Title
			break
		}
	}

	return title
}

// GetAuthor returns author name recorded in book's meta data.
func (reader *EpubReader) GetAuthor() string {
	var author string

	for _, pack := range reader.packs {
		if pack.Metadata.Creator != "" {
			author = pack.Metadata.Creator
			break
		}
	}

	return author
}

// GetMergeOutputBasename returns book title with all invalid path character replaced.
func (reader *EpubReader) GetMergeOutputBasename() string {
	title := reader.GetTitle()
	return common.InvalidPathCharReplace(title)
}

// DumpAsset saves asset file in archive to disk. `srcPath` is resource path
// relative to archive root, `dstPath` is path of output file relative to
// merger's output directory.
func (reader *EpubReader) DumpAsset(srcPath, dstPath string, copyFunc func(dst io.Writer, src io.Reader)) error {
	outputPath := filepath.Join(reader.outputDir, dstPath)

	filePath, err := url.PathUnescape(srcPath)
	if err != nil {
		return fmt.Errorf("failed to unescape path %s: %e", srcPath, err)
	}

	srcFile, err := reader.zipReader.Open(filePath)
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
	if copyFunc != nil {
		copyFunc(dstBuf, srcBuf)
	} else {
		io.Copy(dstBuf, srcBuf)
	}

	err = dstBuf.Flush()
	if err != nil {
		return fmt.Errorf("failed to flush output buffer %s: %s", outputPath, err)
	}

	return nil
}

// BatchDumpAsset takes a map with key as resource path and value as dump path,
// dump all key value pairs to disk.
func (reader *EpubReader) BatchDumpAsset(pathMap map[string]string, copyFunc func(dst io.Writer, src io.Reader)) []error {
	var errorList []error
	jobCnt := reader.jobCnt
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
				err := reader.DumpAsset(task[0], task[1], copyFunc)
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
func (reader *EpubReader) MergerPackageContent(pack *PackageDocument) ([]*html.Node, error) {
	idMap := map[string]*PackageManifestItem{}
	for i := range pack.Manifest {
		item := &pack.Manifest[i]
		idMap[item.Id] = item
	}

	packageDir := path.Dir(pack.FullPath)
	result := []*html.Node{}

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
		case "text/html":
			nodes, err = reader.readHTMLResourceBody(resourcePath)
		case "application/xhtml+xml":
			nodes, err = reader.readXHTMLResourceBody(resourcePath)
		}

		if err != nil {
			log.Warnf("(packge %s) %s", pack.FullPath, err)
		} else if nodes != nil {
			result = append(result, &html.Node{
				Type: html.CommentNode,
				Data: format_common.MetaCommentFileStart + resourcePath,
			})
			result = append(result, nodes...)
			result = append(result, &html.Node{
				Type: html.CommentNode,
				Data: format_common.MetaCommentFileEnd + resourcePath,
			})
		}
	}

	return result, nil
}

type readerPackageTask struct {
	index int
	pack  *PackageDocument
}

type readerPackageResult struct {
	index int
	nodes []*html.Node
	err   error
}

func (reader *EpubReader) BatchMergePackageContent(packs []*PackageDocument) ([]*html.Node, []error) {
	jobCnt := reader.jobCnt
	taskChan := make(chan readerPackageTask, jobCnt)
	resultChan := make(chan readerPackageResult, jobCnt)

	go func() {
		for i, pack := range packs {
			taskChan <- readerPackageTask{
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
				if nodes, err := reader.MergerPackageContent(pack); err == nil {
					resultChan <- readerPackageResult{
						index: task.index,
						nodes: nodes,
					}
				} else {
					resultChan <- readerPackageResult{
						index: task.index,
						err:   fmt.Errorf("failed to merge content of package %s: %s", pack.FullPath, err),
					}
				}

			}

			resultChan <- readerPackageResult{
				err: errFinish,
			}
		}()
	}

	finishedCnt := 0
	resultList := make([]readerPackageResult, len(packs))
	for result := range resultChan {
		if errors.Is(result.err, errFinish) {
			finishedCnt++
			if finishedCnt >= jobCnt {
				break
			}
		} else {
			resultList[result.index] = result
		}
	}

	var nodes []*html.Node
	var errList []error
	for _, result := range resultList {
		nodes = append(nodes, result.nodes...)

		if result.err != nil {
			errList = append(errList, result.err)
		}
	}

	return nodes, errList
}

func (reader *EpubReader) Merge() ([]*html.Node, []error) {
	nodes, errList := reader.BatchMergePackageContent(reader.packs)
	return nodes, errList
}

// GetImageSize returns width and height of certain image in book.
func (reader *EpubReader) GetImageSize(srcPath string) (*image.Point, error) {
	filePath, err := url.PathUnescape(srcPath)
	if err != nil {
		return nil, fmt.Errorf("invalid src path %s: %s", srcPath, err)
	}

	file, err := reader.zipReader.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %s", srcPath, err)
	}
	defer file.Close()

	fileReader := bufio.NewReader(file)
	img, _, err := image.Decode(fileReader)
	if err != nil {
		return nil, fmt.Errorf("can't decode image %s: %s", srcPath, err)
	}

	size := img.Bounds().Size()

	return &size, nil
}
