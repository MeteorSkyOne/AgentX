package config

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
)

type Config struct {
	Addr       string
	DataDir    string
	SQLitePath string
	AdminToken string
}

func FromEnv() Config {
	dataDir := getenv("AGENTX_DATA_DIR", ".agentx")
	return Config{
		Addr:       getenv("AGENTX_ADDR", "127.0.0.1:8080"),
		DataDir:    dataDir,
		SQLitePath: getenv("AGENTX_SQLITE_PATH", filepath.Join(dataDir, "agentx.db")),
		AdminToken: getenv("AGENTX_ADMIN_TOKEN", randomToken()),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func randomToken() string {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("generate admin token: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}
