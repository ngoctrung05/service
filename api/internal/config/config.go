package config

import (
	"os"
	"strconv"
)

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	raw := getEnv(key, "")
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return parsed
}

var (
	FileIndex         = getEnv("FILE_INDEX", "")
	FileFullMeta      = getEnv("FILE_FULL_META", "")
	BridgeRPCURL      = getEnv("BRIDGE_RPC_URL", "")
	ConsensusRPCURL   = getEnv("CONSENSUS_RPC_URL", "")
	DatabaseAPI       = getEnv("DATABASE_API", "")
	ServerPort        = getEnv("SERVER_PORT", "")
	MaxChunkSize      = getEnvInt("MAX_CHUNK_SIZE", 7835388)
	MaxConcurrency    = getEnvInt("MAX_CONCURRENCY", 12)
	MaxMultipartBytes = getEnvInt("MAX_MULTIPART_BYTES", 128<<20)
	BridgeToken       = getEnv("BRIDGE_TOKEN", "")
)
