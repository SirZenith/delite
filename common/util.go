package common

import (
	"bufio"
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/gen2brain/avif"
	"golang.org/x/image/bmp"
	_ "golang.org/x/image/ccitt"
	"golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

// If given `value` is not empty, returns it. Else `defaultValue` will be returned.
func GetStrOr(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	} else {
		return value
	}
}

// GetDurationOr takes two duration value, if the first value is greater
// than or equal to zero, then this function return this value, else the second
// value will be returned.
func GetDurationOr(timeout, defaultValue time.Duration) time.Duration {
	if timeout < 0 {
		return defaultValue
	} else {
		return timeout
	}
}

// logBannerMsg prints a block of message to log.
func LogBannerMsg(msgs []string, paddingLen int) {
	maxLen := 0
	for i := range msgs {
		l := len(msgs[i])
		if l > maxLen {
			maxLen = l
		}
	}

	padding := strings.Repeat(" ", paddingLen)
	stem := strings.Repeat("─", maxLen+paddingLen*2)

	log.Info("╭" + stem + "╮")
	for _, line := range msgs {
		log.Info("│" + padding + line + strings.Repeat(" ", maxLen-len(line)) + padding + " ")
	}
	log.Info("╰" + stem + "╯")
}

const (
	ImageFormatAvif = "avif"
	ImageFormatBmp  = "bmp"
	ImageFormatJpeg = "jpeg"
	ImageFormatPng  = "png"
	ImageFormatTiff = "tiff"
)

var AllImageFormats = []string{
	ImageFormatAvif,
	ImageFormatBmp,
	ImageFormatJpeg,
	ImageFormatPng,
	ImageFormatTiff,
}

func ConvertImageTo(input io.Reader, output io.Writer, outputFormat string) (string, error) {
	img, _, err := image.Decode(input)
	if err != nil {
		return "", fmt.Errorf("image decoding failed: %s", err)
	}

	var outputExt string
	switch outputFormat {
	case ImageFormatAvif:
		err = avif.Encode(output, img)
		outputExt = ImageFormatAvif
	case ImageFormatBmp:
		err = bmp.Encode(output, img)
		outputExt = ImageFormatBmp
	case ImageFormatJpeg:
		err = jpeg.Encode(output, img, nil)
		outputExt = ImageFormatJpeg
	case ImageFormatPng:
		err = png.Encode(output, img)
		outputExt = ImageFormatPng
	case ImageFormatTiff:
		err = tiff.Encode(output, img, nil)
		outputExt = ImageFormatTiff
	default:
		err = png.Encode(output, img)
		outputExt = ImageFormatPng
	}

	if err != nil {
		return "", fmt.Errorf("failed to encode image as %s: %s", outputExt, err)
	}

	return outputExt, nil
}

// SaveImageAs treats given byte slice as raw image data, and convert it to given
// format then saves it to disk.
func SaveImageAs(data []byte, outputName string, outputFormat string) error {
	file, err := os.Create(outputName)
	if err != nil {
		return fmt.Errorf("failed to create output image file %s: %s", outputName, err)
	}
	defer file.Close()

	bufWriter := bufio.NewWriter(file)
	defer bufWriter.Flush()

	reader := bytes.NewReader(data)
	_, err = ConvertImageTo(reader, bufWriter, outputFormat)
	if err != nil {
		return fmt.Errorf("failed to save image as PNG %s: %s", outputName, err)
	}

	return nil
}

func ConvertBookSrcURLToAbs(tocURL *url.URL, src string) (*url.URL, error) {
	parsedSrc, err := url.Parse(src)
	if err != nil {
		return nil, fmt.Errorf("invalid source URL %q: %s", parsedSrc, err)
	}

	if parsedSrc.Scheme == "" {
		parsedSrc.Scheme = tocURL.Scheme
	}

	if parsedSrc.Host == "" {
		parsedSrc.Host = tocURL.Host
	}

	return parsedSrc, nil
}

func GetMangaPageOutputBasename(chapterIndex int, pageIndex int, format string) string {
	return fmt.Sprintf("%04d - %03d.%s", chapterIndex, pageIndex, format)
}
