package merge

const defaultAssetDirName = "assets"

const (
	outputFormatHTML  = "html"
	outputFormatLatex = "latex"
)

// ----------------------------------------------------------------------------
// Default template

const defaultHTMLTemplate = `
<html>
<head>
    <meta charset="UTF-8">
    <meta http-equiv="X-UA-Compatible" content="IE=edge">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Document</title>
</head>
<body>
</body>
</html>
`
const defaultLatexTemplte = `
\documentclass{ltjtbook}

\usepackage{
	afterpage,
    geometry,
    graphicx,
    hyperref,
    pdfpages,
    pxrubrica,
    url,
}

\rubysetup{g}

\geometry{
	paperwidth = 12cm,
	paperheight = 16cm,
    top = 1.5cm,
    bottom = 1.5cm,
    left = 0.5cm,
    right = 0.5cm,
}
`
