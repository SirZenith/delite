package network

import (
	"github.com/charmbracelet/log"
	"github.com/gocolly/colly/v2"
)

// MakeSaveBodyCallback returns a closure that saves response body to given path
// and can be used as colly onResponse callback.
func MakeSaveBodyCallback(outputName string) colly.ResponseCallback {
	return colly.ResponseCallback(func(resp *colly.Response) {
		if err := resp.Save(outputName); err == nil {
			log.Infof("file downloaded: %s", outputName)
		} else {
			log.Warnf("failed to save file %s: %s\n", outputName, err)
		}
	})
}
