package common

const metaCommentPrefix = "delite-meta."

const MetaCommentFileStart = metaCommentPrefix + "file-start: "
const MetaCommentFileEnd = metaCommentPrefix + "file-end: "
const MetaCommentRefAnchor = metaCommentPrefix + "ref-anchor: "
const MetaCommentRawText = metaCommentPrefix + "raw-text: "

const metaAttrPrefix = "delite-"

const MetaAttrWidth = metaAttrPrefix + "width"
const MetaAttrHeight = metaAttrPrefix + "height"
const MetaAttrImageType = metaAttrPrefix + "img-type"
const MetaRubyType = metaAttrPrefix + "ruby-type"

const (
	ImageTypeUnknown        = "unknown"
	ImageTypeInline         = "inline"      // images inserted inline as an icon
	ImageTypeStandalone     = "standalone"  // images occupying a single page as an illustration
	ImageTypeWidthOverflow  = "over-width"  // images with too large with-height ratio
	ImageTypeHeightOverflow = "over-height" // images with too small with-height ratio
)

const StandAloneImageMinSize = 300
const StandAloneMinWHRatio = 0.35
const StandAloneMaxWHRatio = 1.5

const WHRatioTooLarge = 2.25
const WHRatioTooSmall = 0.25
