package common

import "fmt"

// volume name used for books with only one volume, and should be omitted in output
// book's name.
const SingleVolumeName = "_"

// CombineOutputName generates book output name with given book title and volume
// name.
func CombineOutputName(book, volume string) string {
	if volume == SingleVolumeName {
		return book
	} else {
		return fmt.Sprintf("%s %s", book, volume)
	}
}
