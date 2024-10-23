package common

type Options struct {
	TargetURL    string // TOC URL for novel
	OutputDir    string // output directory for downloaded HTML page
	ImgOutputDir string // output directory for downloaded images

	HeaderFile         string // header file path
	ChapterNameMapFile string // chapter name mapping JSON file path

	RequestDelay int64 // delay for each download request
	Timeout      int64 // download timeout
}
