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

	"github.com/tangying-ai/aios-core/internal/config"
	"github.com/tangying-ai/aios-core/internal/context/handler"
	contextSvc "github.com/tangying-ai/aios-core/internal/context/service"
	"github.com/tangying-ai/aios-core/internal/database"
	"github.com/tangying-ai/aios-core/internal/eventbus"
	"github.com/tangying-ai/aios-core/internal/logger"
	"github.com/tangying-ai/aios-core/internal/model"
	"github.com/tangying-ai/aios-core/internal/model/repository"
	"github.com/tangying-ai/aios-core/internal/orchestrator/service"
	orchestratorHandler "github.com/tangying-ai/aios-core/internal/orchestrator/handler"
	"github.com/tangying-ai/aios-core/internal/outbox"
	publishHandler "github.com/tangying-ai/aios-core/internal/publish/handler"
	publishSvc "github.com/tangying-ai/aios-core/internal/publish/service"
	redisClient "github.com/tangying-ai/aios-core/internal/redis"
	skillHandler "github.com/tangying-ai/aios-core/internal/skill/handler"
	skillSvc "github.com/tangying-ai/aios-core/internal/skill/service"
	translatorHandler "github.com/tangying-ai/aios-core/internal/translator/handler"
	translatorSvc "github.com/tangying-ai/aios-core/internal/translator/service"
	workerService "github.com/tangying-ai/aios-core/internal/worker/service"
	"github.com/tangying-ai/aios-core/internal/worker/tool"
	"github.com/tangying-ai/aios-core/internal/worker/tool/builtin"
	"github.com/tangying-ai/aios-core/internal/worker/executor"
	"github.com/tangying-ai/aios-core/internal/media"
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
	toolManifestRepo := repository.NewToolManifestRepository(pool)

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
	toolRegistry.Register(builtin.NewMediaAnalyzerTool(cfg.OpenAI))
	toolRegistry.Register(builtin.NewContentGeneratorTool(cfg.OpenAI))
	toolRegistry.Register(builtin.NewContentCheckerTool(cfg.OpenAI))
	toolRegistry.Register(builtin.NewPlatformAdapterTool(cfg.OpenAI))
	toolRegistry.Register(builtin.NewChatReviseTool(cfg.OpenAI))
	toolRegistry.Register(builtin.NewChatGenerateTool(cfg.OpenAI))
	toolRegistry.Register(builtin.NewExternalTool(toolRegistry))

	directExec := executor.NewDirectExecutor()
	var sandboxExec *executor.SandboxExecutor
	if cfg.Sandbox.Enabled {
		var err error
		sandboxExec, err = executor.NewSandboxExecutor(cfg.Sandbox.Address)
		if err != nil {
			zap.L().Warn("sandbox client init failed, will fallback", zap.Error(err))
		}
	}

	nodeExecutor := workerService.NewNodeExecutor(toolRegistry, producer, cfg.Worker, directExec, sandboxExec, nodeRepo)

	// Publish
	publishService := publishSvc.NewPublishService(cfg.OpenAI, cfg.Services.OrchestratorURL)

	// Tool manifest service (DB-persisted + Redis-cached tool knowledge base)
	toolManifestSvc := skillSvc.NewToolManifestService(toolManifestRepo, rdb, toolRegistry)
	if err := toolManifestSvc.SyncBuiltinTools(ctx); err != nil {
		zap.L().Warn("Failed to sync builtin tools to DB", zap.Error(err))
	}

	// Translator — uses toolManifestSvc to inject available tool list into LLM prompt
	nlService := translatorSvc.NewNlToDagService(cfg.OpenAI, cfg.Services.OrchestratorURL, toolManifestSvc)

	// Skill (AI assistant dialog system — each chat turn = one Task via orchestrator)
	skillLlmClient := skillSvc.NewLLMClient(cfg.OpenAI)
	skillSessionManager := skillSvc.NewSessionManager(rdb)
	skillPlanService := skillSvc.NewPlanService(skillLlmClient, toolManifestSvc)
	skillResultAssembler := skillSvc.NewResultAssembler(orchestratorService, contextService)

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
			case "RUNNING":
				if _, err := stateService.TransitionNode(ctx, event.NodeID, model.NodeRunning, nil, ""); err != nil {
					zap.L().Error("Failed to set node RUNNING", zap.Error(err))
				}
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

	// Media management — initialize before skill handler so we can resolve media URLs
	var mediaSvc *media.MediaService
	if storageSvc, err := media.NewStorageService(cfg.MinIO); err == nil {
		mediaSvc = media.NewMediaService(pool, storageSvc)
		media.NewMediaHandler(mediaSvc).RegisterRoutes(r)
		zap.L().Info("Media service initialized with MinIO storage")
	} else {
		zap.L().Warn("MinIO storage not available, media uploads disabled", zap.Error(err))
	}

	// Wire media service into publish service for video pipeline support
	if mediaSvc != nil {
		publishService.SetMediaService(mediaSvc)
	}

	// Register video pipeline tools — requires mediaSvc for MinIO download
	if mediaSvc != nil {
		toolRegistry.Register(builtin.NewVideoMetadataTool(cfg.OpenAI, mediaSvc))
		toolRegistry.Register(builtin.NewVideoAnalyzerTool(cfg.OpenAI, mediaSvc))
	}
	toolRegistry.Register(builtin.NewVideoCopyGeneratorTool(cfg.OpenAI))

	skillHandler.NewSessionHandler(
		skillSessionManager, skillPlanService, skillResultAssembler, mediaSvc,
	).RegisterRoutes(r)
	publishHandler.NewToolHandler(toolRegistry, toolManifestSvc).RegisterRoutes(r)

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
