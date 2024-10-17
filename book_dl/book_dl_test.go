package book_dl

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestFontDecypher(t *testing.T) {
	sample := `
<!doctype html>
<html>

<head>
    <meta http-equiv="Content-Type" content="text/html; charset=utf-8" />
    <style>
        @font-face {
            font-family: read;
            font-display: block;
            src: url('/public/font/read.woff2') format('woff2'), url('/public/font/read.ttf') format('truetype');
        }

        #TextContent p:last-of-type {
            font-family: "read" !important;
        }
    </style>
</head>

<body class="bg6" id="readbg" onselectstart="return false">
    <div class="mlfy_main">
        <div id="mlfy_main_text">
            <h1>0～15（2/5）</h1>
            <div id="TextContent" class="read-content">
                <p>遭，孙脏唬黄，桶准校匀媚蘑砂？盒摇八劝。</p>
                <div class="dag"> what </div>
            </div>
        </div>
    </div>
</body>

</html>
`

	reader := strings.NewReader(sample)
	document, err := goquery.NewDocumentFromReader(reader)
	if err != nil {
		t.Fatalf("failed to parse sample document: %s", err)
	}

	pageNode := document.Find("div.mlfy_main")
	desktopFontDecypher(pageNode)

	_, err = pageNode.Html()
	if err != nil {
		t.Fatalf("failed to get html content: %s", err)
	}

	// t.Log(html)
}
