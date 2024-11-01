package merge

const containerDocumentPath = "META-INF/container.xml"
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
    geometry,
    graphicx,
    hyperref,
    pdfpages,
    url,
    pxrubrica,
}

\rubysetup{g}

\geometry{
    paper = b6paper,
    top = 1.5cm,
    bottom = 1.5cm,
    left = 1.2cm,
    right = 1.2cm,
}
`

// ----------------------------------------------------------------------------
// Meta comment prefix

const metaCommentPrefix = "delite-meta."

const metaCommentFileStart = metaCommentPrefix + "file-start: "
const metaCommentFileEnd = metaCommentPrefix + "file-end: "

const metaCommentRefAnchor = metaCommentPrefix + "ref-anchor: "
