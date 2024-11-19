package nhentai

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/SirZenith/delite/cmd/nhentai/nhenapi"
	"github.com/SirZenith/delite/common"
	"github.com/charmbracelet/log"
	"github.com/schollz/progressbar/v3"
)

const imageOutputFormat = common.ImageFormatAvif

// ----------------------------------------------------------------------------

type Downloader struct {
	client     *nhenapi.NhenClient
	jobCount   int
	retryCount int

	CurBookId         int
	Book              *nhenapi.Book
	Title             string
	PageIndexTemplate string
}

func NewDownloader(jobCnt, retryCount int) *Downloader {
	return &Downloader{
		client:     nhenapi.NewNhenClient(),
		jobCount:   jobCnt,
		retryCount: retryCount,
	}
}

func (d *Downloader) InitClient(headers map[string]string, httpProxy, httpsProxy string) {
	d.client.SetHeaders(headers)

	if httpProxy != "" || httpsProxy != "" {
		d.client.SetProxy(httpProxy, httpsProxy)
	}
}

// -----------------------------------------------------------------------------
// Metadata

func (d *Downloader) GetBook(bookID int) error {
	book, err := d.client.GetBook(bookID)
	if err != nil {
		return fmt.Errorf("failed to fetch book info for %d: %s", bookID, err)
	}

	d.CurBookId = bookID
	d.Book = book

	rawTitle := common.GetStrOr(book.Title.Japanese, book.Title.English)
	d.Title = getMangaTitle(rawTitle)

	d.PageIndexTemplate = makeIndexTemplate(book.NumPages)

	return nil
}

func makeIndexTemplate(pageCnt int) string {
	length := 0
	for i := pageCnt; i > 0; i /= 10 {
		length++
	}
	return fmt.Sprintf("%%0%dd", length)
}

type BookInfoDump struct {
	ID   int          `json:"book_id"`
	Info nhenapi.Book `json:"info"`
}

func (d *Downloader) DumpBookInfo(filePath string) error {
	dirName := filepath.Dir(filePath)
	if err := os.MkdirAll(dirName, 0o777); err != nil {
		return fmt.Errorf("failed to create info output directory %s: %s", dirName, err)
	}

	dumpInfo := BookInfoDump{
		ID:   d.CurBookId,
		Info: *d.Book,
	}

	data, err := json.MarshalIndent(dumpInfo, "", "    ")
	if err != nil {
		return fmt.Errorf("failed to convert book info to JSON: %s", err)
	}

	err = os.WriteFile(filePath, data, 0o644)
	if err != nil {
		return fmt.Errorf("failed to write info file %s: %s", filePath, err)
	}

	return nil
}

// -----------------------------------------------------------------------------
// Download

// StartDownload spawns workers and distribute tasks to them to complete manga
// downloading.
func (d *Downloader) StartDownload(outputDir string, startingPage int) error {
	if startingPage < 1 {
		startingPage = 1
	}

	bar := progressbar.Default(int64(d.Book.NumPages))
	workChan := make(chan workLoad, d.jobCount)
	var group sync.WaitGroup

	for i := d.jobCount; i > 0; i-- {
		go d.dlWorker(outputDir, workChan, &group, bar)
	}

	for i := startingPage; i <= d.Book.NumPages; i++ {
		group.Add(1)
		workChan <- workLoad{
			pageNum: i,
			url:     d.Book.PageURL(i),
		}
	}

	group.Wait()

	return nil
}

type workLoad struct {
	pageNum int
	url     string
}

// dlWorker waits for tasks comes from task channel, quite if gen ending signal.
func (d *Downloader) dlWorker(outputDir string, workChan chan workLoad, group *sync.WaitGroup, bar *progressbar.ProgressBar) {
	for job := range workChan {
		if err := d.dlSingleImg(outputDir, job); err != nil {
			log.Warnf("\npage %d: %s", job.pageNum, err)
		}
		group.Done()
		bar.Add(1)
	}
}

func (d *Downloader) dlSingleImg(outputDir string, job workLoad) error {
	basename := fmt.Sprintf(d.PageIndexTemplate, job.pageNum) + "." + imageOutputFormat
	filename := filepath.Join(outputDir, basename)

	var err error
	for i := 0; i < d.retryCount; i++ {
		err = d.tryDl(job.url, filename)
		if err == nil {
			break
		}
	}

	return err
}

// tryDl will try to download image from given URL, return error if any on step
// of requesting, read data, write file failed.
func (d *Downloader) tryDl(url, filename string) error {
	resp, err := d.client.Do("GET", url, nil)
	if err != nil {
		return fmt.Errorf("error during get remote data: %s", err)
	}
	defer resp.Body.Close()

	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create output file %s: %s", filename, err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

	_, err = common.ConvertImageTo(resp.Body, writer, imageOutputFormat)
	if err != nil {
		return err
	}

	return nil
}
