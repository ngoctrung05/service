package router

import (
	"api/api/internal/handler"

	"github.com/gin-gonic/gin"
)

func SetupRoutes(r *gin.Engine, h *handler.Handler) {
	r.GET("/last-block-metadata", h.GetLastBlockMetadata)
	r.GET("/block-metadata", h.GetBlockByHeightHandler)
	r.GET("/get-height-namespace", h.GetHeightNamespaceHandler)
	r.GET("/retrieve-meta", h.RetrieveMetaHandler)
	r.GET("/get-blob-data", h.GetBlobDataHandler)
	r.GET("/trace", h.GetTraceHandler)

	r.POST("/submit-file", h.SubmitLargeDataHandler)
	r.POST("/submit", h.SubmitDataHandler)
}
