package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
)

type NameMapEntry struct {
	URL   string `json:"url"`   // URL of the first page of chapter
	Title string `json:"title"` // chapter title on TOC web page
	File  string `json:"file"`  // final title title used in file name for saving downloaded content
}

type GardedNameMap struct {
	lock    sync.Mutex
	NameMap map[string]NameMapEntry
}

// Reads name map from JSON.
func (m *GardedNameMap) ReadNameMap(path string) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		} else {
			return err
		}
	}

	list := []NameMapEntry{}
	err = json.Unmarshal(data, &list)
	if err != nil {
		return fmt.Errorf("failed to parse %s: %s", path, err)
	}

	for _, entry := range list {
		m.NameMap[entry.URL] = entry
	}

	return nil
}

// Save current name map to file.
func (m *GardedNameMap) SaveNameMap(path string) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	list := []NameMapEntry{}
	for _, entry := range m.NameMap {
		list = append(list, entry)
	}

	data, err := json.MarshalIndent(list, "", "    ")
	if err != nil {
		return fmt.Errorf("failed to convert data to JSON: %s", err)
	}

	err = os.WriteFile(path, data, 0o644)
	if err != nil {
		return fmt.Errorf("faield to write name map %s: %s", path, err)
	}

	return nil
}

// Get file name of given chapter key, when title name can not be found in
// current name map, empty string will be returned.
func (m *GardedNameMap) GetMapTo(url string) string {
	m.lock.Lock()
	defer m.lock.Unlock()

	return m.NameMap[url].File
}

// Sets file name used by a chapter key.
func (m *GardedNameMap) SetMapTo(entry *NameMapEntry) {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.NameMap[entry.URL] = *entry
}
