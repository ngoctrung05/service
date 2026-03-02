package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"api/api/internal/config"
	"api/api/internal/model"
	"api/api/internal/storage"

	"github.com/celestiaorg/celestia-app/v6/app"
	"github.com/celestiaorg/celestia-app/v6/app/encoding"
	blobtypes "github.com/celestiaorg/celestia-app/v6/x/blob/types"
	rpcclient "github.com/cometbft/cometbft/rpc/client/http"
	sdk "github.com/cosmos/cosmos-sdk/types"

	client "github.com/celestiaorg/celestia-node/api/rpc/client"
	"github.com/celestiaorg/celestia-node/blob"
	"github.com/celestiaorg/celestia-node/state"
	share "github.com/celestiaorg/go-square/v3/share"
)

var (
	txDecoder         sdk.TxDecoder
	rpcCheckClient    *rpcclient.HTTP
	processingHeights sync.Map
)

func InitVerifier() error {
	var err error
	rpcCheckClient, err = rpcclient.New(config.ConsensusRPCURL, "/websocket")
	if err != nil {
		return err
	}

	encCfg := encoding.MakeConfig(app.ModuleEncodingRegisters...)
	txDecoder = encCfg.TxConfig.TxDecoder()

	return nil
}

func NewCelestiaClient(ctx context.Context) (*client.Client, error) {
	return client.NewClient(ctx, config.BridgeRPCURL, config.BridgeToken)
}

func ProcessStreamAndSubmit(ctx context.Context, c *client.Client, reader io.Reader, nsHex string) ([]model.BlobMetadata, error) {
	var wg sync.WaitGroup
	var mu sync.Mutex

	var results []model.BlobMetadata
	var firstErr error
	sem := make(chan struct{}, config.MaxConcurrency)
	buffer := make([]byte, config.MaxChunkSize)
	partNum := 1

	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			chunkData := make([]byte, n)
			copy(chunkData, buffer[:n])

			wg.Add(1)
			sem <- struct{}{}

			go func(id int, data []byte) {
				defer wg.Done()
				defer func() { <-sem }()

				mu.Lock()
				if firstErr != nil {
					mu.Unlock()
					return
				}
				mu.Unlock()

				meta, err := SubmitChunk(ctx, c, id, data, nsHex)

				mu.Lock()
				if err == nil {
					results = append(results, *meta)
				} else if firstErr == nil {
					firstErr = fmt.Errorf("lỗi gửi chunk %d: %v", id, err)
				}
				mu.Unlock()
			}(partNum, chunkData)

			partNum++
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}

	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}
	return results, nil
}

func SubmitChunk(ctx context.Context, c *client.Client, partID int, data []byte, nsHex string) (*model.BlobMetadata, error) {
	nsBytes, err := hex.DecodeString(nsHex)
	if err != nil {
		return nil, err
	}
	nsp, err := share.NewV0Namespace(nsBytes)
	if err != nil {
		return nil, err
	}

	newBlob, err := blob.NewBlobV0(nsp, data)
	if err != nil {
		return nil, err
	}

	submitCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	options := state.NewTxConfig()
	height, err := c.Blob.Submit(submitCtx, []*blob.Blob{newBlob}, options)
	if err != nil {
		return nil, err
	}

	fmt.Printf("[Part %d] ✅ Height: %d\n", partID, height)

	return &model.BlobMetadata{
		Height:     height,
		Namespace:  nsHex,
		Commitment: base64.StdEncoding.EncodeToString(newBlob.Commitment),
		Signer:     "API-User",
		Size:       len(data),
		Hash:       base64.StdEncoding.EncodeToString(newBlob.Commitment),
		Timestamp:  time.Now().Format(time.RFC3339),
	}, nil
}

func CallBridgeRPC(method string, params []interface{}) (*model.BridgeRPCResponse, error) {
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", config.BridgeRPCURL, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.BridgeToken)

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var rpcResp model.BridgeRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, err
	}
	return &rpcResp, nil
}

func TriggerBlockVerification(targetHeight int64) {
	if _, loaded := processingHeights.LoadOrStore(targetHeight, true); loaded {
		return
	}

	go func() {
		defer processingHeights.Delete(targetHeight)

		fmt.Printf("🔍 Bắt đầu quét toàn bộ Block %d...\n", targetHeight)

		for attempt := 0; attempt < 5; attempt++ {
			blockInfo, err := rpcCheckClient.Block(context.Background(), &targetHeight)
			if err != nil {
				time.Sleep(1 * time.Second)
				continue
			}

			blockTimeUnix := blockInfo.Block.Header.Time.Unix()
			txs := blockInfo.Block.Data.Txs

			for i, rawTx := range txs {
				decodedTx, err := txDecoder(rawTx)
				if err != nil {
					continue
				}

				msgs := decodedTx.GetMsgs()
				blobIndexCounter := 0
				hasBlob := false

				result := model.TransactionResult{
					Height:    targetHeight,
					Timestamp: blockTimeUnix,
					Index:     uint32(i),
					Hash:      hex.EncodeToString(rawTx.Hash()),
					Blobs:     []model.BlobDetail{},
				}

				for _, msg := range msgs {
					if pfb, ok := msg.(*blobtypes.MsgPayForBlobs); ok {
						hasBlob = true

						for k := 0; k < len(pfb.BlobSizes); k++ {
							result.Blobs = append(result.Blobs, model.BlobDetail{
								Height:     targetHeight,
								Index:      blobIndexCounter,
								Namespace:  base64.StdEncoding.EncodeToString(pfb.Namespaces[k]),
								Commitment: base64.StdEncoding.EncodeToString(pfb.ShareCommitments[k]),
								Signer:     "DeFAI Sensor",
								Size:       pfb.BlobSizes[k],
								Timestamp:  blockTimeUnix,
							})
							blobIndexCounter++
						}
					}
				}

				result.Count = len(result.Blobs)

				if hasBlob && result.Count > 0 {
					SubmitToAPI(result)
				}
			}

			fmt.Printf("✅ Hoàn tất verify Block %d.\n", targetHeight)
			return
		}

		fmt.Printf("❌ Timeout: Không lấy được Block %d sau 5s.\n", targetHeight)
	}()
}

func SubmitToAPI(data model.TransactionResult) {
	jsonData, _ := json.Marshal(data)
	req, _ := http.NewRequest("POST", string(config.DatabaseAPI+"/data/submit-tx"), bytes.NewBuffer(jsonData))
	fmt.Println("🚀 Gửi dữ liệu lên API Database..." + string(config.DatabaseAPI+"/data/submit-tx"))
	req.Header.Set("Content-Type", "application/json")

	httpClient := &http.Client{Timeout: 15 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("❌ Lỗi DB API: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		fmt.Printf("💾 [DB SAVE] OK! Status: %d\n", resp.StatusCode)
	} else {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("⚠️ [DB FAIL] Status %d: %s\n", resp.StatusCode, string(body))
	}
}

func BuildTrace(engramTxHash string) (*model.TraceResponse, error) {
	resp := &model.TraceResponse{
		TraceID:   engramTxHash,
		InputType: "Engram_tx_hash",
		Status:    "processing",
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
		return nil, fmt.Errorf("lỗi gọi Engram API: %v", err)
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
	// LAYER 2: Lấy thông tin Batch (Đọc trực tiếp từ file, không lưu RAM)
	// ==========================================

	// Gọi hàm quét file on-the-fly từ tầng storage
	heightRecord, receiptRecord, found := storage.FindBatchInfoByHeight(uint64(engramData.Height))
	if !found {
		return nil, fmt.Errorf("layer 2: không tìm thấy block Engram %d trong hệ thống batch nội bộ", engramData.Height)
	}

	// _batchID := heightRecord.Batch         // Batch ở file jsonl thực chất là Batch ID
	babylonTxHash := receiptRecord.TxHash // Lấy TX Hash để ném sang Layer 3

	step2 := model.Step{
		ID: "layer_2_batch", Layer: 2, Title: "Aggregated into Merkle Batch", State: "confirmed",
		Data: map[string]interface{}{
			"batch_type":   "merkle_sum_tree",
			"start_height": receiptRecord.StartHeight,
			"end_height":   receiptRecord.EndHeight,
			"merkle_root":  receiptRecord.MerkleRoot,
			"leaf_index":   heightRecord.IndexInBatch,
		},
		Links: map[string]interface{}{
			"batch_details": " ",
			"batch_proof":   " ",
		},
	}
	// ==========================================
	// LAYER 3: Gọi API Babylon lấy block height
	// ==========================================
	babylonURL := fmt.Sprintf("%s/cosmos/tx/v1beta1/txs/%s", config.BabylonAPIURL, babylonTxHash)
	res3, err := httpClient.Get(babylonURL)
	if err != nil || res3.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lỗi gọi Babylon API: %v", err)
	}
	defer res3.Body.Close()

	var bblTxData model.BabylonTxResponse
	if err := json.NewDecoder(res3.Body).Decode(&bblTxData); err != nil {
		return nil, fmt.Errorf("lỗi parse JSON Babylon TX: %v", err)
	}

	bblHeightInt, _ := strconv.Atoi(bblTxData.TxResponse.Height)

	// Tính Babylon Epoch (Giả sử epoch_interval testnet hiện tại là 360)
	// Công thức chuẩn: Epoch = (Height - 1) / 360 + 1
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
				"tx_hashes":               btcTxIDs, // Trả về mảng 2 tx
				"height":                  btcHeight,
				"babylon_epoch_finalized": babylonEpoch,
			},
			Links: map[string]interface{}{ // Đổi kiểu map để chứa được slice
				"txs":             btcTxLinks,
				"block_by_height": fmt.Sprintf("%s/block/%d", config.BitcoinExplorerURL, btcHeight),
			},
		}

		resp.Status = "anchored_on_bitcoin"
	} else {
		step4 = model.Step{
			ID: "layer_4_bitcoin", Layer: 4, Title: "Finalized on Bitcoin", State: "pending",
			Data: map[string]interface{}{},
		}
		resp.Status = "anchored_on_babylon"
	}

	// ==========================================
	// TỔNG HỢP VÀ TRẢ VỀ KẾT QUẢ
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
