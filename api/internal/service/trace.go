package service

import (
	"api/api/internal/config"
	"api/api/internal/model"
	"api/api/internal/storage"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func BuildTrace(engramTxHash string) (*model.TraceResponse, error) {
	// Khởi tạo trước các mảng rỗng để JSON không bị trả về null
	resp := &model.TraceResponse{
		TraceID:   engramTxHash,
		InputType: "Engram_tx_hash",
		Status:    "processing",
		Diagnostics: model.Diagnostics{
			MissingSteps: []string{},
			Warnings:     []string{},
		},
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}

	// ==========================================
	// LAYER 1: Gọi API Engram lấy thông tin tx
	// ==========================================
	engramURL := fmt.Sprintf("%s/transactions/%s", config.DatabaseAPI, engramTxHash)
	fmt.Printf("🔍 Bắt đầu trace với Engram API: %s\n", engramURL)
	req1, _ := http.NewRequest("GET", engramURL, nil)
	req1.Header.Set("Accept", "*/*")

	res1, err := httpClient.Do(req1)
	if err != nil || res1.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lỗi gọi Engram API hoặc TX không tồn tại: %v", err)
	}
	defer res1.Body.Close()

	var engramData model.EngramAPIResponse
	if err := json.NewDecoder(res1.Body).Decode(&engramData); err != nil {
		return nil, fmt.Errorf("lỗi parse JSON Engram: %v", err)
	}

	step1 := model.Step{
		ID: "layer_1_Engram", Layer: 1, Title: "Included in Engram", State: "confirmed",
		Data: map[string]interface{}{
			"tx_hash":   engramData.Hash,
			"height":    engramData.Height,
			"timestamp": engramData.Timestamp,
			"signer":    engramData.Signer,
		},
		Links: map[string]interface{}{
			"tx":              fmt.Sprintf("%s/tx/%s", config.EngramURL, engramData.Hash),
			"block_by_height": fmt.Sprintf("%s/block/%d", config.EngramURL, engramData.Height),
		},
	}

	// ==========================================
	// LAYER 2: Lấy thông tin Batch
	// ==========================================
	heightRecord, receiptRecord, found := storage.FindBatchInfoByHeight(uint64(engramData.Height))

	if !found {
		resp.Status = "included_in_engram"
		resp.Diagnostics.MissingSteps = []string{"layer_2_batch", "layer_3_babylon", "layer_4_bitcoin"}

		step2 := createPendingStep("layer_2_batch", 2, "Aggregated into Merkle Batch")
		step3 := createPendingStep("layer_3_babylon", 3, "Anchored on Babylon")
		step4 := createPendingStep("layer_4_bitcoin", 4, "Finalized on Bitcoin")

		resp.Steps = []model.Step{step1, step2, step3, step4}
		resp.Summary = model.Summary{Engram: step1.Data, Batch: step2.Data, Babylon: step3.Data, Bitcoin: step4.Data}
		return resp, nil
	}

	batchID := heightRecord.Batch
	babylonTxHash := receiptRecord.TxHash

	leaves, _ := storage.GetAllHashesInBatch(batchID)
	leafArrayIndex := heightRecord.IndexInBatch - 1
	merklePath := GenerateMerkleProof(leaves, leafArrayIndex)

	step2 := model.Step{
		ID: "layer_2_batch", Layer: 2, Title: "Aggregated into Merkle Batch", State: "confirmed",
		Data: map[string]interface{}{
			"batch_type":   "merkle_sum_tree",
			"start_height": receiptRecord.StartHeight,
			"end_height":   receiptRecord.EndHeight,
			"leaves_count": receiptRecord.EndHeight - receiptRecord.StartHeight + 1,
			"leaf_index":   heightRecord.IndexInBatch,
			"merkle_root":  receiptRecord.MerkleRoot,
			"inclusion_proof": map[string]interface{}{
				"type": "merkle_path",
				"path": merklePath,
				"leaf": heightRecord.Hash,
			},
		},
		// Links: map[string]interface{}{
		// 	"batch_details": fmt.Sprintf("%s/batch/%d", config.EngramURL, batchID),
		// 	"batch_proof":   fmt.Sprintf("%s/batch/%d/proof/%d", config.EngramURL, batchID, heightRecord.IndexInBatch),
		// },
	}

	// ==========================================
	// LAYER 3: Gọi API Babylon lấy block height
	// ==========================================
	babylonURL := fmt.Sprintf("%s/cosmos/tx/v1beta1/txs/%s", config.BabylonAPIURL, babylonTxHash)
	res3, err := httpClient.Get(babylonURL)

	if err != nil || res3.StatusCode != http.StatusOK {
		resp.Status = "aggregated_into_batch"
		resp.Diagnostics.MissingSteps = []string{"layer_3_babylon", "layer_4_bitcoin"}

		step3 := createPendingStep("layer_3_babylon", 3, "Anchored on Babylon")
		step4 := createPendingStep("layer_4_bitcoin", 4, "Finalized on Bitcoin")

		if err != nil {
			resp.Diagnostics.Warnings = append(resp.Diagnostics.Warnings, "Babylon API Error: "+err.Error())
		}

		resp.Steps = []model.Step{step1, step2, step3, step4}
		resp.Summary = model.Summary{Engram: step1.Data, Batch: step2.Data, Babylon: step3.Data, Bitcoin: step4.Data}
		return resp, nil
	}
	defer res3.Body.Close()

	var bblTxData model.BabylonTxResponse
	if err := json.NewDecoder(res3.Body).Decode(&bblTxData); err != nil {
		return nil, fmt.Errorf("lỗi parse JSON Babylon TX: %v", err)
	}

	bblHeightInt, _ := strconv.Atoi(bblTxData.TxResponse.Height)
	babylonEpoch := (bblHeightInt-1)/360 + 1

	step3 := model.Step{
		ID: "layer_3_babylon", Layer: 3, Title: "Anchored on Babylon", State: "confirmed",
		Data: map[string]interface{}{
			"tx_hash":      bblTxData.TxResponse.TxHash,
			"height":       bblHeightInt,
			"epoch":        babylonEpoch,
			"timestamp":    bblTxData.TxResponse.Timestamp,
			"memo_payload": receiptRecord.Payload,
		},
		Links: map[string]interface{}{
			"tx":              fmt.Sprintf("%s/tx/%s", config.BabylonExplorerURL, bblTxData.TxResponse.TxHash),
			"block_by_height": fmt.Sprintf("%s/block/%d", config.BabylonExplorerURL, bblHeightInt),
		},
	}

	// ==========================================
	// LAYER 4: Gọi API Babylon Checkpoint lấy BTC Info
	// ==========================================
	checkpointURL := fmt.Sprintf("%s/babylon/btccheckpoint/v1/%d", config.BabylonAPIURL, babylonEpoch)
	res4, err := httpClient.Get(checkpointURL)

	var step4 model.Step
	if err == nil && res4.StatusCode == http.StatusOK {
		defer res4.Body.Close()
		var cpData model.BabylonCheckpointResponse
		json.NewDecoder(res4.Body).Decode(&cpData)

		btcBlockHash := cpData.Info.BtcBlockHash
		btcHeight := cpData.Info.BtcBlockHeight

		var btcTxIDs []string
		var btcTxLinks []string

		if len(cpData.Info.Transactions) > 0 {
			for _, tx := range cpData.Info.Transactions {
				mempoolURL := fmt.Sprintf("%s/block/%s/txid/%d", config.MempoolAPIURL, btcBlockHash, tx.Index)
				if mpRes, mpErr := httpClient.Get(mempoolURL); mpErr == nil && mpRes.StatusCode == http.StatusOK {
					bodyBytes, _ := io.ReadAll(mpRes.Body)
					txid := string(bodyBytes)

					btcTxIDs = append(btcTxIDs, txid)
					btcTxLinks = append(btcTxLinks, fmt.Sprintf("%s/tx/%s", config.BitcoinExplorerURL, txid))
					mpRes.Body.Close()
				}
			}
		}

		step4 = model.Step{
			ID: "layer_4_bitcoin", Layer: 4, Title: "Finalized on Bitcoin", State: "confirmed",
			Data: map[string]interface{}{
				"tx_hashes":               btcTxIDs,
				"height":                  btcHeight,
				"babylon_epoch_finalized": babylonEpoch,
			},
			Links: map[string]interface{}{
				"txs":             btcTxLinks,
				"block_by_height": fmt.Sprintf("%s/block/%d", config.BitcoinExplorerURL, btcHeight),
			},
		}
		resp.Status = "anchored_on_bitcoin"
	} else {
		step4 = createPendingStep("layer_4_bitcoin", 4, "Finalized on Bitcoin")
		resp.Status = "anchored_on_babylon"
		resp.Diagnostics.MissingSteps = []string{"layer_4_bitcoin"}
	}

	// ==========================================
	// TỔNG HỢP VÀ TRẢ VỀ KẾT QUẢ (NẾU FULL 4 LAYER)
	// ==========================================
	resp.Steps = []model.Step{step1, step2, step3, step4}
	resp.Summary = model.Summary{
		Engram:  step1.Data,
		Batch:   step2.Data,
		Babylon: step3.Data,
		Bitcoin: step4.Data,
	}

	return resp, nil
}

func createPendingStep(id string, layer int, title string) model.Step {
	return model.Step{
		ID:    id,
		Layer: layer,
		Title: title,
		State: "pending",
		Data:  make(map[string]interface{}),
		Links: make(map[string]interface{}),
	}
}

func GenerateMerkleProof(leaves []string, index int) []string {
	if len(leaves) == 0 || index < 0 || index >= len(leaves) {
		return []string{}
	}

	var path []string
	var nodes [][]byte

	// 1. Cấu hình lá: Băm chuỗi văn bản in hoa (Hash_StringUpper)
	for _, l := range leaves {
		upperLeaf := strings.ToUpper(l)
		h := sha256.Sum256([]byte(upperLeaf))
		nodes = append(nodes, h[:])
	}

	currIndex := index

	for len(nodes) > 1 {
		var nextLevel [][]byte
		for i := 0; i < len(nodes); i += 2 {
			left := nodes[i]

			if i+1 < len(nodes) {
				right := nodes[i+1]

				if currIndex == i {
					path = append(path, hex.EncodeToString(right))
				} else if currIndex == i+1 {
					path = append(path, hex.EncodeToString(left))
				}

				combined := append(left, right...)
				hash := sha256.Sum256(combined)
				nextLevel = append(nextLevel, hash[:])
			} else {
				nextLevel = append(nextLevel, left)
			}
		}
		currIndex /= 2
		nodes = nextLevel
	}

	return path
}
