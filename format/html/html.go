package html

import (
	"image"
	"path"
	"strconv"
	"strings"

	"github.com/SirZenith/delite/common"
	"github.com/SirZenith/delite/common/html_util"
	format_common "github.com/SirZenith/delite/format/common"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func replaceImageReference(attr *html.Attribute, contextFile, assetOutDir, outputExt string) (string, string) {
	packDir := path.Dir(contextFile)
	fullPath := path.Join(packDir, attr.Val)
	basename := path.Base(attr.Val)
	if outputExt != "" {
		basename = common.ReplaceFileExt(basename, outputExt)
	}

	attr.Val = path.Join(assetOutDir, basename)
	return fullPath, attr.Val
}

func ImageReferenceRedirect(node *html.Node, contextFile, assetOutDir, outputExt string, outNameMap map[string]string) string {
	childContextFile := contextFile
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		childContextFile = ImageReferenceRedirect(child, childContextFile, assetOutDir, outputExt, outNameMap)
		if childContextFile == "" {
			childContextFile = contextFile
		}
	}

	switch node.Type {
	case html.CommentNode:
		switch {
		case strings.HasPrefix(node.Data, format_common.MetaCommentFileStart):
			contextFile = node.Data[len(format_common.MetaCommentFileStart):]
		case strings.HasPrefix(node.Data, format_common.MetaCommentFileEnd):
			contextFile = ""
		}
	case html.ElementNode:
		switch node.DataAtom {
		case atom.Img:
			attr := html_util.GetNodeAttr(node, "src")
			if attr != nil {
				srcPath, dstPath := replaceImageReference(attr, contextFile, assetOutDir, outputExt)
				outNameMap[srcPath] = dstPath
			}
		case atom.Image:
			attr := html_util.GetNodeAttr(node, "href")
			if attr != nil {
				srcPath, dstPath := replaceImageReference(attr, contextFile, assetOutDir, outputExt)
				outNameMap[srcPath] = dstPath
			}
		}
	}

	return contextFile
}

func setImageSizeAttr(node *html.Node, sizeMap map[string]*image.Point, srcPath string) {
	if size := sizeMap[srcPath]; size != nil {
		node.Attr = append(node.Attr, html.Attribute{
			Key: format_common.MetaAttrWidth,
			Val: strconv.Itoa(size.X),
		})
		node.Attr = append(node.Attr, html.Attribute{
			Key: format_common.MetaAttrHeight,
			Val: strconv.Itoa(size.Y),
		})
	}
}

func SetImageSizeMeta(node *html.Node, sizeMap map[string]*image.Point) {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		SetImageSizeMeta(child, sizeMap)
	}

	if node.Type != html.ElementNode {
		return
	}

	switch node.DataAtom {
	case atom.Img:
		if attr := html_util.GetNodeAttr(node, "src"); attr != nil {
			setImageSizeAttr(node, sizeMap, attr.Val)
		}
	case atom.Image:
		if attr := html_util.GetNodeAttr(node, "href"); attr != nil {
			setImageSizeAttr(node, sizeMap, attr.Val)
		}
	}
}

func setImageTypeAttr(node *html.Node, imgType string) {
	node.Attr = append(node.Attr, html.Attribute{
		Key: format_common.MetaAttrImageType,
		Val: imgType,
	})
}

func getImageTypeBySize(widthStr string, heightStr string) string {
	imgType := format_common.ImageTypeUnknown

	width, err := strconv.Atoi(widthStr)
	if err != nil {
		return imgType
	}

	height, err := strconv.Atoi(heightStr)
	if err != nil {
		return imgType
	}

	imgType = format_common.ImageTypeInline
	if width < format_common.StandAloneImageMinSize || height < format_common.StandAloneImageMinSize {
		return imgType
	}

	ratio := float64(width) / float64(height)
	if format_common.StandAloneMinWHRatio < ratio && ratio < format_common.StandAloneMaxWHRatio {
		imgType = format_common.ImageTypeStandalone
	} else if ratio <= format_common.WHRatioTooSmall {
		imgType = format_common.ImageTypeHeightOverflow
	} else if ratio >= format_common.WHRatioTooLarge {
		imgType = format_common.ImageTypeWidthOverflow
	}

	return imgType
}

func SetImageTypeMeta(node *html.Node) {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		SetImageTypeMeta(child)
	}

	if node.Type != html.ElementNode {
		return
	}

	if node.DataAtom != atom.Image && node.DataAtom != atom.Img {
		return
	}

	widthStr, _ := html_util.GetNodeAttrVal(node, format_common.MetaAttrWidth, "")
	heightStr, _ := html_util.GetNodeAttrVal(node, format_common.MetaAttrHeight, "")

	imgType := getImageTypeBySize(widthStr, heightStr)
	setImageTypeAttr(node, imgType)
}
