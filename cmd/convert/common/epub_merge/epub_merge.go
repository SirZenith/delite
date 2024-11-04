package epub_merge

import (
	"fmt"
	"image"
	"os"
	"path/filepath"

	"github.com/SirZenith/delite/format/epub"
	format_html "github.com/SirZenith/delite/format/html"
	"github.com/charmbracelet/log"
	"golang.org/x/net/html"
)

type EpubMergeOptions struct {
	EpubFile     string
	OutputDir    string
	AssetDirName string

	JobCnt int

	PreprocessFunc func(nodes []*html.Node) []*html.Node
	SaveOutputFunc func(nodes []*html.Node, fileBasename string, author string) error
}

func Merge(options EpubMergeOptions) error {
	if _, err := os.Stat(options.EpubFile); err != nil {
		return fmt.Errorf("can't access target file %s: %s", options.EpubFile, err)
	}

	if err := os.MkdirAll(options.OutputDir, 0o755); err != nil {
		return fmt.Errorf("failed to create output directory %s: %s", options.OutputDir, err)
	}

	assetOutDir := filepath.Join(options.OutputDir, options.AssetDirName)
	if err := os.MkdirAll(assetOutDir, 0o755); err != nil {
		return fmt.Errorf("failed to create asset directory %s: %s", assetOutDir, err)
	}

	merger := new(epub.EpubReader)
	if err := merger.Init(epub.EpubReaderOptions{
		EpubFile:     options.EpubFile,
		OutputDir:    options.OutputDir,
		AssetDirName: options.AssetDirName,
		JobCnt:       options.JobCnt,
	}); err != nil {
		return err
	}

	defer merger.Close()

	nodes, errList := merger.Merge()
	for _, err := range errList {
		log.Warnf("%s", err)
	}

	NodePreprocess(options, merger, nodes)

	outputBasename := merger.GetMergeOutputBasename()
	author := merger.GetAuthor()

	return options.SaveOutputFunc(nodes, outputBasename, author)
}

func NodePreprocess(options EpubMergeOptions, merger *epub.EpubReader, nodes []*html.Node) {
	// image reference handling
	nameMap := map[string]string{}
	contextFile := ""
	for _, node := range nodes {
		contextFile = format_html.ImageReferenceRedirect(node, contextFile, options.AssetDirName, nameMap)
	}

	if errList := merger.BatchDumpAsset(nameMap); errList != nil {
		for _, err := range errList {
			log.Warnf("%s", err)
		}
	}

	// image meta data injection
	sizeMap := map[string]*image.Point{}
	for srcPath, dstPath := range nameMap {
		if size, err := merger.GetImageSize(srcPath); err == nil {
			sizeMap[dstPath] = size
		} else {
			log.Warnf("failed to get image size: %s", err)
		}
	}
	for _, node := range nodes {
		format_html.SetImageSizeMeta(node, sizeMap)
	}

	nodes = options.PreprocessFunc(nodes)
}
