package network

import (
	"errors"

	"github.com/SirZenith/delite/common"
	"github.com/charmbracelet/log"
	"github.com/gocolly/colly/v2"
)

var ErrMaxRetry = errors.New("max retry")

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

func MakeSaveImageBodyCallback(outputName string, outputFormat string) colly.ResponseCallback {
	return colly.ResponseCallback(func(resp *colly.Response) {
		err := common.SaveImageAs(resp.Body, outputName, outputFormat)
		if err == nil {
			log.Infof("image downloaded: %s", outputName)
		} else {
			log.Warnf("failed to save image %s: %s\n", outputName, err)
		}
	})
}

// RetryRequest reads `retryCnt` and `maxRetryCnt` from request context. If
// current retry count is less than max retry count, this function retries given
// request, else a `ErrMaxRetry` will be retruned.
// This function returns retry count after operation, and error happenes during
// operation.
func RetryRequest(req *colly.Request) (int, error) {
	ctx := req.Ctx

	maxRetryCnt, _ := ctx.GetAny("maxRetryCnt").(int)

	retryCnt, _ := ctx.GetAny("retryCnt").(int)
	if retryCnt >= maxRetryCnt {
		return retryCnt, ErrMaxRetry
	}

	retryCnt++
	ctx.Put("retryCnt", retryCnt+1)

	return retryCnt, req.Retry()
}
