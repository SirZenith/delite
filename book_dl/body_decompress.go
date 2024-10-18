package book_dl

import (
	"bytes"
	"compress/gzip"
	"io"
	"log"

	"github.com/klauspost/compress/zstd"
)

type bodyDecompressFunc = func([]byte) ([]byte, error)

// Returns a byte decompress function according to encoding type.
func getBodyDecompressFunc(encoding string) bodyDecompressFunc {
	switch encoding {
	case "gzip":
		return gzipDecompress
	case "zstd":
		return zstdDecompress
	default:
		if encoding == "" {
			return noDecompress
		}

		log.Println("unhandled content-encoding:", encoding)
		return nil
	}
}

// Decompresses given data with decompress function.
func bodyDecompress(body []byte, decompressMaker func(io.Reader) (io.Reader, error)) ([]byte, error) {
	byteReader := bytes.NewReader(body)

	reader, err := decompressMaker(byteReader)
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

// Decompresses data with gzip.
func gzipDecompress(body []byte) ([]byte, error) {
	return bodyDecompress(body, func(reader io.Reader) (io.Reader, error) {
		return gzip.NewReader(reader)
	})
}

// Decompress data with zstd.
func zstdDecompress(body []byte) ([]byte, error) {
	return bodyDecompress(body, func(reader io.Reader) (io.Reader, error) {
		return zstd.NewReader(reader)
	})
}
