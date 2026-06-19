package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	skillHandler "github.com/tangying-ai/aios-core/internal/agents/chat/handler"
	skillSvc "github.com/tangying-ai/aios-core/internal/agents/chat/service"
	publishHandler "github.com/tangying-ai/aios-core/internal/agents/publish/handler"
	publishSvc "github.com/tangying-ai/aios-core/internal/agents/publish/service"
	"github.com/tangying-ai/aios-core/internal/core/config"
	"github.com/tangying-ai/aios-core/internal/core/context/handler"
	contextSvc "github.com/tangying-ai/aios-core/internal/core/context/service"
	"github.com/tangying-ai/aios-core/internal/core/database"
	"github.com/tangying-ai/aios-core/internal/core/eventbus"
	"github.com/tangying-ai/aios-core/internal/core/logger"
	"github.com/tangying-ai/aios-core/internal/core/media"
	"github.com/tangying-ai/aios-core/internal/core/model"
	"github.com/tangying-ai/aios-core/internal/core/model/repository"
	orchestratorHandler "github.com/tangying-ai/aios-core/internal/core/orchestrator/handler"
	"github.com/tangying-ai/aios-core/internal/core/orchestrator/service"
	"github.com/tangying-ai/aios-core/internal/core/outbox"
	redisClient "github.com/tangying-ai/aios-core/internal/core/redis"
	translatorHandler "github.com/tangying-ai/aios-core/internal/core/translator/handler"
	translatorSvc "github.com/tangying-ai/aios-core/internal/core/translator/service"
	"github.com/tangying-ai/aios-core/internal/core/worker/executor"
	workerService "github.com/tangying-ai/aios-core/internal/core/worker/service"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool/builtin"

	bidHandler "github.com/tangying-ai/aios-core/internal/agents/bid/handler"
	bidRepo "github.com/tangying-ai/aios-core/internal/agents/bid/repository"
	bidsvc "github.com/tangying-ai/aios-core/internal/agents/bid/service"
	videoHandler "github.com/tangying-ai/aios-core/internal/agents/video/handler"
	videoRepo "github.com/tangying-ai/aios-core/internal/agents/video/repository"
	videoSvc "github.com/tangying-ai/aios-core/internal/agents/video/service"
	"github.com/tangying-ai/aios-core/internal/core/skillruntime"
	"github.com/tangying-ai/aios-core/internal/core/workflow"
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

	// Wire StateMachine into Scheduler for heartbeat timeout handling
	scheduler.SetStateMachine(stateMachine)

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

	// Chat (AI assistant dialog system — each chat turn = one Task via orchestrator)
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

	// Progress consumer — handles heartbeat and progress events from long-running nodes
	progressConsumer := eventbus.NewConsumer(cfg.Kafka, "ai-progress-group",
		[]string{eventbus.TopicProgress},
		func(event eventbus.Event) error {
			switch event.Status {
			case "HEARTBEAT":
				// Update heartbeat timestamp in DB
				if err := nodeRepo.UpdateHeartbeat(ctx, event.NodeID, -1, ""); err != nil {
					zap.L().Error("Failed to update heartbeat", zap.Error(err))
				}
			case "PROGRESS":
				// Update progress and step in DB
				var progress float64
				var step string
				if event.Output != nil {
					if p, ok := event.Output["progress"].(float64); ok {
						progress = p
					}
					if s, ok := event.Output["step"].(string); ok {
						step = s
					}
				}
				if err := nodeRepo.UpdateHeartbeat(ctx, event.NodeID, progress, step); err != nil {
					zap.L().Error("Failed to update progress", zap.Error(err))
				}
				// Also record context for auditing
				if progress > 0 {
					c := &model.Context{
						ContextType:  model.ContextNodeProgress,
						TaskID:       event.TaskID,
						NodeID:       event.NodeID,
						SourceModule: "ProgressConsumer",
						Message:      fmt.Sprintf("Progress: %.0f%% — %s", progress*100, step),
						Metadata:     map[string]interface{}{"progress": progress, "step": step},
					}
					_ = contextRepo.Save(ctx, c)
				}
			case "CHECKPOINT":
				// Persist checkpoint data
				var progress float64
				var step string
				var checkpointData map[string]interface{}
				if event.Output != nil {
					if p, ok := event.Output["progress"].(float64); ok {
						progress = p
					}
					if s, ok := event.Output["step"].(string); ok {
						step = s
					}
					if cp, ok := event.Output["checkpoint"].(map[string]interface{}); ok {
						checkpointData = cp
					}
				}
				if err := nodeRepo.UpdateHeartbeat(ctx, event.NodeID, progress, step); err != nil {
					zap.L().Error("Failed to update checkpoint heartbeat", zap.Error(err))
				}
				c := &model.Context{
					ContextType:  model.ContextNodeCheckpoint,
					TaskID:       event.TaskID,
					NodeID:       event.NodeID,
					SourceModule: "ProgressConsumer",
					Message:      fmt.Sprintf("Checkpoint at %.0f%% — %s", progress*100, step),
					SnapshotData: checkpointData,
					Metadata:     map[string]interface{}{"progress": progress, "step": step},
				}
				_ = contextRepo.Save(ctx, c)
				zap.L().Info("Checkpoint persisted",
					zap.String("nodeId", event.NodeID),
					zap.Float64("progress", progress),
				)
			}
			return nil
		},
	)
	progressConsumer.Start()
	defer progressConsumer.Stop()

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

	// Bid (tender) generation module
	bidRepository := bidRepo.NewBidRepository(pool)
	bidService := bidsvc.NewBidService(bidRepository, orchestratorService, taskExecutionCtrl, stateService, taskRepo, nodeRepo)
	bidHandler.NewBidHandler(bidService).RegisterRoutes(r)
	zap.L().Info("Bid service registered")

	// Workflow templates — reusable DAG blueprints
	workflowRepo := workflow.NewRepository(pool)
	workflowService := workflow.NewService(workflowRepo, orchestratorService)
	workflow.NewHandler(workflowService).RegisterRoutes(r)
	// Ensure schema and seed built-in templates
	workflow.EnsureSchema(ctx, pool)
	zap.L().Info("Workflow service registered")

	// ── Video Creation Upgrade (feature-gated) ──
	if cfg.Video.VideoCreationEnabled {
		zap.L().Info("Video creation enabled — registering video modules",
			zap.String("model_provider_mode", cfg.Video.ModelProviderMode),
		)

		// Skill Runtime
		skillReg, skillErrs := skillruntime.LoadSkills(cfg.Video.SkillRoot)
		for _, err := range skillErrs {
			zap.L().Warn("Skill load error", zap.Error(err))
		}
		skillHandler := skillruntime.NewHandler(skillReg)
		skillHandler.SetCompiler(func(s *skillruntime.SkillManifest) (json.RawMessage, error) {
			return workflow.CompileSkillToDAG(s)
		})
		skillHandler.RegisterRoutes(r)
		zap.L().Info("Skill runtime registered", zap.Int("skills_loaded", len(skillReg.List())))

		// Auto-register each loaded skill as a workflow template
		for _, skill := range skillReg.List() {
			if skill.Health != skillruntime.HealthHealthy {
				continue
			}
			dag, err := workflow.CompileSkillToDAG(skill)
			if err != nil {
				zap.L().Warn("Skill compile failed", zap.String("skill", skill.Name), zap.Error(err))
				continue
			}
			templateID := workflow.TemplateIDForSkill(skill.Name, skill.Version)
			tmpl, err := workflowService.Upsert(ctx, &workflow.CreateTemplateRequest{
				ID:          templateID,
				Version:     skill.Version,
				Name:        skill.Name + "-workflow",
				Description: skill.Description,
				Category:    skill.Category,
				DAG:         dag,
			})
			if err != nil {
				zap.L().Warn("Auto-register workflow failed", zap.String("skill", skill.Name), zap.Error(err))
			} else {
				zap.L().Info("Auto-registered workflow", zap.String("id", tmpl.ID), zap.String("skill", skill.Name+"@"+skill.Version))
			}
		}

		// Video Projects
		videoProjectRepo := videoRepo.NewProjectRepository(pool)
		videoProjectSvc := videoSvc.NewProjectService(videoProjectRepo)
		videoHandler.NewProjectHandler(videoProjectSvc).RegisterRoutes(r)

		// Workflow Runs
		workflowRunRepo := workflow.NewRunRepository(pool)
		workflowRunSvc := workflow.NewRunService(workflowRepo, workflowRunRepo, orchestratorService)
		videoHandler.NewWorkflowHandler(workflowRunSvc).RegisterRoutes(r)
		zap.L().Info("Video project and workflow run services registered")
	}

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
