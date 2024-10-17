package base

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type BookInfo struct {
	Title  string `json:"title"`  // Book title
	Author string `json:"author"` // Book author

	TocURL        string `json:"toc_url"`         // URL to book's table of contents page
	RawHTMLOutput string `json:"raw_html_output"` // directory for cyphered HTML output
	HTMLOutput    string `json:"html_output"`     // directory for decyphered HTML output
	ImgOutput     string `json:"img_output"`      // directory for downloaded images
	EpubOutput    string `json:"epub_output"`     // directory for writing epub file to
}

// Read book info from JSON file.
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

	info.RawHTMLOutput = resolveRelativePath(info.RawHTMLOutput, infoDir)
	info.HTMLOutput = resolveRelativePath(info.HTMLOutput, infoDir)
	info.ImgOutput = resolveRelativePath(info.ImgOutput, infoDir)
	info.EpubOutput = resolveRelativePath(info.EpubOutput, infoDir)

	return info, nil
}

// Expand `target` relative to given path if its a relative path, else it will
// be returned unchanged.
func resolveRelativePath(target, relativeTo string) string {
	if filepath.IsAbs(target) {
		return target
	}

	target = filepath.Join(relativeTo, target)
	target = filepath.Clean(target)

	return target
}
