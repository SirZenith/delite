package base

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type BookInfo struct {
	Title  string `json:"title"`  // Book title
	Author string `json:"author"` // Book author

	TocURL string `json:"toc_url"` // URL to book's table of contents page

	RawHTMLOutput string `json:"raw_html_output"` // directory for cyphered HTML output
	HTMLOutput    string `json:"html_output"`     // directory for decyphered HTML output
	ImgOutput     string `json:"img_output"`      // directory for downloaded images
	EpubOutput    string `json:"epub_output"`     // directory for writing epub file to

	HeaderFile  string `json:"header_file"` // JSON header list file, containing Array<{ name: string, value: string }>
	NameMapFile string `json:"name_map"`    // JSON file containing chapter title to file name mapping, in form of Array<{ title: string, file: string }>
}

// Generates book info struct from JSON file.
func ReadBookInfo(infoFile string) (*BookInfo, error) {
	data, err := os.ReadFile(infoFile)
	if err != nil {
		return nil, err
	}

	info := &BookInfo{}
	if err := json.Unmarshal(data, info); err != nil {
		return nil, err
	}

	infoDir := filepath.Dir(infoFile)

	info.RawHTMLOutput = ResolveRelativePath(info.RawHTMLOutput, infoDir)
	info.HTMLOutput = ResolveRelativePath(info.HTMLOutput, infoDir)
	info.ImgOutput = ResolveRelativePath(info.ImgOutput, infoDir)
	info.EpubOutput = ResolveRelativePath(info.EpubOutput, infoDir)

	info.HeaderFile = ResolveRelativePath(info.HeaderFile, infoDir)
	info.NameMapFile = ResolveRelativePath(info.NameMapFile, infoDir)

	return info, nil
}
