package storage

import (
	"encoding/json"
	"os"
	"strings"
	"sync"

	"api/api/internal/config"
	"api/api/internal/model"
)

var fileMutex sync.Mutex

func LoadFile(f string, v interface{}) error {
	b, err := os.ReadFile(f)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func FilterMetadata(allMeta []model.BlobMetadata, height uint64, nsHex string) []model.BlobMetadata {
	var results []model.BlobMetadata
	cleanQueryNS := strings.TrimLeft(nsHex, "0")
	for _, item := range allMeta {
		cleanFileNS := strings.TrimLeft(item.Namespace, "0")
		if item.Height == height && strings.EqualFold(cleanFileNS, cleanQueryNS) {
			results = append(results, item)
		}
	}
	return results
}

func SaveDataToFiles(newBlobs []model.BlobMetadata) error {
	fileMutex.Lock()
	defer fileMutex.Unlock()

	var existingMeta []model.BlobMetadata
	if _, err := os.Stat(config.FileFullMeta); err == nil {
		content, err := os.ReadFile(config.FileFullMeta)
		if err == nil {
			_ = json.Unmarshal(content, &existingMeta)
		}
	}

	combinedMeta := append(newBlobs, existingMeta...)

	fullMetaBytes, err := json.MarshalIndent(combinedMeta, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(config.FileFullMeta, fullMetaBytes, 0644); err != nil {
		return err
	}

	var existingIndex []model.IndexData
	if _, err := os.Stat(config.FileIndex); err == nil {
		content, err := os.ReadFile(config.FileIndex)
		if err == nil {
			_ = json.Unmarshal(content, &existingIndex)
		}
	}

	var newIndices []model.IndexData
	for _, b := range newBlobs {
		newIndices = append(newIndices, model.IndexData{
			Height:    b.Height,
			Namespace: b.Namespace,
			Timestamp: b.Timestamp,
		})
	}

	combinedIndex := append(newIndices, existingIndex...)

	indexBytes, err := json.MarshalIndent(combinedIndex, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(config.FileIndex, indexBytes, 0644); err != nil {
		return err
	}

	return nil
}
