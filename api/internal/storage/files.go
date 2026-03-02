package storage

import (
	"bufio"
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

func FindBatchInfoByHeight(height uint64) (model.HeightMapRecord, model.AnchorReceipt, bool) {
	var hRec model.HeightMapRecord
	var rRec model.AnchorReceipt
	var foundHeight bool

	// ==========================================
	// BƯỚC 1: Quét file height-epoch-map.jsonl
	// ==========================================
	f1, err := os.Open(config.FileHeightMap)
	if err != nil {
		return hRec, rRec, false
	}
	defer f1.Close()

	scanner1 := bufio.NewScanner(f1)
	for scanner1.Scan() {
		var tempRec model.HeightMapRecord
		if err := json.Unmarshal(scanner1.Bytes(), &tempRec); err == nil {
			if tempRec.Height == int(height) {
				hRec = tempRec
				foundHeight = true
				break
			}
		}
	}

	if err := scanner1.Err(); err != nil || !foundHeight {
		return hRec, rRec, false
	}

	// ==========================================
	// BƯỚC 2: Quét file babylon-anchor-receipts.jsonl
	// ==========================================
	var foundReceipt bool
	f2, err := os.Open(config.FileAnchorReceipts)
	if err != nil {
		return hRec, rRec, false
	}
	defer f2.Close()

	scanner2 := bufio.NewScanner(f2)
	for scanner2.Scan() {
		var tempRec model.AnchorReceipt
		if err := json.Unmarshal(scanner2.Bytes(), &tempRec); err == nil {
			if tempRec.Batch == hRec.Batch {
				rRec = tempRec
				foundReceipt = true
				break
			}
		}
	}

	return hRec, rRec, foundReceipt
}
