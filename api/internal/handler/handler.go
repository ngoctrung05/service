package handler

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"api/api/internal/config"
	"api/api/internal/model"
	"api/api/internal/service"
	"api/api/internal/storage"

	"github.com/celestiaorg/celestia-node/blob"
	"github.com/celestiaorg/celestia-node/state"
	share "github.com/celestiaorg/go-square/v3/share"
	"github.com/gin-gonic/gin"
)

type Handler struct{}

func New() *Handler {
	return &Handler{}
}

func (h *Handler) GetBlobDataHandler(c *gin.Context) {
	heightStr := c.Query("height")
	height, err := strconv.ParseUint(heightStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Height không hợp lệ"})
		return
	}

	nsHex := c.Query("namespace")
	if nsHex == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Thiếu namespace"})
		return
	}

	nsBytes, err := hex.DecodeString(nsHex)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Namespace Hex không hợp lệ"})
		return
	}

	nsp, err := share.NewV0Namespace(nsBytes)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Không thể tạo Namespace V0", "details": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	celestiaClient, err := service.NewCelestiaClient(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Lỗi kết nối RPC Celestia", "details": err.Error()})
		return
	}
	defer celestiaClient.Close()

	blobs, err := celestiaClient.Blob.GetAll(ctx, height, []share.Namespace{nsp})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Lỗi khi lấy Blob từ Node", "details": err.Error()})
		return
	}

	var results []model.BlobDataResponse
	for _, b := range blobs {
		results = append(results, model.BlobDataResponse{
			Data:       string(b.Data()),
			Base64Data: base64.StdEncoding.EncodeToString(b.Data()),
			Commitment: base64.StdEncoding.EncodeToString(b.Commitment),
		})
	}

	c.JSON(http.StatusOK, gin.H{"count": len(results), "blobs": results})
}

func (h *Handler) SubmitLargeDataHandler(c *gin.Context) {
	namespace := c.PostForm("namespace")
	if namespace == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Thiếu namespace"})
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Không tìm thấy file upload"})
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể mở file upload"})
		return
	}
	defer file.Close()

	fmt.Printf("📦 Nhận request upload file: %s (%d bytes)\n", fileHeader.Filename, fileHeader.Size)

	ctx := context.Background()
	celestiaClient, err := service.NewCelestiaClient(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Lỗi kết nối RPC Celestia", "details": err.Error()})
		return
	}
	defer celestiaClient.Close()

	start := time.Now()
	blobsMetadata, err := service.ProcessStreamAndSubmit(ctx, celestiaClient, file, namespace)
	duration := time.Since(start)

	if err != nil {
		fmt.Printf("❌ Upload thất bại: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Lỗi trong quá trình gửi dữ liệu", "details": err.Error()})
		return
	}

	sort.Slice(blobsMetadata, func(i, j int) bool { return blobsMetadata[i].Height < blobsMetadata[j].Height })

	go func(metas []model.BlobMetadata) {
		if len(metas) == 0 {
			return
		}
		var lastHeight uint64
		for _, meta := range metas {
			if meta.Height != lastHeight {
				service.TriggerBlockVerification(int64(meta.Height))
				lastHeight = meta.Height
			}
		}
	}(blobsMetadata)

	if err := storage.SaveDataToFiles(blobsMetadata); err != nil {
		fmt.Printf("⚠️ Gửi OK nhưng lỗi lưu file: %v\n", err)
	} else {
		fmt.Println("💾 Đã cập nhật public_full_meta.json và public_index.json")
	}

	var heights []uint64
	for _, b := range blobsMetadata {
		heights = append(heights, b.Height)
	}

	c.JSON(http.StatusOK, gin.H{
		"file_name":    fileHeader.Filename,
		"total_chunks": len(heights),
		"heights":      heights,
		"duration":     duration.String(),
		"namespace":    namespace,
	})
}

func (h *Handler) SubmitDataHandler(c *gin.Context) {
	var req model.SubmitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dữ liệu không hợp lệ", "details": err.Error()})
		return
	}

	nsBytes, err := hex.DecodeString(req.Namespace)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Namespace phải là chuỗi Hex hợp lệ"})
		return
	}
	nsp, err := share.NewV0Namespace(nsBytes)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Lỗi tạo Namespace", "details": err.Error()})
		return
	}

	ctx := context.Background()
	celestiaClient, err := service.NewCelestiaClient(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Lỗi kết nối RPC", "details": err.Error()})
		return
	}
	defer celestiaClient.Close()

	blobData, err := blob.NewBlobV0(nsp, []byte(req.Data))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Lỗi tạo Blob", "details": err.Error()})
		return
	}

	submitCtx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	options := state.NewTxConfig()
	height, err := celestiaClient.Blob.Submit(submitCtx, []*blob.Blob{blobData}, options)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Lỗi Submit lên Celestia", "details": err.Error()})
		return
	}

	meta := model.BlobMetadata{
		Height:     height,
		Namespace:  req.Namespace,
		Commitment: base64.StdEncoding.EncodeToString(blobData.Commitment),
		Signer:     "API-Raw-Submit",
		Size:       len(req.Data),
		Hash:       base64.StdEncoding.EncodeToString(blobData.Commitment),
		Timestamp:  time.Now().Format(time.RFC3339),
	}

	if err := storage.SaveDataToFiles([]model.BlobMetadata{meta}); err != nil {
		fmt.Printf("⚠️ Gửi OK nhưng lỗi lưu file: %v\n", err)
	} else {
		fmt.Println("💾 Đã lưu metadata từ raw submit vào file.")
	}

	go service.TriggerBlockVerification(int64(height))

	c.JSON(http.StatusOK, gin.H{
		"height":     height,
		"commitment": meta.Commitment,
		"namespace":  req.Namespace,
	})
}

func (h *Handler) GetLastBlockMetadata(c *gin.Context) {
	rpcResp, err := service.CallBridgeRPC("header.LocalHead", []interface{}{})
	if err != nil {
		c.JSON(500, gin.H{"error": "Lỗi kết nối Bridge Node", "details": err.Error()})
		return
	}
	if rpcResp.Error.Message != "" {
		c.JSON(500, gin.H{"error": rpcResp.Error.Message})
		return
	}
	c.JSON(200, model.BlockMetadataResponse{
		LatestBlockHash:   rpcResp.Result.Commit.BlockID.Hash,
		LatestBlockHeight: rpcResp.Result.Header.Height,
		LatestBlockTime:   rpcResp.Result.Header.Time,
	})
}

func (h *Handler) GetBlockByHeightHandler(c *gin.Context) {
	heightStr := c.Query("height")
	heightInt, _ := strconv.Atoi(heightStr)
	rpcResp, err := service.CallBridgeRPC("header.GetByHeight", []interface{}{heightInt})
	if err != nil {
		c.JSON(500, gin.H{"error": "Lỗi kết nối Bridge Node"})
		return
	}
	if rpcResp.Error.Message != "" {
		c.JSON(404, gin.H{"error": "Dữ liệu không tồn tại", "details": rpcResp.Error.Message})
		return
	}
	c.JSON(200, model.BlockMetadataResponse{
		LatestBlockHash:   rpcResp.Result.Commit.BlockID.Hash,
		LatestBlockHeight: rpcResp.Result.Header.Height,
		LatestBlockTime:   rpcResp.Result.Header.Time,
	})
}

func (h *Handler) GetHeightNamespaceHandler(c *gin.Context) {
	var data []model.IndexData
	if err := storage.LoadFile(config.FileIndex, &data); err != nil {
		c.JSON(200, []model.IndexData{})
		return
	}
	count, _ := strconv.Atoi(c.DefaultQuery("count", "0"))
	if count > 0 && count < len(data) {
		data = data[:count]
	}
	c.JSON(200, data)
}

func (h *Handler) RetrieveMetaHandler(c *gin.Context) {
	heightStr := c.Query("height")
	nsHex := c.Query("namespace")
	height, _ := strconv.ParseUint(heightStr, 10, 64)
	var allMeta []model.BlobMetadata
	_ = storage.LoadFile(config.FileFullMeta, &allMeta)
	results := storage.FilterMetadata(allMeta, height, nsHex)
	c.JSON(200, gin.H{"count": len(results), "blobs": results})
}

// Bạn thêm hàm này vào file handler.go hiện tại
func (h *Handler) GetTraceHandler(c *gin.Context) {
	txHash := c.Query("hash")

	if txHash == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Thiếu tham số hash giao dịch (tx_hash)"})
		return
	}

	traceData, err := service.BuildTrace(txHash)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(strings.ToLower(errStr), "not found") {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "Không tìm thấy dữ liệu truy vết",
				"details": errStr,
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Lỗi hệ thống khi truy vết giao dịch",
			"details": errStr,
		})
		return
	}

	c.JSON(http.StatusOK, traceData)
}
