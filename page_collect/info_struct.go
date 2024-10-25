package page_collect

import (
	"fmt"
	"path/filepath"

	"github.com/SirZenith/litnovel-dl/common"
)

type VolumeInfo struct {
	VolIndex        int
	Title           string
	TotalChapterCnt int

	OutputDir    string
	ImgOutputDir string
}

type ChapterInfo struct {
	VolumeInfo
	ChapIndex int    // chapter index of this chapter
	Title     string // chapter title

	URL string // absolute URL of the first page of chapter
}

type PageContent struct {
	PageNumber int    // page number of this content in this chapter
	Title      string // display title of the chapter will be update to this value if it's not empty
	Content    string // page content

	NextChapterURL string // when non-empty, it's value will be used to initialize downloading of next chapter

	Err error
}

type ChapterDownloadState struct {
	Info          ChapterInfo
	RootNameExt   string
	RootNameStem  string
	ResultChan    chan PageContent
	CurPageNumber int
}

// Composes outputpath of chapter content with chapter info.
func (c *ChapterInfo) GetChapterOutputPath(title string) string {
	outputTitle := common.InvalidPathCharReplace(title)
	if outputTitle == "" {
		outputTitle = fmt.Sprintf("Chap.%04d.html", c.ChapIndex)
	} else {
		outputTitle = fmt.Sprintf("%04d - %s.html", c.ChapIndex, outputTitle)
	}

	return filepath.Join(c.OutputDir, outputTitle)
}

// Composes chapter key used by name map look up.
func (c *ChapterInfo) GetNameMapKey(title string) string {
	return fmt.Sprintf("%03d-%04d-%s", c.VolIndex, c.ChapIndex, title)
}

func (c *ChapterInfo) GetLogName(title string) string {
	return fmt.Sprintf("Vol.%03d - Chap.%04d - %s", c.VolIndex, c.ChapIndex, title)
}
