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
	orchestratorHandler "github.com/lingxi-ai/lingxi-ai-operation-system/internal/orchestrator/handler"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/orchestrator/service"
	redisClient "github.com/lingxi-ai/lingxi-ai-operation-system/internal/redis"
	translatorHandler "github.com/lingxi-ai/lingxi-ai-operation-system/internal/translator/handler"
	translatorSvc "github.com/lingxi-ai/lingxi-ai-operation-system/internal/translator/service"
	workerSvc "github.com/lingxi-ai/lingxi-ai-operation-system/internal/worker/service"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/worker/tool"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/worker/tool/builtin"
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
	stateService := service.NewStateService(nodeRepo, taskRepo, depRepo, contextRepo, producer)
	orchestratorService := service.NewOrchestratorService(taskRepo, nodeRepo, depRepo, contextRepo, stateService)
	stateMachine := service.NewStateMachine(stateService, nodeRepo, taskRepo, producer)
	dependencyChecker := service.NewDependencyChecker(nodeRepo, stateService, producer)
	taskExecutionCtrl := service.NewTaskExecutionControl(taskRepo, nodeRepo, stateService)
	scheduler := service.NewScheduler(nodeRepo, stateService, producer)
	contextService := contextSvc.NewContextService(contextRepo, nodeRepo, taskRepo)

	// Worker
	toolRegistry := tool.NewToolRegistry()
	toolRegistry.Register(builtin.NewBashTool(cfg.BashTool))
	toolRegistry.Register(builtin.NewLlmApiTool(cfg.OpenAI))

	nodeExecutor := workerSvc.NewNodeExecutor(toolRegistry, producer, cfg.Worker)

	// Translator
	nlService := translatorSvc.NewNlToDagService(cfg.OpenAI, cfg.Services.OrchestratorURL)

	// Kafka consumers
	// Worker consumer: listens for node ready events
	workerConsumer := eventbus.NewConsumer(cfg.Kafka, "ai-worker-group",
		[]string{eventbus.TopicNodeReady},
		func(event eventbus.Event) error {
			go nodeExecutor.ExecuteNode(ctx, event)
			return nil
		},
	)
	workerConsumer.Start()
	defer workerConsumer.Stop()

	// Orchestrator consumer: listens for node result and node executed events
	orchestratorConsumer := eventbus.NewConsumer(cfg.Kafka, "orchestrator-group",
		[]string{eventbus.TopicNodeResult, eventbus.TopicNodeExecuted},
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

	// Context consumer: records all events for audit
	contextConsumer := eventbus.NewConsumer(cfg.Kafka, "ai-context-group",
		[]string{eventbus.TopicNodeResult, eventbus.TopicNodeExecuted, eventbus.TopicNodeFailed,
			eventbus.TopicTaskCompleted, eventbus.TopicTaskFailed},
		func(event eventbus.Event) error {
			return contextService.HandleEvent(ctx, event)
		},
	)
	contextConsumer.Start()
	defer contextConsumer.Stop()

	// Start scheduler
	scheduler.Start(ctx)
	defer scheduler.Stop()

	// HTTP server
	r := gin.Default()

	orchestratorHandler.NewOrchestratorHandler(orchestratorService, stateMachine, taskExecutionCtrl, contextService).RegisterRoutes(r)
	translatorHandler.NewTranslatorHandler(nlService).RegisterRoutes(r)
	handler.NewContextHandler(contextService).RegisterRoutes(r)

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
