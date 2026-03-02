package model

import "time"

type BlockMetadataResponse struct {
	LatestBlockHash   string    `json:"latest_block_hash"`
	LatestBlockHeight string    `json:"latest_block_height"`
	LatestBlockTime   time.Time `json:"latest_block_time"`
}

type BridgeRPCResponse struct {
	Jsonrpc string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Result  struct {
		Header struct {
			Height string    `json:"height"`
			Time   time.Time `json:"time"`
		} `json:"header"`
		Commit struct {
			BlockID struct {
				Hash string `json:"hash"`
			} `json:"block_id"`
		} `json:"commit"`
	} `json:"result"`
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type BlobMetadata struct {
	Height     uint64 `json:"height"`
	Namespace  string `json:"namespace"`
	Commitment string `json:"commitment"`
	Signer     string `json:"signer"`
	Size       int    `json:"size"`
	Hash       string `json:"hash"`
	Timestamp  string `json:"timestamp"`
}

type BlobDataResponse struct {
	Data       string `json:"data"`
	Base64Data string `json:"base64_data"`
	Commitment string `json:"commitment"`
}

type IndexData struct {
	Height    uint64 `json:"height"`
	Namespace string `json:"namespace"`
	Timestamp string `json:"timestamp"`
}

type SubmitRequest struct {
	Namespace string `json:"namespace" binding:"required"`
	Data      string `json:"data" binding:"required"`
}

type OutputTx struct {
	Height    int64        `json:"height"`
	Timestamp int64        `json:"timestamp"`
	Index     uint32       `json:"index"`
	Hash      string       `json:"hash"`
	Count     int          `json:"count"`
	Blobs     []OutputBlob `json:"blobs"`
}

type OutputBlob struct {
	Height     int64  `json:"height"`
	Index      int    `json:"index"`
	Namespace  string `json:"namespace"`
	Commitment string `json:"commitment"`
	Signer     string `json:"signer"`
	Size       uint32 `json:"size"`
	Timestamp  int64  `json:"timestamp"`
}

type TransactionResult struct {
	Height    int64        `json:"height"`
	Timestamp int64        `json:"timestamp"`
	Index     uint32       `json:"index"`
	Hash      string       `json:"hash"`
	Count     int          `json:"count"`
	Blobs     []BlobDetail `json:"blobs"`
}

type BlobDetail struct {
	Height     int64  `json:"height"`
	Index      int    `json:"index"`
	Namespace  string `json:"namespace"`
	Commitment string `json:"commitment"`
	Signer     string `json:"signer"`
	Size       uint32 `json:"size"`
	Timestamp  int64  `json:"timestamp"`
}

type TraceResponse struct {
	TraceID     string      `json:"trace_id"`
	InputType   string      `json:"input_type"`
	Status      string      `json:"status"`
	Summary     Summary     `json:"summary"`
	Steps       []Step      `json:"steps"`
	Diagnostics Diagnostics `json:"diagnostics"`
}

type Summary struct {
	Engram  map[string]interface{} `json:"Engram"`
	Batch   map[string]interface{} `json:"batch"`
	Babylon map[string]interface{} `json:"babylon"`
	Bitcoin map[string]interface{} `json:"bitcoin"`
}

type Step struct {
	ID    string                 `json:"id"`
	Layer int                    `json:"layer"`
	Title string                 `json:"title"`
	State string                 `json:"state"`
	Data  map[string]interface{} `json:"data"`
	Links map[string]interface{} `json:"links"`
}

type Diagnostics struct {
	MissingSteps []string `json:"missing_steps"`
	Warnings     []string `json:"warnings"`
}

type HeightMapRecord struct {
	Batch        int    `json:"batch"`
	IndexInBatch int    `json:"indexInBatch"`
	Height       int    `json:"height"`
	Hash         string `json:"hash"`
}

type AnchorReceipt struct {
	Batch       int    `json:"batch"`
	TxHash      string `json:"txHash"`
	MerkleRoot  string `json:"merkleRoot"`
	Payload     string `json:"payload"`
	StartHeight int    `json:"startHeight"`
	EndHeight   int    `json:"endHeight"`
}

type EngramAPIResponse struct {
	Height    int    `json:"height"`
	Hash      string `json:"hash"`
	Signer    string `json:"signer"`
	Timestamp int64  `json:"timestamp"`
}

type BabylonTxResponse struct {
	TxResponse struct {
		Height    string `json:"height"` // Cosmos SDK trả height dạng string
		TxHash    string `json:"txhash"`
		Timestamp string `json:"timestamp"`
	} `json:"tx_response"`
}

type BabylonCheckpointResponse struct {
	Info struct {
		BtcBlockHeight int    `json:"best_submission_btc_block_height"`
		BtcBlockHash   string `json:"best_submission_btc_block_hash"`
		Transactions   []struct {
			Index int    `json:"index"`
			Hash  string `json:"hash"`
		} `json:"best_submission_transactions"`
	} `json:"info"`
}
