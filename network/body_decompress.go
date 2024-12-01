package network

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"fmt"
	"io"

	"github.com/andybalholm/brotli"
	"github.com/gocolly/colly/v2"
	"github.com/klauspost/compress/zstd"
)

type bodyDecompressFunc = func([]byte) ([]byte, error)
type decompressorFactory = func(io.Reader) (io.Reader, error)

func DecompressResponseBody(r *colly.Response) ([]byte, error) {
	encoding := r.Headers.Get("content-encoding")
	decompressFunc, err := getBodyDecompressFunc(encoding)
	if err != nil {
		return nil, err
	}

	data, err := decompressFunc(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress response: %s", err)
	}

	return data, nil
}

// Returns a byte decompress function according to encoding type.
func getBodyDecompressFunc(encoding string) (bodyDecompressFunc, error) {
	switch encoding {
	case "br":
		return brotliDecompress, nil
	case "deflate":
		return flateDecompress, nil
	case "gzip":
		return gzipDecompress, nil
	case "zstd":
		return zstdDecompress, nil
	case "", "identity":
		return noDecompress, nil
	default:
		return nil, fmt.Errorf("unknown content-encoding: %s", encoding)
	}
}

// Decompresses given data with decompress function.
func decompressBodyWith(body []byte, factory decompressorFactory) ([]byte, error) {
	byteReader := bytes.NewReader(body)

	reader, err := factory(byteReader)
	if err != nil {
		return nil, err
	}

	output, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	return output, nil
}

// An no-opt decompress function. Input data will be returned directly.
func noDecompress(body []byte) ([]byte, error) {
	return body, nil
}

// brotliDecompress decodes data with brotli
func brotliDecompress(body []byte) ([]byte, error) {
	return decompressBodyWith(body, func(reader io.Reader) (io.Reader, error) {
		return brotli.NewReader(reader), nil
	})
}

// flateDecompress decodes data with flate
func flateDecompress(body []byte) ([]byte, error) {
	return decompressBodyWith(body, func(reader io.Reader) (io.Reader, error) {
		return flate.NewReader(reader), nil
	})
}

// gzipDecompress decodes data with gzip.
func gzipDecompress(body []byte) ([]byte, error) {
	return decompressBodyWith(body, func(reader io.Reader) (io.Reader, error) {
		return gzip.NewReader(reader)
	})
}

// zstdDecompress decodes data with zstd.
func zstdDecompress(body []byte) ([]byte, error) {
	return decompressBodyWith(body, func(reader io.Reader) (io.Reader, error) {
		return zstd.NewReader(reader)
	})
}
