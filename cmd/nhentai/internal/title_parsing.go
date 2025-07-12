package nhentai

import (
	"fmt"
	"strings"

	"github.com/SirZenith/delite/common"
)

type SegmentType int

const (
	SegmentNone SegmentType = iota
	SegmentText
	SegmentBracket
	SegmentParen
)

type segment struct {
	segType SegmentType
	st      int
	ed      int
}

const placeholderTitleText = "未知"

func GetMangaTitle(rawTitle string) string {
	segments := make([]segment, 3)
	segType, st, ed := SegmentNone, 0, 0

	appendSegment := func() {
		segments = append(segments, segment{
			segType, st, ed,
		})
	}

	tryEndState := func(index int, targetType, newType SegmentType) {
		if segType != targetType {
			return
		}
		ed = index
		if segType != SegmentNone {
			appendSegment()
		}
		segType = newType
		st = index
		if newType != SegmentText {
			st++
		}
	}

	tryBeginState := func(index int, newType SegmentType) {
		switch segType {
		case SegmentText:
			if newType != SegmentText {
				tryEndState(index, SegmentText, newType)
			}
		case SegmentNone:
			tryEndState(index, SegmentNone, newType)
		}
	}

	// parsing title
	titleRunes := []rune(rawTitle)
	for i := 0; i < len(titleRunes); i++ {
		curr := string(titleRunes[i])
		switch true {
		case curr == "(":
			tryBeginState(i, SegmentParen)
		case curr == "[":
			tryBeginState(i, SegmentBracket)
		case curr == ")":
			tryEndState(i, SegmentParen, SegmentNone)
		case curr == "]":
			tryEndState(i, SegmentBracket, SegmentNone)
		case curr == " ":

		default:
			tryBeginState(i, SegmentText)
		}
	}
	if segType != SegmentNone {
		tryEndState(len(titleRunes), segType, SegmentNone)
	}

	var paro, title, artist string
	for _, seg := range segments {
		text := string(titleRunes[seg.st:seg.ed])
		text = strings.TrimSpace(text)

		switch seg.segType {
		case SegmentParen:
			if artist != "" {
				if paro != "" {
					// when there's multiple parenthesis segments, only the last one
					// shall be treated as manga parody.
					title = fmt.Sprintf("%s (%s)", title, paro)
				}
				paro = text
			}
		case SegmentBracket:
			if artist == "" {
				artist = text
			}
		case SegmentText:
			if title == "" {
				title = text
			}
		}
	}

	paro = common.GetStrOr(paro, placeholderTitleText)
	title = common.GetStrOr(title, placeholderTitleText)
	artist = common.GetStrOr(artist, placeholderTitleText)

	result := fmt.Sprintf("%s - %s - %s", paro, title, artist)
	result = common.InvalidPathCharReplace(result)

	return result
}
