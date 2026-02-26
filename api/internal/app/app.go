package app

import (
	"fmt"

	"api/api/internal/config"
	"api/api/internal/handler"
	"api/api/internal/router"
	"api/api/internal/service"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func Run() {
	if config.BridgeToken == "" {
		fmt.Println("⚠️  CẢNH BÁO: Chưa cấu hình biến môi trường BRIDGE_TOKEN")
	}

	if err := service.InitVerifier(); err != nil {
		fmt.Printf("❌ Verify Client Error: %v\n", err)
	} else {
		fmt.Printf("✅ Verify Client OK: %s\n", config.ConsensusRPCURL)
	}

	r := gin.Default()
	r.MaxMultipartMemory = int64(config.MaxMultipartBytes)
	r.Use(cors.Default())
	r.Static("/files", "./")

	h := handler.New()
	router.SetupRoutes(r, h)

	fmt.Printf("🚀 API Server (Bridge Node Mode) đang chạy tại %s\n", config.ServerPort)
	_ = r.Run(config.ServerPort)
}
