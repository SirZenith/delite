package book_management

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/SirZenith/delite/common"
)

type Config struct {
	HttpProxy  string `json:"http_proxy"`
	HttpsProxy string `json:"https_proxy"`
	JobCount   int    `json:"job_count"`
	RetryCount int    `json:"retry"`

	OutputDir  string `json:"output_dir"`
	TargetList string `json:"list_file"`
	HeaderFile string `json:"header_file"`
}

// readConfig read configuration from JSON file. All config given from command
// line will not be overridden.
func ReadConfigFile(filePath string) (Config, error) {
	c := Config{}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return c, fmt.Errorf("failed to read config file %s: %s", filePath, err)
	}

	if err := json.Unmarshal([]byte(data), &c); err != nil {
		return c, fmt.Errorf("failed to parse config JSON %s: %s", filePath, err)
	}

	configDir := filepath.Dir(filePath)

	c.OutputDir = common.ResolveRelativePath(c.OutputDir, configDir)
	c.TargetList = common.ResolveRelativePath(c.TargetList, configDir)
	c.HeaderFile = common.ResolveRelativePath(c.HeaderFile, configDir)

	return c, nil
}
