package html

import (
	"image"
	"path"
	"strconv"
	"strings"

	"github.com/SirZenith/delite/common/html_util"
	"github.com/SirZenith/delite/format/common"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func replaceImageReference(attr *html.Attribute, contextFile, assetOutDir string) (string, string) {
	packDir := path.Dir(contextFile)
	fullPath := path.Join(packDir, attr.Val)
	basename := path.Base(attr.Val)
	attr.Val = path.Join(assetOutDir, basename)
	return fullPath, attr.Val
}

func ImageReferenceRedirect(node *html.Node, contextFile, assetOutDir string, outNameMap map[string]string) string {
	childContextFile := contextFile
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		childContextFile = ImageReferenceRedirect(child, childContextFile, assetOutDir, outNameMap)
		if childContextFile == "" {
			childContextFile = contextFile
		}
	}

	switch node.Type {
	case html.CommentNode:
		switch {
		case strings.HasPrefix(node.Data, common.MetaCommentFileStart):
			contextFile = node.Data[len(common.MetaCommentFileStart):]
		case strings.HasPrefix(node.Data, common.MetaCommentFileEnd):
			contextFile = ""
		}
	case html.ElementNode:
		switch node.DataAtom {
		case atom.Img:
			attr := html_util.GetNodeAttr(node, "src")
			if attr != nil {
				srcPath, dstPath := replaceImageReference(attr, contextFile, assetOutDir)
				outNameMap[srcPath] = dstPath
			}
		case atom.Image:
			attr := html_util.GetNodeAttr(node, "href")
			if attr != nil {
				srcPath, dstPath := replaceImageReference(attr, contextFile, assetOutDir)
				outNameMap[srcPath] = dstPath
			}
		}
	}

	return contextFile
}

func setImageSizeAttr(node *html.Node, sizeMap map[string]*image.Point, srcPath string) {
	if size := sizeMap[srcPath]; size != nil {
		node.Attr = append(node.Attr, html.Attribute{
			Key: common.MetaAttrWidth,
			Val: strconv.Itoa(size.X),
		})
		node.Attr = append(node.Attr, html.Attribute{
			Key: common.MetaAttrHeight,
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
		Key: common.MetaAttrImageType,
		Val: imgType,
	})
}

func getImageTypeBySize(widthStr string, heightStr string) string {
	imgType := common.ImageTypeUnknown

	width, err := strconv.Atoi(widthStr)
	if err != nil {
		return imgType
	}

	height, err := strconv.Atoi(heightStr)
	if err != nil {
		return imgType
	}

	imgType = common.ImageTypeInline
	if width < common.StandAloneImageMinSize || height < common.StandAloneImageMinSize {
		return imgType
	}

	ratio := float64(width) / float64(height)
	if common.StandAloneMinWHRatio < ratio && ratio < common.StandAloneMaxWHRatio {
		imgType = common.ImageTypeStandalone
	} else if ratio <= common.WHRatioTooSmall {
		imgType = common.ImageTypeHeightOverflow
	} else if ratio >= common.WHRatioTooLarge {
		imgType = common.ImageTypeWidthOverflow
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

	widthStr := html_util.GetNodeAttrVal(node, common.MetaAttrWidth, "")
	heightStr := html_util.GetNodeAttrVal(node, common.MetaAttrHeight, "")

	imgType := getImageTypeBySize(widthStr, heightStr)
	setImageTypeAttr(node, imgType)
}
