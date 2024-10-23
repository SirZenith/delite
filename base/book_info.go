package base

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Represents infomation about a single book.
type BookInfo struct {
	Title  string `json:"title"`  // Book title
	Author string `json:"author"` // Book author

	TocURL string `json:"toc_url"` // URL to book's table of contents page

	RootDir string `json:"root_dir"`  // root directory of book
	RawDir  string `json:"raw_dir"`   // directory for cyphered HTML output
	TextDir string `json:"text_dir"`  // directory for decyphered HTML output
	ImgDir  string `json:"image_dir"` // directory for downloaded images
	EpubDir string `json:"epub_dir"`  // directory for writing epub file to

	HeaderFile  string `json:"header_file"` // JSON header list file, containing Array<{ name: string, value: string }>
	NameMapFile string `json:"name_map"`    // JSON file containing chapter title to file name mapping, in form of Array<{ title: string, file: string }>
}

// Generates book info struct from JSON file.
func ReadBookInfo(infoPath string) (*BookInfo, error) {
	data, err := os.ReadFile(infoPath)
	if err != nil {
		return nil, fmt.Errorf("can't read info file %s: %s", infoPath, err)
	}

	info := &BookInfo{}
	if err := json.Unmarshal(data, info); err != nil {
		return nil, fmt.Errorf("unable to parse info data in %s: %s", infoPath, err)
	}

	info.RootDir = GetStrOr(info.RootDir, filepath.Dir(infoPath))
	info.RawDir = ResolveRelativePath(info.RawDir, info.RootDir)
	info.TextDir = ResolveRelativePath(info.TextDir, info.RootDir)
	info.ImgDir = ResolveRelativePath(info.ImgDir, info.RootDir)
	info.EpubDir = ResolveRelativePath(info.EpubDir, info.RootDir)

	info.HeaderFile = ResolveRelativePath(info.HeaderFile, info.RootDir)
	info.NameMapFile = ResolveRelativePath(info.NameMapFile, info.RootDir)

	return info, nil
}
