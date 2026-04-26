package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/config"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/context/handler"
	contextSvc "github.com/lingxi-ai/lingxi-ai-operation-system/internal/context/service"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/database"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/eventbus"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/logger"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/model/repository"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/orchestrator/service"
	orchestratorHandler "github.com/lingxi-ai/lingxi-ai-operation-system/internal/orchestrator/handler"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/outbox"
	publishHandler "github.com/lingxi-ai/lingxi-ai-operation-system/internal/publish/handler"
	publishSvc "github.com/lingxi-ai/lingxi-ai-operation-system/internal/publish/service"
	redisClient "github.com/lingxi-ai/lingxi-ai-operation-system/internal/redis"
	translatorHandler "github.com/lingxi-ai/lingxi-ai-operation-system/internal/translator/handler"
	translatorSvc "github.com/lingxi-ai/lingxi-ai-operation-system/internal/translator/service"
	workerService "github.com/lingxi-ai/lingxi-ai-operation-system/internal/worker/service"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/worker/tool"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/worker/tool/builtin"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/worker/executor"
)

func main() {
	mode := "development"
	if os.Getenv("GIN_MODE") == "release" {
		mode = "production"
		gin.SetMode(gin.ReleaseMode)
	}

	logger.Init(mode)
	cfg := config.Load()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Infrastructure
	pool := database.NewPool(ctx, cfg.Postgres)
	defer pool.Close()

	database.RunMigrations(ctx, pool)

	rdb := redisClient.NewClient(cfg.Redis)
	defer rdb.Close()

	producer := eventbus.NewProducer(cfg.Kafka)
	defer producer.Close()

	// Repositories
	taskRepo := repository.NewTaskRepository(pool)
	nodeRepo := repository.NewNodeRepository(pool)
	depRepo := repository.NewNodeDependencyRepository(pool)
	contextRepo := repository.NewContextRepository(pool)

	// Services
	eventSaver := outbox.NewOutboxSaver(pool)
	stateService := service.NewStateService(nodeRepo, taskRepo, depRepo, contextRepo, eventSaver)
	orchestratorService := service.NewOrchestratorService(taskRepo, nodeRepo, depRepo, contextRepo, stateService)
	stateMachine := service.NewStateMachine(stateService, nodeRepo, taskRepo, eventSaver)
	dependencyChecker := service.NewDependencyChecker(nodeRepo, stateService, eventSaver)
	taskExecutionCtrl := service.NewTaskExecutionControl(taskRepo, nodeRepo, stateService)
	scheduler := service.NewScheduler(nodeRepo, stateService, producer)
	contextService := contextSvc.NewContextService(contextRepo, nodeRepo, taskRepo)

	// Wire DependencyChecker into StateService for event-driven scheduling
	stateService.SetDependencyChecker(dependencyChecker)

	// Outbox relay
	outboxRelay := outbox.NewRelay(pool, producer)
	outboxRelay.Start(ctx)
	defer outboxRelay.Stop()

	// Worker
	toolRegistry := tool.NewToolRegistry()
	toolRegistry.Register(builtin.NewBashTool(cfg.BashTool))
	toolRegistry.Register(builtin.NewLlmApiTool(cfg.OpenAI))
	toolRegistry.Register(builtin.NewPythonTool())
	toolRegistry.Register(builtin.NewPolisherTool(cfg.OpenAI))

	directExec := executor.NewDirectExecutor()
	var sandboxExec *executor.SandboxExecutor
	if cfg.Sandbox.Enabled {
		var err error
		sandboxExec, err = executor.NewSandboxExecutor(cfg.Sandbox.Address)
		if err != nil {
			zap.L().Warn("sandbox client init failed, will fallback", zap.Error(err))
		}
	}

	nodeExecutor := workerService.NewNodeExecutor(toolRegistry, producer, cfg.Worker, directExec, sandboxExec)

	// Translator
	nlService := translatorSvc.NewNlToDagService(cfg.OpenAI, cfg.Services.OrchestratorURL)

	// Publish
	publishService := publishSvc.NewPublishService(cfg.OpenAI, cfg.Services.OrchestratorURL)

	// Kafka consumers
	workerConsumer := eventbus.NewConsumer(cfg.Kafka, "ai-worker-group",
		[]string{eventbus.TopicNodeReady},
		func(event eventbus.Event) error {
			go nodeExecutor.ExecuteNode(ctx, event)
			return nil
		},
	)
	workerConsumer.Start()
	defer workerConsumer.Stop()

	orchestratorConsumer := eventbus.NewConsumer(cfg.Kafka, "orchestrator-group",
		[]string{eventbus.TopicNodeResult},
		func(event eventbus.Event) error {
			switch event.Status {
			case "SUCCESS":
				if err := stateMachine.OnSuccess(ctx, event.NodeID, event.Output); err != nil {
					zap.L().Error("Failed to handle node success", zap.Error(err))
				}
				dependencyChecker.OnNodeExecuted(ctx, event.NodeID, event.TaskID)
			case "FAILED":
				if err := stateMachine.OnFailure(ctx, event.NodeID, event.ErrorMessage); err != nil {
					zap.L().Error("Failed to handle node failure", zap.Error(err))
				}
			}
			return nil
		},
	)
	orchestratorConsumer.Start()
	defer orchestratorConsumer.Stop()

	contextConsumer := eventbus.NewConsumer(cfg.Kafka, "ai-context-group",
			[]string{eventbus.TopicNodeResult, eventbus.TopicNodeFailed},
		func(event eventbus.Event) error {
			return contextService.HandleEvent(ctx, event)
		},
	)
	contextConsumer.Start()
	defer contextConsumer.Stop()

	// Start scheduler (30s fallback)
	scheduler.Start(ctx)
	defer scheduler.Stop()

	// HTTP server
	r := gin.Default()

	// CORS middleware
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	orchestratorHandler.NewOrchestratorHandler(orchestratorService, stateMachine, taskExecutionCtrl, contextService).RegisterRoutes(r)
	translatorHandler.NewTranslatorHandler(nlService).RegisterRoutes(r)
	handler.NewContextHandler(contextService).RegisterRoutes(r)
	publishHandler.NewPublishHandler(publishService).RegisterRoutes(r)
	publishHandler.NewTraceHandler(orchestratorService, contextService).RegisterRoutes(r)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: r,
	}

	go func() {
		zap.L().Info("Server starting", zap.Int("port", cfg.Server.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			zap.L().Fatal("Server failed to start", zap.Error(err))
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	zap.L().Info("Shutting down server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		zap.L().Fatal("Server forced to shutdown", zap.Error(err))
	}

	zap.L().Info("Server exited")
}
