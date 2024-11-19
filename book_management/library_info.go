package book_management

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/SirZenith/delite/common"
	"github.com/gocolly/colly/v2"
)

type HeaderFilePattern struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
}

type LimitRule struct {
	DomainRegexp string        `json:"domain_regex,omitempty"`
	DomainGlob   string        `json:"domain_glob,omitempty"`
	Delay        time.Duration `json:"delay,omitempty"`
	RandomDelay  time.Duration `json:"random_delay,omitempty"`
	Parallelism  int           `json:"parallelism,omitempty"`
}

func (rule *LimitRule) ToCollyLimitRule() *colly.LimitRule {
	return &colly.LimitRule{
		DomainRegexp: rule.DomainRegexp,
		DomainGlob:   rule.DomainGlob,
		Delay:        rule.Delay,
		RandomDelay:  rule.RandomDelay,
		Parallelism:  rule.Parallelism,
	}
}

type LatexLibConfig struct {
	TemplateFile     string `json:"template_file"`
	PreprocessScript string `json:"preprocess_script"`
}

// Represents information about a library directory
type LibraryInfo struct {
	RootDir      string `json:"root"`       // root directory of library
	RawDirName   string `json:"raw_name"`   // name for directory for cyphered HTML output in each book directory, if not specified by book info
	TextDirName  string `json:"text_name"`  // name for directory for decyphered HTML output in each book directory, if not specified by book info
	ImgDirName   string `json:"image_name"` // name for directory for downloaded images in each book directory, if not specified by book info
	EpubDirName  string `json:"epub_name"`  // name for directory for writing epub file to in each book directory, if not specified by book info
	LatexDirName string `json:"latex_name"` // name for directory for writing latex file to in each book directory, if not specified by book info
	ZipDirName   string `json:"zip_name"`   // name for directory for writing manga zip archive to in each book directory, if not specified by book info

	DatabasePath string `json:"database_path"` // path to sqlite database file.

	HeaderFileList []HeaderFilePattern `json:"header_file_map"` // Mapping domain glob string to header file path used by matching domains.
	LimitRules     []LimitRule         `json:"limit,omitempty"` // limit rules for colly collector

	LatexConfig LatexLibConfig `json:"latex_config"` // global latex config value for all books

	Books         []BookInfo         `json:"books,omitempty"`          // a list of book info
	GelbooruBooks []GelbooruBookInfo `json:"gelbooru_books,omitempty"` // a list of gelbooru tag info
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

	infoDir := filepath.Dir(infoPath)
	info.RootDir = common.ResolveRelativePath(info.RootDir, infoDir)
	info.DatabasePath = common.ResolveRelativePath(info.DatabasePath, infoDir)

	for i := range info.HeaderFileList {
		entry := &info.HeaderFileList[i]
		entry.Path = common.ResolveRelativePath(entry.Path, info.RootDir)
	}

	for i := range info.Books {
		book := &info.Books[i]

		if book.Title == "" {
			return nil, fmt.Errorf("book %d contains no title", i)
		}

		book.RootDir = common.GetStrOr(book.RootDir, book.Title)
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

		book.ZipDir = common.GetStrOr(book.ZipDir, info.ZipDirName)
		book.ZipDir = common.ResolveRelativePath(book.ZipDir, book.RootDir)

		// header file path may be provided library wide, so ResolveRelativePath
		// is called before using value provided by library.
		book.HeaderFile = common.ResolveRelativePath(book.HeaderFile, book.RootDir)
		book.HeaderFile = common.GetStrOr(book.HeaderFile, info.GetHeaderFileFor(book.TocURL))

		if book.LatexInfo != nil {
			latexInfo := book.LatexInfo

			latexInfo.TemplateFile = common.GetStrOr(latexInfo.TemplateFile, info.LatexConfig.TemplateFile)
			latexInfo.TemplateFile = common.ResolveRelativePath(latexInfo.TemplateFile, book.RootDir)

			latexInfo.PreprocessScript = common.GetStrOr(latexInfo.PreprocessScript, info.LatexConfig.PreprocessScript)
			latexInfo.PreprocessScript = common.ResolveRelativePath(latexInfo.PreprocessScript, book.RootDir)
		}
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
