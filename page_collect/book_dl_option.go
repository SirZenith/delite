package page_collect

import (
	"sync"
	"time"

	"github.com/gocolly/colly/v2"
	"gorm.io/gorm"
)

type CtxGlobal struct {
	Target    *DlTarget
	Collector *colly.Collector
	Db        *gorm.DB
	Link      *ChapterLink
}

func NewCtxGlobal() *CtxGlobal {
	return &CtxGlobal{
		Link: &ChapterLink{
			visited:    map[int64]struct{}{},
			urlMap:     map[int64]string{},
			volInfoMap: map[int64]*VolumeInfo{},
		},
	}
}

type Options struct {
	Timeout    time.Duration      // download timeout
	RetryCnt   int64              // retry count for each page download request
	LimitRules []*colly.LimitRule // a list of requeest limit rule.

	IgnoreTakenDownFlag bool // also process books that has been taken down
}

type DlTarget struct {
	Options *Options

	Title  string
	Author string

	TargetURL    string // TOC URL for novel
	OutputDir    string // output directory for downloaded HTML page
	ImgOutputDir string // output directory for downloaded images

	HeaderFile string // header file path
	DbPath     string // path to book database file

	IsTakenDown bool
	IsLocal     bool
}

// Mapping volume and chapter index to absolute URL.
type ChapterLink struct {
	lock       sync.Mutex
	visited    map[int64]struct{}
	urlMap     map[int64]string
	volInfoMap map[int64]*VolumeInfo
}

func (c *ChapterLink) makeKey(volIndex, chapIndex int) int64 {
	return (int64(volIndex) << 32) + int64(chapIndex)
}

func (c *ChapterLink) CheckVisited(volIndex, chapIndex int) bool {
	c.lock.Lock()
	defer c.lock.Unlock()

	key := c.makeKey(volIndex, chapIndex)
	_, ok := c.visited[key]

	return ok
}

func (c *ChapterLink) MarkVisited(volIndex, chapIndex int) {
	c.lock.Lock()
	defer c.lock.Unlock()

	key := c.makeKey(volIndex, chapIndex)
	c.visited[key] = struct{}{}
}

func (c *ChapterLink) GetAndRemoveURL(volIndex, chapIndex int) string {
	c.lock.Lock()
	defer c.lock.Unlock()

	key := c.makeKey(volIndex, chapIndex)
	value, ok := c.urlMap[key]
	if ok {
		delete(c.urlMap, key)
	}

	return value
}

func (c *ChapterLink) SetURL(volIndex, chapIndex int, url string) {
	c.lock.Lock()
	defer c.lock.Unlock()

	key := c.makeKey(volIndex, chapIndex)
	c.urlMap[key] = url
}

func (c *ChapterLink) GetAndRemoveVolInfo(volIndex, chapIndex int) *VolumeInfo {
	c.lock.Lock()
	defer c.lock.Unlock()

	key := c.makeKey(volIndex, chapIndex)
	value, ok := c.volInfoMap[key]
	if ok {
		delete(c.volInfoMap, key)
	}

	return value
}

func (c *ChapterLink) SetVolInfo(volIndex, chapIndex int, info *VolumeInfo) {
	c.lock.Lock()
	defer c.lock.Unlock()

	key := c.makeKey(volIndex, chapIndex)
	c.volInfoMap[key] = info
}
