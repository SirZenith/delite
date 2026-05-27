package book_management

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/SirZenith/delite/common"
)

const (
	LocalBookTypeEpub  = "epub"
	LocalBookTypeHTML  = "html"
	LocalBookTypeImage = "image"
	LocalBookTypeLatex = "latex"
	LocalBookTypePdf   = "pdf"
	LocalBookTypeTypst = "typst"
	LocalBookTypeZip   = "zip"
)

var AllLocalBookType = []string{
	LocalBookTypeEpub,
	LocalBookTypeImage,
	LocalBookTypeLatex,
	LocalBookTypeTypst,
	LocalBookTypeZip,
}

type LocalInfo struct {
	Type         string         `json:"book_type"`
	BundleOption map[string]any `json:"bundle_option"`
}

type LatexBookInfo struct {
	TemplateFile     string `json:"template_file"`
	PreprocessScript string `json:"preprocess_script"`
	IsHorizontal     *bool  `json:"is_horizontal,omitempty"`
}

type BookMeta struct {
	Rating      float64  `json:"rating"`      // Rating of this book
	Description []string `json:"description"` // Book Description
	Genre       []string `json:"genre"`       // Book genre tags
	Status      int      `json:"status"`      // ongoing, completed, licensed, publishing, cancelled, on hiatus, etc.

	IsRead bool `json:"is_read,omitempty"` // if all volume of this book series is read.

	IsTakenDown          bool `json:"is_taken_down,omitempty"`           // if the book has been takend down from website
	IsHasLocalVersion    bool `json:"is_has_local_version,omitempty"`    // if this book has a local coressponding
	IsPreferLocalVersion bool `json:"is_prefer_local_version,omitempty"` // when set to true, book transfer process should use local version of this book
	NoTranster           bool `json:"no_transfer,omitempty"`             // when set to true, book will not be transfered to ebook device
}

// Represents infomation about a single book.
type BookInfo struct {
	Title  string `json:"title"`   // Book title
	Author string `json:"author"`  // Book author
	Artist string `json:"artist"`  // Book artist
	TocURL string `json:"toc_url"` // URL to book's table of contents page

	RootDir     string `json:"root_dir,omitempty"`     // root directory of book
	RawDir      string `json:"raw_dir,omitempty"`      // directory for cyphered HTML output
	TextDir     string `json:"text_dir,omitempty"`     // directory for decyphered HTML output
	ImgDir      string `json:"image_dir,omitempty"`    // directory for downloaded images
	EpubDir     string `json:"epub_dir,omitempty"`     // directory for writing epub file to
	LatexDir    string `json:"latex_dir,omitempty"`    // directory for writing latex file to
	MarkdownDir string `json:"markdown_dir,omitempty"` // directory for writing markdown file to
	PdfDir      string `json:"pdf_dir,omitempty"`      // directory for storing PDF book to
	TypstDir    string `json:"typst_dir,omitempty"`    // directory for writing typst file to
	ZipDir      string `json:"zip_dir,omitempty"`      // directory for writing manga zip archive to

	HeaderFile string `json:"header_file,omitempty"` // JSON header list file, containing Array<{ name: string, value: string }>

	LocalInfo *LocalInfo `json:"local,omitempty"` // extra info for local book

	Meta *BookMeta `json:"meta,omitempty"`
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
	info.LatexDir = common.ResolveRelativePath(info.LatexDir, info.RootDir)
	info.PdfDir = common.ResolveRelativePath(info.PdfDir, info.RootDir)
	info.ZipDir = common.ResolveRelativePath(info.ZipDir, info.RootDir)

	info.HeaderFile = common.ResolveRelativePath(info.HeaderFile, info.RootDir)

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
