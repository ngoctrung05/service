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
	"sync"
	"time"

	"api/api/internal/config"
	"api/api/internal/model"

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
