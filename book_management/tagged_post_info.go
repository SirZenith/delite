package book_management

type TaggedPostInfo struct {
	Title   string `json:"title"`    // Book title
	Tag     string `json:"tag"`      // post tag
	PageCnt int    `json:"page_cnt"` // total page number
}
