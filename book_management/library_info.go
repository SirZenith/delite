package book_management

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"

	"github.com/SirZenith/delite/common"
)

type HeaderFilePattern struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
}

// Represents information about a library directory
type LibraryInfo struct {
	RootDir      string `json:"root"`       // root directory of library
	RawDirName   string `json:"raw_name"`   // name for directory for cyphered HTML output in each book directory, if not specified by book info
	TextDirName  string `json:"text_name"`  // name for directory for decyphered HTML output in each book directory, if not specified by book info
	ImgDirName   string `json:"image_name"` // name for directory for downloaded images in each book directory, if not specified by book info
	EpubDirName  string `json:"epub_name"`  // name for directory for writing epub file to in each book directory, if not specified by book info
	LatexDirName string `json:"latex_name"` // name for directory for writing latex file to in each book directory, if not specified by book info

	NameMapFile string `json:"name_map_file_name"` // JSON file containing chapter title to file name mapping, in form of Array<{ title: string, file: string }>

	HeaderFileList []HeaderFilePattern `json:"header_file_map"` // Mapping domain glob string to header file path used by matching domains.

	Books []BookInfo `json:"books"` // a list of book info
}

func ReadLibraryInfo(infoPath string) (*LibraryInfo, error) {
	data, err := os.ReadFile(infoPath)
	if err != nil {
		return nil, fmt.Errorf("can't read info file %s: %s", infoPath, err)
	}

	info := &LibraryInfo{}
	if err := json.Unmarshal(data, info); err != nil {
		return nil, fmt.Errorf("unable to parse info data in %s: %s", infoPath, err)
	}

	info.SetupDefaultValues()

	for i := range info.HeaderFileList {
		entry := &info.HeaderFileList[i]
		entry.Path = common.ResolveRelativePath(entry.Path, info.RootDir)
	}

	for i := range info.Books {
		book := &info.Books[i]

		if book.Title == "" {
			return nil, fmt.Errorf("book %d contains no title", i)
		}

		book.RootDir = common.GetStrOr(book.RootDir, filepath.Join(info.RootDir, book.Title))
		book.RootDir = common.ResolveRelativePath(book.RootDir, info.RootDir)

		book.RawDir = common.GetStrOr(book.RawDir, info.RawDirName)
		book.RawDir = common.ResolveRelativePath(book.RawDir, book.RootDir)

		book.TextDir = common.GetStrOr(book.TextDir, info.TextDirName)
		book.TextDir = common.ResolveRelativePath(book.TextDir, book.RootDir)

		book.ImgDir = common.GetStrOr(book.ImgDir, info.ImgDirName)
		book.ImgDir = common.ResolveRelativePath(book.ImgDir, book.RootDir)

		book.EpubDir = common.GetStrOr(book.EpubDir, info.EpubDirName)
		book.EpubDir = common.ResolveRelativePath(book.EpubDir, book.RootDir)

		book.LatexDir = common.GetStrOr(book.LatexDir, info.LatexDirName)
		book.LatexDir = common.ResolveRelativePath(book.LatexDir, book.RootDir)

		book.NameMapFile = common.GetStrOr(book.NameMapFile, info.NameMapFile)
		book.NameMapFile = common.ResolveRelativePath(book.NameMapFile, book.RootDir)

		// header file path may be provided library wide, so ResolveRelativePath
		// is called before using value provided by library.
		book.HeaderFile = common.ResolveRelativePath(book.HeaderFile, book.RootDir)
		book.HeaderFile = common.GetStrOr(book.HeaderFile, info.GetHeaderFileFor(book.TocURL))
	}

	return info, nil
}

// SetupDefaultValues sets necessary default values for LibraryInfo fields if
// them are still zero value of their type.
func (info *LibraryInfo) SetupDefaultValues() {
	if info.RootDir == "" {
		info.RootDir = "./"
	}

	if info.HeaderFileList == nil {
		info.HeaderFileList = []HeaderFilePattern{}
	}

	if info.Books == nil {
		info.Books = []BookInfo{}
	}
}

// GetHeaderFileFor returns header file path for given URL.
func (info *LibraryInfo) GetHeaderFileFor(urlStr string) string {
	target := ""

	u, err := url.Parse(urlStr)
	if err != nil {
		return target
	}

	hostname := u.Hostname()
	for _, entry := range info.HeaderFileList {
		ok, err := path.Match(entry.Pattern, hostname)
		if err == nil && ok {
			target = entry.Path
		}
	}

	return target
}

// Save book info struct to file.
func (info *LibraryInfo) SaveFile(filename string) error {
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
