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
