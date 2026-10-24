package gelbooru

import (
	"strings"
	"testing"
)

func TestGenTargetListWithThumbnailURL(t *testing.T) {
	thumbnailURL := "https://img4.gelbooru.com/thumbnails/aa/bb/thumbnail_0123456789abcdef.jpg"

	urlList, err := genTargetListWithThumbnailURL(thumbnailURL)
	if err != nil {
		t.Fatalf("genTargetListWithThumbnailURL() error = %v", err)
	}
	if len(urlList) == 0 {
		t.Fatal("genTargetListWithThumbnailURL() returned no URL")
	}

	expected := "https://img4.gelbooru.com/images/aa/bb/0123456789abcdef.jpg"
	if urlList[0] != expected {
		t.Errorf("first URL = %q, want %q", urlList[0], expected)
	}

	hasWebm := false
	for _, u := range urlList {
		if strings.Contains(u, "img.gelbooru.com") || strings.Contains(u, "img3.gelbooru.com") {
			t.Errorf("dead CDN domain still used: %q", u)
		}
		if strings.Contains(u, "://images") || strings.Contains(u, "//images") {
			t.Errorf("malformed URL with duplicated slash: %q", u)
		}
		if strings.HasSuffix(u, ".webm") {
			hasWebm = true
		}
	}
	if !hasWebm {
		t.Error("no .webm candidate generated")
	}
}
