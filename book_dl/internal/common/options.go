package common

import "github.com/gocolly/colly/v2"

type Options struct {
	RequestDelay int64 // delay for each download request
	Timeout      int64 // download timeout

	Targets []DlTarget
}

type DlTarget struct {
	RequestDelay int64 // delay for each download request
	Timeout      int64 // download timeout

	Title  string
	Author string

	TargetURL    string // TOC URL for novel
	OutputDir    string // output directory for downloaded HTML page
	ImgOutputDir string // output directory for downloaded images

	HeaderFile         string // header file path
	ChapterNameMapFile string // chapter name mapping JSON file path
}

type CtxGlobal struct {
	Target    *DlTarget
	Collector *colly.Collector
	NameMap   *GardedNameMap
}
