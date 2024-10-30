package book_management

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/SirZenith/delite/common"
)

// Represents infomation about a single book.
type BookInfo struct {
	Title       string `json:"title"`                 // Book title
	Author      string `json:"author"`                // Book author
	TocURL      string `json:"toc_url"`               // URL to book's table of contents page
	PageCnt     int    `json:"page_cnt"`              // optional total page number meta data
	IsFinished  bool   `json:"is_finished,omitempty"` // if the book is finished or still on going
	IsTakenDown bool   `json:"is_taken_down"`         // if the book has been takend down from website

	RootDir string `json:"root_dir,omitempty"`  // root directory of book
	RawDir  string `json:"raw_dir,omitempty"`   // directory for cyphered HTML output
	TextDir string `json:"text_dir,omitempty"`  // directory for decyphered HTML output
	ImgDir  string `json:"image_dir,omitempty"` // directory for downloaded images
	EpubDir string `json:"epub_dir,omitempty"`  // directory for writing epub file to

	HeaderFile  string `json:"header_file,omitempty"` // JSON header list file, containing Array<{ name: string, value: string }>
	NameMapFile string `json:"name_map,omitempty"`    // JSON file containing chapter title to file name mapping, in form of Array<{ title: string, file: string }>
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

	infoDir := filepath.Dir(infoPath)

	info.RootDir = common.ResolveRelativePath(info.RootDir, infoDir)
	info.RootDir = common.GetStrOr(info.RootDir, infoDir)
	info.RawDir = common.ResolveRelativePath(info.RawDir, info.RootDir)
	info.TextDir = common.ResolveRelativePath(info.TextDir, info.RootDir)
	info.ImgDir = common.ResolveRelativePath(info.ImgDir, info.RootDir)
	info.EpubDir = common.ResolveRelativePath(info.EpubDir, info.RootDir)

	info.HeaderFile = common.ResolveRelativePath(info.HeaderFile, info.RootDir)
	info.NameMapFile = common.ResolveRelativePath(info.NameMapFile, info.RootDir)

	return info, nil
}

// Save book info struct to file.
func (info *BookInfo) SaveFile(filename string) error {
	data, err := json.MarshalIndent(info, "", "    ")
	if err != nil {
		return fmt.Errorf("JSON conversion failed: %s", err)
	}

	err = os.WriteFile(filename, data, 0o644)
	if err != nil {
		return fmt.Errorf("failed to write info file: %s", err)
	}

	return nil
}
