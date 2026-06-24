package main

import (
	"context"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/tangying-ai/tangying-ai-operation-system/local-backend/internal/localagent"
	"github.com/tangying-ai/tangying-ai-operation-system/local-backend/internal/localrunner"
	"github.com/tangying-ai/tangying-ai-operation-system/local-backend/internal/localtool"
)

func main() {
	addr := flag.String("addr", envOrDefault("TANGYING_LOCAL_AGENT_ADDR", "127.0.0.1:18080"), "local agent listen address")
	dataDir := flag.String("data-dir", os.Getenv("TANGYING_LOCAL_DATA_DIR"), "local data directory")
	cloudAPIBase := flag.String("cloud-api-base", os.Getenv("TANGYING_CLOUD_API_BASE"), "cloud API base URL")
	userToken := flag.String("user-token", os.Getenv("TANGYING_USER_TOKEN"), "cloud user access token for local runner")
	deviceID := flag.String("device-id", os.Getenv("TANGYING_DEVICE_ID"), "stable local device identifier")
	flag.Parse()

	server := localagent.NewServer(localagent.Config{DataDir: *dataDir, CloudAPIBase: *cloudAPIBase})
	if err := server.EnsureDirs(); err != nil {
		log.Fatalf("init local directories: %v", err)
	}

	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("listen %s: %v", *addr, err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *cloudAPIBase != "" && *userToken != "" && *deviceID != "" {
		registry := localtool.NewRegistry()
		registry.Register(localtool.NewHyperFramesProjectExecutor(server.Paths().DataDir), "HYPERFRAMES_PROJECT_GENERATE")
		runnerClient := localrunner.NewClient(localrunner.Config{
			CloudAPIBase: *cloudAPIBase,
			UserToken:    *userToken,
			DeviceID:     *deviceID,
		})
		runnerLoop := localrunner.NewLoop(runnerClient, registry, localrunner.LoopOptions{
			DeviceID:      *deviceID,
			RunnerVersion: "1.0.0",
			WorkspaceRoot: "local://aios/projects",
		})
		go func() {
			if err := runnerLoop.Run(ctx); err != nil && ctx.Err() == nil {
				log.Printf("local runner loop stopped: %v", err)
			}
		}()
		log.Printf("Tangying local runner enabled for cloud %s", *cloudAPIBase)
	} else {
		log.Printf("Tangying local runner disabled; set TANGYING_CLOUD_API_BASE, TANGYING_USER_TOKEN, and TANGYING_DEVICE_ID to enable")
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
