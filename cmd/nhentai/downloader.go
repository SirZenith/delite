package nhentai

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/SirZenith/delite/cmd/nhentai/nhenapi"
	"github.com/SirZenith/delite/common"
	"github.com/charmbracelet/log"
	"github.com/schollz/progressbar/v3"
)

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
	if err := os.MkdirAll(dirName, 0o755); err != nil {
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
	workChan := make(chan workLoad, d.jobCount)
	resultChan := make(chan struct{}, d.jobCount)

	for i := 0; i < d.jobCount; i++ {
		go d.dlWorker(outputDir, workChan, resultChan)
	}

	if startingPage < 1 {
		startingPage = 1
	}
	go d.dlBoss(workChan, startingPage)

	recv := startingPage - 1
	total := d.Book.NumPages

	bar := progressbar.Default(int64(total))

	for range resultChan {
		recv++
		bar.Add(1)
		if recv == d.Book.NumPages {
			break
		}
	}

	close(workChan)

	return nil
}

type workLoad struct {
	pageNum int
	url     string
}

func (d *Downloader) dlBoss(workChan chan workLoad, startingPage int) {
	for i := startingPage; i <= d.Book.NumPages; i++ {
		workChan <- workLoad{
			pageNum: i,
			url:     d.Book.PageURL(i),
		}
	}
}

// dlWorker waits for tasks comes from task channel, quite if gen ending signal.
func (d *Downloader) dlWorker(outputDir string, workChan chan workLoad, resultChan chan struct{}) {
	for job := range workChan {
		if err := d.dlSingleImg(outputDir, job); err != nil {
			log.Warnf("\npage %d: %s", job.pageNum, err)
		}
		resultChan <- struct{}{}
	}
}

func (d *Downloader) dlSingleImg(outputDir string, job workLoad) error {
	ext := "." + d.Book.GetPage(job.pageNum).GetExt()
	basename := fmt.Sprintf(d.PageIndexTemplate, job.pageNum) + ext
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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error during reading response body: %s", err)
	}

	err = os.WriteFile(filename, body, 0o644)
	if err != nil {
		return fmt.Errorf("error during writing file `%s`: %s", filename, err)
	}

	return nil
}
