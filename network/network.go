package network

import (
	"bufio"
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"os"

	"github.com/charmbracelet/log"
	_ "github.com/gen2brain/avif"
	"github.com/gocolly/colly/v2"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/ccitt"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

// MakeSaveBodyCallback returns a closure that saves response body to given path
// and can be used as colly onResponse callback.
func MakeSaveBodyCallback(outputName string) colly.ResponseCallback {
	return colly.ResponseCallback(func(resp *colly.Response) {
		if err := resp.Save(outputName); err == nil {
			log.Infof("file downloaded: %s", outputName)
		} else {
			log.Warnf("failed to save file %s: %s\n", outputName, err)
		}
	})
}

// SaveBodyAsPNG try to decode response body as an image, and save it to given
// path as a PNG image.
func SaveBodyAsPNG(resp *colly.Response, outputName string) error {
	img, _, err := image.Decode(bytes.NewReader(resp.Body))
	if err != nil {
		return fmt.Errorf("failed to decode image %s: %s", resp.Request.URL, err)
	}

	file, err := os.Create(outputName)
	if err != nil {
		return fmt.Errorf("failed to create output image file %s: %s", outputName, err)
	}
	defer file.Close()

	bufWriter := bufio.NewWriter(file)
	defer bufWriter.Flush()

	err = png.Encode(bufWriter, img)
	if err != nil {
		return fmt.Errorf("failed to save image as PNG %s: %s", outputName, err)
	}

	return nil
}
