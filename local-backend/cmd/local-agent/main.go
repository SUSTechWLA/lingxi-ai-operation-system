package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/tangying-ai/tangying-ai-operation-system/local-backend/internal/localagent"
)

func main() {
	addr := flag.String("addr", envOrDefault("TANGYING_LOCAL_AGENT_ADDR", "127.0.0.1:18080"), "local agent listen address")
	dataDir := flag.String("data-dir", os.Getenv("TANGYING_LOCAL_DATA_DIR"), "local data directory")
	cloudAPIBase := flag.String("cloud-api-base", os.Getenv("TANGYING_CLOUD_API_BASE"), "cloud API base URL")
	flag.Parse()

	server := localagent.NewServer(localagent.Config{DataDir: *dataDir, CloudAPIBase: *cloudAPIBase})
	if err := server.EnsureDirs(); err != nil {
		log.Fatalf("init local directories: %v", err)
	}

	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("listen %s: %v", *addr, err)
	}
	log.Printf("Tangying local agent listening on http://%s", listener.Addr().String())
	if err := http.Serve(listener, server.Handler()); err != nil {
		log.Fatalf("serve local agent: %v", err)
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
