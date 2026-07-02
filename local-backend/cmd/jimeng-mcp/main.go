package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/tangying-ai/tangying-ai-operation-system/local-backend/internal/jimengmcp"
)

func main() {
	addr := flag.String("addr", envOrDefault("JIMENG_MCP_ADDR", "127.0.0.1:18180"), "JiMeng MCP listen address")
	flag.Parse()

	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("listen %s: %v", *addr, err)
	}
	log.Printf("JiMeng MCP listening on http://%s", listener.Addr().String())
	if err := http.Serve(listener, jimengmcp.NewServer(jimengmcp.NewAdapter(jimengmcp.AdapterConfig{}))); err != nil {
		log.Fatalf("serve JiMeng MCP: %v", err)
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
