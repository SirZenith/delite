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
	expecting := "是说，连下雨天都陪着妹妹，说不定我其实是个很了不起的好哥哥？我老王卖瓜地想着。"

	reader := strings.NewReader(sample)
	document, err := goquery.NewDocumentFromReader(reader)
	if err != nil {
		t.Fatalf("failed to parse sample document: %s", err)
	}

	translate := desktopGetFontDescrambleMap()
	desktopFontDecypher(document.Selection, translate)

	node := document.Find("div#TextContent p").Last()
	text := node.Text()
	if text != expecting {
		t.Errorf("output:\n\t%q\nwant:\n\t%q", text, expecting)
	}
}
