package book_management

import (
	"strconv"
	"strings"
)

type SearchKeyword struct {
	raw string

	isNum bool
	index int
}

func NewSearchKeyword(text string) *SearchKeyword {
	keyword := &SearchKeyword{
		raw: text,
	}

	index, err := strconv.Atoi(text)
	if err == nil {
		keyword.isNum = true
		keyword.index = index
	}

	return keyword
}

// MatchBook checks if given book info is matched by current keyword.
func (word *SearchKeyword) MatchBook(index int, book BookInfo) bool {
	if word.raw == "" {
		return true
	}

	if word.isNum && word.index == index {
		return true
	}

	if strings.Contains(book.Author, word.raw) {
		return true
	}

	if strings.Contains(book.Title, word.raw) {
		return true
	}

	return false
}

// MatchTaggedPost checks if given tagged post info is matched by current keyword.
func (word *SearchKeyword) MatchTaggedPost(index int, tag TaggedPostInfo) bool {
	if word.raw == "" {
		return true
	}

	if word.isNum && word.index == index {
		return true
	}

	if strings.Contains(tag.Title, word.raw) {
		return true
	}

	if strings.Contains(tag.Tag, word.raw) {
		return true
	}

	return false
}
