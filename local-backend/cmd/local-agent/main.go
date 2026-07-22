package main

import (
	"context"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

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
	hfServiceURL := flag.String("hf-service-url", envOrDefault("TANGYING_HYPERFRAMES_SERVICE_URL", "http://127.0.0.1:8787"), "HyperFrames Render Service base URL")
	renderTimeoutSec := flag.Int("render-timeout-sec", envOrDefaultInt("TANGYING_RENDER_TIMEOUT_SEC", 1800), "HyperFrames render timeout in seconds")
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

		if err := localtool.RegisterDefaultExecutors(registry, localtool.ExecutorConfig{
			DataDir:               server.Paths().DataDir,
			HyperFramesServiceURL: *hfServiceURL,
			RenderTimeoutSec:      *renderTimeoutSec,
			MCPProviderLoader:     server.ReadMCPProviders,
		}); err != nil {
			log.Fatalf("register local tool executors: %v", err)
		}
		runnerClient := localrunner.NewClient(localrunner.Config{
			CloudAPIBase: *cloudAPIBase,
			UserToken:    *userToken,
			DeviceID:     *deviceID,
		})
		runnerLoop := localrunner.NewLoop(runnerClient, registry, localrunner.LoopOptions{
			DeviceID:             *deviceID,
			RunnerVersion:        "1.0.0",
			WorkspaceRoot:        "local://aios/projects",
			DataDir:              server.Paths().DataDir,
			MCPToolCatalogSource: localrunner.NewMCPToolCatalogDiscoverer(localrunner.MCPProviderLoader(server.ReadMCPProviders)),
		})
		go func() {
			if err := runnerLoop.Run(ctx); err != nil && err != context.Canceled {
				log.Printf("local runner loop stopped: %v", err)
			}
			// Flush pending reports on clean shutdown.
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := runnerLoop.Shutdown(shutdownCtx); err != nil {
				log.Printf("runner shutdown flush failed: %v", err)
			}
		}()
		log.Printf("Tangying local runner enabled for cloud %s", *cloudAPIBase)
	} else {
		log.Printf("Tangying local runner disabled; set TANGYING_CLOUD_API_BASE, TANGYING_USER_TOKEN, and TANGYING_DEVICE_ID to enable")
	}

	log.Printf("Tangying local agent listening on http://%s", listener.Addr().String())
	httpServer := &http.Server{Handler: server.Handler()}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("local agent shutdown failed: %v", err)
			_ = listener.Close()
		}
	}()
	if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Fatalf("serve local agent: %v", err)
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envOrDefaultInt(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		if n, err := strconv.Atoi(value); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}
