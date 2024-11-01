package merge

import (
	"archive/zip"
	"bufio"
	"bytes"
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
)

var errFinish = errors.New("task finished")

func Cmd() *cli.Command {
	var epubFile string

	cmd := &cli.Command{
		Name:  "merge",
		Usage: "merge HTML content of EPUB book into a single HTML file.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "template-file",
				Aliases: []string{"T"},
				Usage:   "path to file containing template string, ignored when `template` flag has non-empty value.",
			},
			&cli.StringFlag{
				Name:    "template",
				Aliases: []string{"t"},
				Usage: strings.Join([]string{
					"output template string.",
					"    1. For HTML format, this should be HTML text.",
					"       By default book content will be filled into `body` tag of template,",
					"       User can specify container element by ID attribute and `html-id` flag",
				}, "\n"),
			},
			&cli.StringFlag{
				Name:  "html-id",
				Usage: "id of target HTML tag to fill book content to",
			},
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "path to output directory, if no value is given, a directory with the same name as book file (without extension) will be created, and result will be written to that file",
			},
			&cli.StringFlag{
				Name:    "format",
				Aliases: []string{"f"},
				Usage: "output format, valid values are: " + strings.Join([]string{
					outputFormatHTML,
					outputFormatLatex,
				}, ", "),
				Value: outputFormatHTML,
			},
			&cli.StringFlag{
				Name:  "script",
				Usage: "path to preprocess script",
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
	template        string
	htmlContainerID string

	epubFile         string
	outputDir        string
	assetDirName     string
	outputFormat     string
	preprocessScript string

	jobCnt int
}

func getOptionsFromCmd(cmd *cli.Command, epubFile string) (options, error) {
	options := options{
		template:        cmd.String("template"),
		htmlContainerID: cmd.String("html-id"),

		epubFile:         epubFile,
		outputDir:        cmd.String("output"),
		assetDirName:     defaultAssetDirName,
		outputFormat:     cmd.String("format"),
		preprocessScript: cmd.String("script"),

		jobCnt: runtime.NumCPU(),
	}

	if options.outputDir == "" {
		ext := filepath.Ext(options.epubFile)
		basename := filepath.Base(options.epubFile)
		options.outputDir = basename[:len(basename)-len(ext)]
	}

	switch options.outputFormat {
	case outputFormatHTML, outputFormatLatex:
		// pass
	default:
		return options, fmt.Errorf("invalid output format: %q", options.outputFormat)
	}

	templateFile := cmd.String("template-file")
	if options.template != "" {
		// pass
	} else if templateFile == "" {
		options.template = getDefaultTemplate(options.outputFormat)
	} else {
		data, err := os.ReadFile(templateFile)
		if err != nil {
			return options, fmt.Errorf("failed to read template file %s: %s", templateFile, err)
		}

		options.template = string(data)
	}

	return options, nil
}

func getDefaultTemplate(format string) string {
	switch format {
	case outputFormatHTML:
		return defaultHTMLTemplate
	case outputFormatLatex:
		return defaultLatexTemplte
	default:
		return ""
	}
}

func cmdMain(options options) error {
	if _, err := os.Stat(options.epubFile); err != nil {
		return fmt.Errorf("can't access target file %s: %s", options.epubFile, err)
	}

	if err := os.MkdirAll(options.outputDir, 0o755); err != nil {
		return fmt.Errorf("failed to create output directory %s: %s", options.outputDir, err)
	}

	assetOutDir := filepath.Join(options.outputDir, options.assetDirName)
	if err := os.MkdirAll(assetOutDir, 0o755); err != nil {
		return fmt.Errorf("failed to create asset directory %s: %s", assetOutDir, err)
	}

	merger := new(EpubMerger)
	if err := merger.Init(options); err != nil {
		return err
	}
	defer merger.Close()

	nodes, errList := merger.Merge()
	for _, err := range errList {
		log.Warnf("%s", err)
	}

	if options.preprocessScript != "" {
		if processed, err := runPreprocessScript(nodes, options.preprocessScript); err == nil {
			nodes = processed
		} else {
			log.Warnf("failed to run preprocess script %s:\n%s", options.preprocessScript, err)
		}
	}

	var (
		nameMap map[string]string
		err     error
	)
	outputBasename := merger.GetMergeOutputBasename()

	switch options.outputFormat {
	case "html":
		nameMap, err = saveHTMLOutput(options, nodes, outputBasename)
	case "latex":
		nameMap, err = saveLatexOutput(options, nodes, outputBasename)
	}

	if err != nil {
		return err
	}

	if nameMap != nil {
		if errList := merger.BatchDumpAsset(nameMap); errList != nil {
			for _, err := range errList {
				log.Warnf("%s", err)
			}
		}
	}

	return nil
}

// ----------------------------------------------------------------------------

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

// ----------------------------------------------------------------------------

// EpubMerger is an object used for handling EPUB content, fields are initialized
// with `Init` method, and being read-only through out operation.
type EpubMerger struct {
	filePath string
	reader   *zip.ReadCloser

	outputDir    string // path to output directory
	assetDirName string // base name of asset output directory under output directory
	jobCnt       int

	packs []*PackageDocument
}

func (merger *EpubMerger) Init(options options) error {
	merger.Close()

	reader, err := zip.OpenReader(options.epubFile)
	if err != nil {
		return fmt.Errorf("can't open ZIP archive %s: %s", options.epubFile, err)
	}

	merger.filePath = options.epubFile
	merger.reader = reader

	merger.outputDir = options.outputDir
	merger.assetDirName = options.assetDirName
	merger.jobCnt = options.jobCnt

	packs, err := merger.loadPackages()
	if err != nil {
		return err
	}
	merger.packs = packs

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
func (merger *EpubMerger) loadPackages() ([]*PackageDocument, error) {
	container, err := readXMLData[ContainerFile](merger.reader, containerDocumentPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read container document: %s", err)
	}

	packages := []*PackageDocument{}
	for _, info := range container.RootFileInfos {
		if info.MediaType == "application/oebps-package+xml" {
			pack, err := readXMLData[PackageDocument](merger.reader, info.FullPath)
			if err == nil {
				pack.FullPath = info.FullPath
				packages = append(packages, pack)
			}
		}
	}

	return packages, nil
}

// readHTMLResourceBody extracts body tag from XML/XHTML/HTML content, and parse
// it as HTML text. Returns parsed result as a slice of HTML node.
func (merger *EpubMerger) readHTMLResourceBody(path string) ([]*html.Node, error) {
	file, err := merger.reader.Open(path)
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

// GetTitle finds book title through iterating meta data of all packages. If no
// such meta data is fond, epub file name (without extension) will be used.
func (merger *EpubMerger) GetTitle() string {
	ext := filepath.Ext(merger.filePath)
	basename := filepath.Base(merger.filePath)
	title := basename[:len(basename)-len(ext)]

	for _, pack := range merger.packs {
		if pack.Metadata.Title != "" {
			title = pack.Metadata.Title
			break
		}
	}

	return title
}

// GetMergeOutputBasename returns book title with all invalid path character replaced.
func (merger *EpubMerger) GetMergeOutputBasename() string {
	title := merger.GetTitle()
	return common.InvalidPathCharReplace(title)
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
func (merger *EpubMerger) MergerPackageContent(pack *PackageDocument) ([]*html.Node, error) {
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
		case "text/html", "application/xhtml+xml":
			nodes, err = merger.readHTMLResourceBody(resourcePath)
		}

		if err != nil {
			log.Warnf("(packge %s) %s", pack.FullPath, err)
		} else if nodes != nil {
			result = append(result, &html.Node{
				Type: html.CommentNode,
				Data: metaCommentFileStart + resourcePath,
			})
			result = append(result, nodes...)
			result = append(result, &html.Node{
				Type: html.CommentNode,
				Data: metaCommentFileEnd + resourcePath,
			})
		}
	}

	return result, nil
}

type mergePackageTask struct {
	index int
	pack  *PackageDocument
}

type mergePackageResult struct {
	index int
	nodes []*html.Node
	err   error
}

func (merger *EpubMerger) BatchMergePackageContent(packs []*PackageDocument) ([]*html.Node, []error) {
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
				nodes, err := merger.MergerPackageContent(pack)
				if err != nil {
					resultChan <- mergePackageResult{
						index: task.index,
						err:   fmt.Errorf("failed to merge content of package %s: %s", pack.FullPath, err),
					}
					continue
				}

				resultChan <- mergePackageResult{
					index: task.index,
					nodes: nodes,
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
	var errList []error
	for _, result := range resultList {
		nodes = append(nodes, result.nodes...)

		if result.err != nil {
			errList = append(errList, result.err)
		}
	}

	return nodes, errList
}

func (merger *EpubMerger) Merge() ([]*html.Node, []error) {
	nodes, errList := merger.BatchMergePackageContent(merger.packs)
	return nodes, errList
}
