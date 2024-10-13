package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gocolly/colly/v2"
)

type Options struct {
	targetURL    string
	outputDir    string
	requestDelay time.Duration
	timeout      time.Duration
	cookie       string
	headerFile   string
}

func main() {
	options := Options{
		targetURL:    "https://www.bilinovel.com/novel/3181/catalog",
		outputDir:    "以为转生就能逃掉吗，哥哥？",
		requestDelay: 800 * time.Millisecond,
		timeout:      5 * time.Second,
		headerFile:   "./header.json",
	}

	c, err := makeCollector(options)
	if err != nil {
		log.Fatalln(err)
	}

	c.Visit(options.targetURL)
}

func makeCollector(options Options) (*colly.Collector, error) {
	if stat, err := os.Stat(options.outputDir); errors.Is(err, os.ErrNotExist) {
		if err = os.MkdirAll(options.outputDir, 0o755); err != nil {
			return nil, fmt.Errorf("failed to create output directory: %s", err)
		}
	} else if !stat.IsDir() {
		return nil, fmt.Errorf("An file with name %s already exists", options.outputDir)
	}

	headers, err := readHeaderFile(options.headerFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read header file: %s", err)
	}

	c := colly.NewCollector(
		colly.Headers(headers),
	)

	c.Limit(&colly.LimitRule{
		DomainGlob: "*.bilinovel.com",
		Delay:      options.requestDelay,
		// Parallelism: 2,
	})

	c.OnRequest(func(r *colly.Request) {
		r.Ctx.Put("options", &options)
	})
	c.OnError(func(r *colly.Response, err error) {
		if onError, ok := r.Ctx.GetAny("onError").(colly.ErrorCallback); ok {
			onError(r, err)
		} else {
			log.Printf("error requesting %s: %s", r.Request.URL, err)
		}
	})
	c.OnHTML("li.chapter-li a.chapter-li-a", onChapterAddress)
	c.OnHTML("body#aread", onPageContent)

	return c, nil
}

func makeCookie(targetURL, cookie string) (http.CookieJar, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	cookies := []*http.Cookie{}

	pairs := strings.Split(cookie, ";")
	for _, pair := range pairs {
		pair = strings.TrimLeft(pair, " ")
		parts := strings.SplitN(pair, "=", 2)

		if len(parts) == 2 {
			cookies = append(cookies, &http.Cookie{
				Name:  parts[0],
				Value: parts[1],
			})
		}
	}

	hostURL, err := url.Parse(targetURL)
	jar.SetCookies(hostURL, cookies)

	return jar, nil
}

type HeaderValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func readHeaderFile(path string) (map[string]string, error) {
	result := map[string]string{}

	data, err := os.ReadFile(path)
	if err != nil {
		return result, err
	}

	list := []HeaderValue{}
	json.Unmarshal(data, &list)

	for _, entry := range list {
		result[entry.Name] = entry.Value
	}

	return result, nil
}
