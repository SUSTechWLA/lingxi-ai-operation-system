package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	publishHandler "github.com/tangying-ai/aios-core/internal/agents/publish/handler"
	publishSvc "github.com/tangying-ai/aios-core/internal/agents/publish/service"
	"github.com/tangying-ai/aios-core/internal/core/agentruntime"
	"github.com/tangying-ai/aios-core/internal/core/apispec"
	"github.com/tangying-ai/aios-core/internal/core/artifact"
	"github.com/tangying-ai/aios-core/internal/core/auth"
	"github.com/tangying-ai/aios-core/internal/core/config"
	"github.com/tangying-ai/aios-core/internal/core/context/handler"
	contextSvc "github.com/tangying-ai/aios-core/internal/core/context/service"
	"github.com/tangying-ai/aios-core/internal/core/database"
	"github.com/tangying-ai/aios-core/internal/core/eventbus"
	"github.com/tangying-ai/aios-core/internal/core/health"
	"github.com/tangying-ai/aios-core/internal/core/hyperframes"
	"github.com/tangying-ai/aios-core/internal/core/localrunner"
	"github.com/tangying-ai/aios-core/internal/core/logger"
	"github.com/tangying-ai/aios-core/internal/core/media"
	"github.com/tangying-ai/aios-core/internal/core/model"
	"github.com/tangying-ai/aios-core/internal/core/model/repository"
	"github.com/tangying-ai/aios-core/internal/core/modelgateway"
	"github.com/tangying-ai/aios-core/internal/core/modelgateway/providers/fake"
	"github.com/tangying-ai/aios-core/internal/core/modelgateway/providers/openai"
	"github.com/tangying-ai/aios-core/internal/core/modelgateway/providers/stability"
	orchestratorHandler "github.com/tangying-ai/aios-core/internal/core/orchestrator/handler"
	"github.com/tangying-ai/aios-core/internal/core/orchestrator/service"
	"github.com/tangying-ai/aios-core/internal/core/outbox"
	redisClient "github.com/tangying-ai/aios-core/internal/core/redis"
	"github.com/tangying-ai/aios-core/internal/core/skillcapability"
	translatorHandler "github.com/tangying-ai/aios-core/internal/core/translator/handler"
	translatorSvc "github.com/tangying-ai/aios-core/internal/core/translator/service"
	"github.com/tangying-ai/aios-core/internal/core/worker/executor"
	workerService "github.com/tangying-ai/aios-core/internal/core/worker/service"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool/builtin"

	videoAssets "github.com/tangying-ai/aios-core/internal/agents/video/assets"
	videoAssistant "github.com/tangying-ai/aios-core/internal/agents/video/assistant"
	videoHandler "github.com/tangying-ai/aios-core/internal/agents/video/handler"
	videoPlanJudge "github.com/tangying-ai/aios-core/internal/agents/video/planjudge"
	videoRepo "github.com/tangying-ai/aios-core/internal/agents/video/repository"
	videoSvc "github.com/tangying-ai/aios-core/internal/agents/video/service"
	"github.com/tangying-ai/aios-core/internal/core/skillruntime"
	videodirector "github.com/tangying-ai/aios-core/internal/core/video/director"
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
	authRepo := auth.NewRepository(pool)
	taskRepo := repository.NewTaskRepository(pool)
	nodeRepo := repository.NewNodeRepository(pool)
	depRepo := repository.NewNodeDependencyRepository(pool)
	contextRepo := repository.NewContextRepository(pool)
	toolManifestRepo := repository.NewToolManifestRepository(pool)

	// Services
	authService := auth.NewService(authRepo, auth.NewTokenIssuer(auth.TokenConfig{
		Secret:          cfg.Auth.TokenSecret,
		AccessTokenTTL:  time.Duration(cfg.Auth.AccessTokenTTLSeconds) * time.Second,
		RefreshTokenTTL: time.Duration(cfg.Auth.RefreshTokenTTLSeconds) * time.Second,
	}))
	authMiddleware := auth.NewMiddleware(authService)
	requireAuth := authMiddleware.RequireAuth()
	eventSaver := outbox.NewOutboxSaver(pool)
	stateService := service.NewStateService(nodeRepo, taskRepo, depRepo, contextRepo, eventSaver)
	orchestratorService := service.NewOrchestratorService(taskRepo, nodeRepo, depRepo, contextRepo, stateService)
	stateMachine := service.NewStateMachine(stateService, nodeRepo, taskRepo, eventSaver)
	dependencyChecker := service.NewDependencyChecker(nodeRepo, stateService, eventSaver)
	taskExecutionCtrl := service.NewTaskExecutionControl(taskRepo, nodeRepo, stateService)
	scheduler := service.NewScheduler(nodeRepo, stateService, producer)
	contextService := contextSvc.NewContextService(contextRepo, nodeRepo, taskRepo)
	localRunnerService := localrunner.NewService(pool)

	// Wire DependencyChecker into StateService for event-driven scheduling
	stateService.SetDependencyChecker(dependencyChecker)

	// Wire StateMachine into Scheduler for heartbeat timeout handling
	scheduler.SetStateMachine(stateMachine)

	// Outbox relay with retry, DLQ, and exponential backoff
	outboxRelay := outbox.NewRelay(pool, producer, outbox.DefaultRelayConfig())
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
	toolRegistry.Register(builtin.NewExternalTool(toolRegistry))
	if cfg.Video.VideoCreationEnabled {
		builtin.RegisterVideoCreationExternalTools(toolRegistry)
	}

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
	nodeExecutor.SetLocalJobDispatcher(localRunnerService)

	// Publish
	publishService := publishSvc.NewPublishService(cfg.OpenAI, cfg.Services.OrchestratorURL)

	// Tool manifest service (DB-persisted + Redis-cached tool knowledge base)
	toolManifestSvc := tool.NewToolManifestService(toolManifestRepo, rdb, toolRegistry)
	if err := toolManifestSvc.SyncBuiltinTools(ctx); err != nil {
		zap.L().Warn("Failed to sync builtin tools to DB", zap.Error(err))
	}

	skillCapabilityReg, capabilityToolManifests, skillCapErrs := skillcapability.LoadCapabilities(cfg.Video.SkillCapabilityRoot)
	for _, err := range skillCapErrs {
		zap.L().Warn("Skill capability load error", zap.Error(err))
	}
	for _, manifest := range capabilityToolManifests {
		toolRegistry.RegisterExternal(manifest)
		if err := toolManifestSvc.RegisterManifest(ctx, manifest); err != nil {
			zap.L().Warn("Failed to register skill capability tool",
				zap.String("name", manifest.Name),
				zap.Error(err))
		}
	}
	zap.L().Info("Skill capability registry initialized",
		zap.Int("capabilities", len(skillCapabilityReg.List())),
		zap.Int("tools", len(capabilityToolManifests)))
	videoDirectorRegistry := videodirector.DefaultRegistry()
	videoDirectorRegistry.RegisterRoleAgents(skillCapabilityReg.RoleAgents("video_creation"))
	zap.L().Info("Video role agent registry initialized",
		zap.Int("roleAgents", len(videoDirectorRegistry.List())))

	// Translator — uses toolManifestSvc to inject available tool list into LLM prompt
	nlService := translatorSvc.NewNlToDagService(cfg.OpenAI, cfg.Services.OrchestratorURL, toolManifestSvc)

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
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, DeviceID")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	auth.NewHandler(authService).RegisterRoutes(r)
	orchestratorHandler.NewOrchestratorHandler(orchestratorService, stateMachine, taskExecutionCtrl, contextService).RegisterRoutes(r, requireAuth)
	health.NewHandler([]health.DependencyCheck{
		{Name: "postgres", Check: pool.Ping},
		{Name: "redis", Check: func(ctx context.Context) error {
			return rdb.Ping(ctx).Err()
		}},
		{Name: "kafka", Check: health.KafkaCheck(cfg.Kafka.BootstrapServers)},
	}).RegisterRoutes(r)
	translatorHandler.NewTranslatorHandler(nlService).RegisterRoutes(r, requireAuth)
	handler.NewContextHandler(contextService).RegisterRoutes(r, requireAuth)
	publishHandler.NewPublishHandler(publishService).RegisterRoutes(r, requireAuth)
	publishHandler.NewTraceHandler(orchestratorService, contextService).RegisterRoutes(r, requireAuth)
	localRunnerHandler := localrunner.NewHandler(localRunnerService, stateMachine, requireAuth)
	localRunnerHandler.RegisterRoutes(r)
	// Preflight: check local capabilities before starting a video pipeline.
	r.GET("/api/video/preflight", requireAuth, localrunner.HandleVideoPreflight(localRunnerService))

	// Media management — initialize before skill handler so we can resolve media URLs
	var mediaSvc *media.MediaService
	if storageSvc, err := media.NewStorageService(cfg.MinIO); err == nil {
		mediaSvc = media.NewMediaService(pool, storageSvc)
		media.NewMediaHandler(mediaSvc, requireAuth).RegisterRoutes(r)
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

	publishHandler.NewToolHandler(toolRegistry, toolManifestSvc).RegisterRoutes(r, requireAuth)
	skillcapability.NewHandler(skillCapabilityReg).RegisterRoutes(r, requireAuth)
	videodirector.NewHandler(videoDirectorRegistry).RegisterRoutes(r, requireAuth)

	agentRunRepo := agentruntime.NewRepository(pool)

	// ── ModelGateway (initialized early so LLMPlanner and PromptTools can use it) ──
	var gw *modelgateway.Gateway
	if cfg.Video.VideoCreationEnabled {
		gw = modelgateway.NewGateway(cfg.Video.ModelProviderMode)
		if cfg.Video.ModelProviderMode == "real" {
			openaiProvider := openai.NewProvider()
			switch cfg.Video.ImageProvider {
			case "stability":
				gw.RegisterProvider(stability.NewProvider(), modelgateway.CapTextToImage)
			default:
				gw.RegisterProvider(openaiProvider, modelgateway.CapTextToImage)
			}
			gw.RegisterProvider(openaiProvider, modelgateway.CapTextToText)
		} else {
			fakeProvider := fake.NewProvider()
			gw.RegisterProvider(fakeProvider, modelgateway.CapTextToImage)
			gw.RegisterProvider(fakeProvider, modelgateway.CapTextToVideo)
			gw.RegisterProvider(fakeProvider, modelgateway.CapImageToVideo)
			gw.RegisterProvider(fakeProvider, modelgateway.CapTextToText)
		}
		builtin.SetModelGateway(gw)
		builtin.SetVideoCreationConfig(cfg.OpenAI, cfg.Video.SkillRoot)
		builtin.SetEncryptionSecret(cfg.Auth.TokenSecret)
		builtin.SetRuntimeConfigPersistPath(filepath.Join(cfg.Video.SkillRoot, "..", "runtime-model-provider.json"))
		// HyperFrames Render Service config (replaces CLI dependency).
		if cfg.Video.HyperFramesCLIPath != "" {
			builtin.SetHyperFramesCLIPath(cfg.Video.HyperFramesCLIPath)
		}
		builtin.SetHyperFramesConfig(hyperframes.Config{
			Mode:           hyperframes.Mode(cfg.HyperFrames.Mode),
			ServiceURL:     cfg.HyperFrames.ServiceURL,
			TimeoutSec:     cfg.HyperFrames.TimeoutSec,
			DefaultFPS:     cfg.HyperFrames.DefaultFPS,
			DefaultQuality: cfg.HyperFrames.DefaultQuality,
			DefaultFormat:  cfg.HyperFrames.DefaultFormat,
			MaxWorkers:     cfg.HyperFrames.MaxWorkers,
			UseGPU:         cfg.HyperFrames.UseGPU,
			ProjectRoot:    cfg.HyperFrames.ProjectRoot,
			OutputRoot:     cfg.HyperFrames.OutputRoot,
		})
		zap.L().Info("ModelGateway initialized for agent planner and video tools",
			zap.String("mode", cfg.Video.ModelProviderMode))
	}

	agentPlanner := buildAgentPlanner(cfg, toolRegistry, gw)
	videoDirectorAdapter := &stageDirectorRegistry{videoDirectorRegistry}
	agentRunner := agentruntime.NewRunner(
		orchestratorService,
		agentRunRepo,
		agentPlanner,
		agentruntime.NewPlanGuard(toolRegistry, localRunnerService).WithDirectors(videoDirectorAdapter),
		func() *agentruntime.PlanCompiler {
			pc := agentruntime.NewPlanCompiler(toolRegistry)
			pc.WithDirectors(videoDirectorAdapter)
			return pc
		}(),
	).WithPlanJudge(videoPlanJudge.NewRuntimeJudge())
	agentRuntimeHandler := agentruntime.NewHandler(agentRunner, nodeRepo, stateMachine)
	agentRuntimeHandler.RegisterRoutes(r, requireAuth)

	// Workflow templates — reusable DAG blueprints
	workflowRepo := workflow.NewRepository(pool)
	workflowService := workflow.NewService(workflowRepo, orchestratorService)
	workflow.NewHandler(workflowService).RegisterRoutes(r, requireAuth)
	// Ensure schema and seed built-in templates
	workflow.EnsureSchema(ctx, pool)
	zap.L().Info("Workflow service registered")

	// ── Video Creation Upgrade (feature-gated) ──
	if cfg.Video.VideoCreationEnabled {
		zap.L().Info("Video creation enabled — registering video modules",
			zap.String("model_provider_mode", cfg.Video.ModelProviderMode),
		)
		// ModelGateway and builtin config are initialized earlier (before buildAgentPlanner).
		// gw is already created and providers registered; we just reference it here.

		// Runtime model-provider config — synced from frontend Desktop page, persisted to disk
		modelProviderHandler := func(c *gin.Context) {
			switch c.Request.Method {
			case "GET":
				effective := builtin.GetVideoCreationOpenAIConfig()
				runtime := builtin.GetRuntimeModelProviderConfig()
				c.JSON(200, gin.H{"code": 200, "message": "ok", "data": gin.H{
					"baseUrl":  effective.BaseURL,
					"model":    effective.Model,
					"hasKey":   effective.APIKey != "",
					"endpoint": strings.TrimRight(effective.BaseURL, "/") + "/chat/completions",
					"fromUser": runtime.BaseURL != "",
					"fromEnv":  builtin.GetEnvOpenAIConfig().APIKey != "" || runtime.APIKey == "",
				}})
			case "PUT":
				var req struct {
					BaseURL string `json:"baseUrl"`
					APIKey  string `json:"apiKey"`
					Model   string `json:"model"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(400, gin.H{"code": 400, "message": "invalid request", "data": nil})
					return
				}
				// Merge with existing: keep old key when not provided
				existing := builtin.GetRuntimeModelProviderConfig()
				if req.APIKey == "" {
					req.APIKey = existing.APIKey
				}
				builtin.SetRuntimeModelProviderConfig(builtin.RuntimeModelProviderConfig{
					BaseURL: req.BaseURL,
					APIKey:  req.APIKey,
					Model:   req.Model,
				})
				zap.L().Info("Runtime model-provider config updated",
					zap.String("baseUrl", req.BaseURL),
					zap.String("model", req.Model),
					zap.Bool("hasApiKey", req.APIKey != ""),
				)
				c.JSON(200, gin.H{"code": 200, "message": "ok", "data": nil})
			case "DELETE":
				builtin.ClearRuntimeModelProviderConfig()
				zap.L().Info("Runtime model-provider config cleared — using env defaults")
				c.JSON(200, gin.H{"code": 200, "message": "cleared, using env defaults", "data": nil})
			default:
				c.JSON(405, gin.H{"code": 405, "message": "method not allowed", "data": nil})
			}
		}
		r.GET("/api/config/model-provider", requireAuth, modelProviderHandler)
		r.PUT("/api/config/model-provider", requireAuth, modelProviderHandler)
		r.DELETE("/api/config/model-provider", requireAuth, modelProviderHandler)

		// Skill Runtime
		skillReg, skillErrs := skillruntime.LoadSkills(cfg.Video.SkillRoot)
		for _, err := range skillErrs {
			zap.L().Warn("Skill load error", zap.Error(err))
		}
		skillHandler := skillruntime.NewHandler(skillReg)
		skillHandler.SetOpenAIConfig(cfg.OpenAI)
		skillHandler.SetCompiler(func(s *skillruntime.SkillManifest) (json.RawMessage, error) {
			return workflow.CompileSkillToDAG(s)
		})
		skillHandler.RegisterRoutes(r, requireAuth)
		zap.L().Info("Skill runtime registered", zap.Int("skills_loaded", len(skillReg.List())))

		if cfg.Video.LegacySkillWorkflowAutoRegister {
			// Legacy compatibility only: dynamic agent runs are the primary path,
			// so Skill packages are no longer forced into workflow_templates.
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
		} else {
			zap.L().Info("Legacy skill-to-workflow auto-register disabled; dynamic agent runtime is primary")
		}

		// Video Projects
		videoProjectRepo := videoRepo.NewProjectRepository(pool)
		videoProjectSvc := videoSvc.NewProjectService(videoProjectRepo)
		projectHandler := videoHandler.NewProjectHandler(videoProjectSvc, requireAuth)
		projectHandler.RegisterRoutes(r)
		videoCreationSvc := videoSvc.NewCreationService(videoProjectRepo)
		videoHandler.NewCreationHandler(videoCreationSvc, requireAuth).RegisterRoutes(r)
		videoAssistant.NewHandler(requireAuth).RegisterRoutes(r)

		// Workflow Runs
		workflowRunRepo := workflow.NewRunRepository(pool)
		// Wire the state machine to sync node statuses to the workflow run's
		// stage_statuses JSONB so the frontend progress panel shows live status.
		stateMachine.SetStageStatusSyncer(&runStatusSyncer{runRepo: workflowRunRepo})
		workflowRunSvc := workflow.NewRunService(workflowRepo, workflowRunRepo, orchestratorService)
		stageApprovalSvc := workflow.NewStageApprovalService(workflowRunRepo, nodeRepo, stateMachine)

		// Checkpoint store and service — persists stage boundaries for recovery.
		checkpointStore := workflow.NewCheckpointStore(pool)
		checkpointSvc := workflow.NewCheckpointService(checkpointStore, workflowRunRepo, nodeRepo)
		stateService.SetTransitionHook(checkpointSvc.TransitionHook())
		_ = workflow.EnsureCheckpointSchema(ctx, pool)

		// Decision log store — audit trail for every approval, rejection, and pipeline decision.
		decisionLogStore := workflow.NewDecisionLogStore(pool)
		_ = workflow.EnsureDecisionLogSchema(ctx, pool)

		// Wire decision log into the agent runtime handler so approve/reject
		// writes audit-trail entries automatically.
		agentRuntimeHandler.WithDecisionLogWriter(&decisionLogAdapter{store: decisionLogStore})

		artifactRepo := artifact.NewRepository(pool)
		artifactSvc := artifact.NewService(artifactRepo)
		stageApprovalSvc.WithArtifactApprover(artifactSvc)
		videoAssets.NewHandler(artifactSvc, requireAuth).RegisterRoutes(r)
		videoHandler.NewWorkflowHandler(workflowRunSvc, stageApprovalSvc).
			WithCheckpointService(checkpointSvc).
			RegisterRoutes(r, requireAuth)
		artifactHandler := artifact.NewHandler(artifactSvc, workflowRunRepo, nodeRepo).
			WithAgentTaskStore(taskRepo)

		// Wire session dependencies into the project handler
		projectHandler.WithSessionDependencies(artifactSvc, localRunnerService)
		// Wire LLM-based revision support so the /artifacts/:id/revise endpoint
		// can actually call the LLM with original content + revision instruction.
		artifactHandler.SetRevisionConfig(cfg.Video.SkillRoot, func(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
			effectiveCfg := builtin.GetVideoCreationOpenAIConfig()
			// Also try to pull config from the local desktop agent, matching
			// the behavior of executeSkillStageAgent (the workflow LLM call path).
			if localCfg, ok := builtin.TryFetchLocalAgentConfig(); ok {
				if localCfg.BaseURL != "" {
					effectiveCfg.BaseURL = localCfg.BaseURL
				}
				if localCfg.APIKey != "" {
					effectiveCfg.APIKey = localCfg.APIKey
				}
				if localCfg.Model != "" {
					effectiveCfg.Model = localCfg.Model
				}
			}
			if effectiveCfg.APIKey == "" {
				return "", fmt.Errorf("LLM API key 未配置，无法执行返工。请在桌面端设置页面配置 API Key。")
			}
			llmTool := builtin.NewLlmApiTool(effectiveCfg)
			var toolCtx tool.ToolContext
			result := llmTool.Execute(ctx, map[string]interface{}{
				"prompt":     systemPrompt + "\n\n---\n\n" + userPrompt,
				"max_tokens": 8000,
			}, toolCtx)
			if !result.Success {
				return "", fmt.Errorf("LLM 返工调用失败: %s", result.Error)
			}
			content, _ := result.Data["content"].(string)
			if content == "" {
				return "", fmt.Errorf("LLM 返回了空内容")
			}
			return content, nil
		})
		artifactHandler.RegisterRoutes(r, requireAuth)

		// Wire artifact service into the agent runtime handler for stale tracking
		// and artifact approval on review actions.
		if artifactReviewStore := newAgentRuntimeArtifactReviewStore(pool); artifactReviewStore != nil {
			agentRuntimeHandler.WithArtifactReviewStore(artifactReviewStore)
		}
		agentRuntimeHandler.
			WithArtifactService(artifactSvc).
			WithProjectIDResolver(&taskProjectIDResolver{runRepo: workflowRunRepo, taskRepo: taskRepo})

		// Wire repository-backed render dependency checker so HYPERFRAMES_RENDER
		// validates database facts (artifact status) before dispatching a LocalJob.
		nodeExecutor.SetRenderDependencyChecker(
			workerService.NewRepositoryBackedRenderDependencyChecker(
				&artifactStateAdapter{svc: artifactSvc},
				workerService.NewRepositoryReviewApprovalChecker(&artifactStateAdapter{svc: artifactSvc}),
				workerService.NewRunnerCapabilityChecker(&runnerServiceAdapter{svc: localRunnerService}),
			),
		)

		// Wire artifact sync callback so local job completions automatically
		// write artifact metadata to the cloud ArtifactIndex.
		localRunnerHandler.WithArtifactSyncCallback(func(ctx context.Context, projectID, taskID, nodeID, toolName, command string, output map[string]interface{}) error {
			workflowRunID, err := workflowRunRepo.FindRunIDByTaskID(ctx, taskID)
			if err != nil || workflowRunID == "" {
				zap.L().Warn("artifact sync: workflowRunID missing, fallback to taskID",
					zap.String("taskID", taskID),
					zap.String("nodeID", nodeID),
					zap.String("projectID", projectID),
					zap.String("toolName", toolName),
					zap.String("command", command),
					zap.Error(err),
				)
				workflowRunID = taskID
			}
			return syncArtifactsFromLocalJob(ctx, artifactSvc, nodeRepo, projectID, workflowRunID, taskID, nodeID, toolName, command, output)
		})

		zap.L().Info("Video project and watch workflow run services registered")
	}

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: r,
	}

	// OpenAPI spec + Swagger UI (always up-to-date with registered routes)
	apispec.Register(r, apispec.BuildCloudSpec())
	zap.L().Info("OpenAPI docs and Swagger UI registered at /docs")

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

func buildAgentPlanner(cfg *config.Config, toolRegistry *tool.ToolRegistry, gw *modelgateway.Gateway) agentruntime.Planner {
	maxTools := cfg.Agent.PlannerMaxTools
	if maxTools <= 0 {
		maxTools = 6
	}
	heuristic := agentruntime.NewHeuristicPlannerWithMaxTools(toolRegistry, maxTools)

	// Prefer ModelGateway for LLMPlanner (caching + retry + provider routing),
	// fall back to direct OpenAI HTTP client when gateway is unavailable.
	var plannerClient agentruntime.PlannerLLMClient
	if gw != nil {
		plannerClient = agentruntime.NewGatewayPlannerClient(gw, cfg.OpenAI.Model)
	} else {
		plannerClient = agentruntime.NewOpenAIPlannerClient(cfg.OpenAI)
	}

	llm := agentruntime.NewLLMPlanner(
		toolRegistry,
		plannerClient,
		agentruntime.LLMPlannerOptions{MaxTools: maxTools},
	)

	switch strings.ToLower(strings.TrimSpace(cfg.Agent.PlannerMode)) {
	case "llm":
		return llm
	case "heuristic":
		return heuristic
	default:
		return agentruntime.NewHybridPlanner(llm, heuristic)
	}
}

// runStatusSyncer adapts the workflow RunRepository to the orchestrator's
// StageStatusSyncer interface, keeping the run's stage_statuses JSONB in
// sync with the DAG node state machine.
type runStatusSyncer struct {
	runRepo *workflow.RunRepository
}

func (s *runStatusSyncer) UpdateStageStatus(ctx context.Context, runID, stageName string, status string) error {
	return s.runRepo.UpdateStageStatus(ctx, runID, stageName, workflow.StageStatus(status))
}

func (s *runStatusSyncer) FindRunIDByTaskID(ctx context.Context, taskID string) (string, error) {
	return s.runRepo.FindRunIDByTaskID(ctx, taskID)
}

// taskProjectIDResolver resolves a project ID from a task ID by looking up
// the workflow run associated with the task.
type taskProjectIDResolver struct {
	runRepo  *workflow.RunRepository
	taskRepo interface {
		FindByID(ctx context.Context, id string) (*model.Task, error)
	}
}

func (r *taskProjectIDResolver) ResolveProjectID(ctx context.Context, taskID string) (string, error) {
	if r.runRepo != nil {
		run, err := r.runRepo.FindByTaskID(ctx, taskID)
		if err == nil && run != nil && strings.TrimSpace(run.ProjectID) != "" {
			return run.ProjectID, nil
		}
	}
	if r.taskRepo == nil {
		return "", nil
	}
	task, err := r.taskRepo.FindByID(ctx, taskID)
	if err != nil || task == nil {
		return "", err
	}
	return projectIDFromTaskInput(task.Input), nil
}

func projectIDFromTaskInput(input map[string]interface{}) string {
	if input == nil {
		return ""
	}
	if projectID, ok := input["projectId"].(string); ok && strings.TrimSpace(projectID) != "" {
		return strings.TrimSpace(projectID)
	}
	if projectID, ok := input["projectID"].(string); ok && strings.TrimSpace(projectID) != "" {
		return strings.TrimSpace(projectID)
	}
	contextMap, _ := input["context"].(map[string]interface{})
	if contextMap == nil {
		return ""
	}
	if projectID, ok := contextMap["projectId"].(string); ok {
		return strings.TrimSpace(projectID)
	}
	if projectID, ok := contextMap["projectID"].(string); ok {
		return strings.TrimSpace(projectID)
	}
	return ""
}

// artifactStateAdapter adapts artifact.Service to workerService.ArtifactStateProvider.
type artifactStateAdapter struct {
	svc *artifact.Service
}

func (a *artifactStateAdapter) FindCurrentByKind(ctx context.Context, projectID, stageName string) (*workerService.ArtifactState, error) {
	art, err := a.svc.FindCurrentByKind(ctx, projectID, stageName)
	if err != nil || art == nil {
		return nil, err
	}
	return &workerService.ArtifactState{
		ID:            art.ID,
		StageName:     art.StageName,
		Kind:          string(art.Kind),
		Status:        art.Status,
		HumanApproved: art.HumanApproved,
		Metadata:      art.Metadata,
	}, nil
}

func (a *artifactStateAdapter) FindCurrentByStageAndKind(ctx context.Context, projectID, stageName, artifactKind string) (*workerService.ArtifactState, error) {
	art, err := a.svc.FindCurrentByStageAndKind(ctx, projectID, stageName, artifactKind)
	if err != nil || art == nil {
		return nil, err
	}
	return &workerService.ArtifactState{
		ID:            art.ID,
		StageName:     art.StageName,
		Kind:          string(art.Kind),
		Status:        art.Status,
		HumanApproved: art.HumanApproved,
		Metadata:      art.Metadata,
	}, nil
}

// runnerServiceAdapter adapts localrunner.Service to workerService.RunnerService
// by delegating to SupportsCommandForAnyUser (no user context needed for guard checks).
type runnerServiceAdapter struct {
	svc *localrunner.Service
}

func (a *runnerServiceAdapter) SupportsCommand(ctx context.Context, command string) (bool, error) {
	if a.svc == nil {
		return false, nil
	}
	return a.svc.SupportsCommandForAnyUser(ctx, command)
}

// syncArtifactsFromLocalJob materializes artifact records from a local job's
// output after it completes successfully. This ensures the cloud ArtifactIndex
// is always in sync with local artifact production.
//
// It processes two sources of artifact data:
//  1. Node output (via BuildArtifactRequestsFromNode) — LLM-generated artifacts
//     that were stored in the node's stdout.
//  2. Local job output.artifacts[] — artifacts produced by local executors
//     (HYPERFRAMES_RENDER, FFMPEG_PROBE, FINAL_REVIEW, ARTIFACT_PACKAGE, etc.)
//     that include stage_name-agnostic metadata like kind, dependsOn, producedByTool,
//     producedByRole, status, and humanApproved.
func syncArtifactsFromLocalJob(
	ctx context.Context,
	artifactSvc *artifact.Service,
	nodeRepo repository.NodeRepo,
	projectID, workflowRunID, taskID, nodeID, toolName, command string,
	output map[string]interface{},
) error {
	// Get the node to extract stage/role metadata.
	node, err := nodeRepo.FindByID(ctx, nodeID)
	if err != nil || node == nil {
		zap.L().Warn("artifact sync: cannot find node",
			zap.String("nodeId", nodeID), zap.Error(err))
		return err
	}

	stage := stageNameFromNodeInput(node)

	if workflowRunID == "" {
		zap.L().Warn("artifact sync: workflowRunID missing, fallback to taskID",
			zap.String("taskID", taskID),
			zap.String("nodeID", nodeID),
			zap.String("projectID", projectID),
			zap.String("toolName", toolName),
			zap.String("command", command),
		)
		workflowRunID = taskID
	}

	syncNode := *node
	syncNode.Status = model.NodeSuccess
	syncNode.Output = output
	if syncNode.Input == nil {
		syncNode.Input = map[string]interface{}{}
	}
	if _, ok := syncNode.Input["stage"]; !ok && stage != "" {
		syncNode.Input["stage"] = stage
	}
	if toolName != "" {
		syncNode.Input["tool"] = toolName
	}

	createdArtifacts, err := artifact.NewArtifactSyncService(artifactSvc, zap.L()).SyncFromNodeOutput(
		ctx,
		projectID,
		workflowRunID,
		taskID,
		&syncNode,
	)
	if err != nil {
		return err
	}
	for _, created := range createdArtifacts {
		_ = bindArtifactIDToReviewGates(ctx, nodeRepo, taskID, nodeID, created)
	}

	return nil
}

type reviewGateNodeFinder interface {
	FindByTaskID(ctx context.Context, taskID string) ([]*model.Node, error)
}

type nodeInputFieldUpdater interface {
	UpdateInputFields(ctx context.Context, id string, fields map[string]interface{}) error
}

func bindArtifactIDToReviewGates(
	ctx context.Context,
	nodeRepo reviewGateNodeFinder,
	taskID, sourceNodeID string,
	created *artifact.Artifact,
) error {
	if nodeRepo == nil || created == nil || created.ID == "" {
		return nil
	}
	updater, ok := nodeRepo.(nodeInputFieldUpdater)
	if !ok {
		return nil
	}
	nodes, err := nodeRepo.FindByTaskID(ctx, taskID)
	if err != nil {
		return err
	}
	for _, node := range nodes {
		if !reviewGateMatchesArtifact(node, sourceNodeID, created) {
			continue
		}
		requiredOutputs := stringSliceFromInterface(node.Input["requiredOutputs"])
		if len(requiredOutputs) == 0 {
			requiredOutputs = []string{string(created.Kind)}
		}
		fields := map[string]interface{}{
			"artifactId":      created.ID,
			"stage":           created.StageName,
			"requiredOutputs": requiredOutputs,
		}
		if err := updater.UpdateInputFields(ctx, node.ID, fields); err != nil {
			return err
		}
	}
	return nil
}

func reviewGateMatchesArtifact(node *model.Node, sourceNodeID string, created *artifact.Artifact) bool {
	if node == nil || created == nil || node.Input == nil {
		return false
	}
	if node.Type != model.NodeTypeControl && node.Type != model.NodeTypeReviewGate {
		return false
	}
	if existing, _ := node.Input["artifactId"].(string); existing != "" {
		return false
	}
	if source, _ := node.Input["sourceNode"].(string); source != "" && source == sourceNodeID {
		return true
	}
	stage, _ := node.Input["stage"].(string)
	if stage != "" && stage != created.StageName {
		return false
	}
	return containsString(stringSliceFromInterface(node.Input["requiredOutputs"]), string(created.Kind)) ||
		containsString(stringSliceFromInterface(node.Input["reviewArtifactKinds"]), string(created.Kind)) ||
		containsString(stringSliceFromInterface(node.Input["artifactKinds"]), string(created.Kind))
}

// buildArtifactRequestsFromOutputArtifacts converts a local job's output.artifacts[]
// into CreateArtifactRequest records. It uses the node's input metadata to fill in
// stage_name, task_id, and role_agent_id context.
func buildArtifactRequestsFromOutputArtifacts(
	projectID, workflowRunID, taskID, stage string,
	node *model.Node,
	toolName string,
	rawArtifacts interface{},
) ([]*artifact.CreateArtifactRequest, error) {
	items, err := localJobArtifactItems(rawArtifacts)
	if err != nil {
		return nil, err
	}

	roleAgentID := ""
	if node != nil && node.Input != nil {
		if v, ok := node.Input["roleAgentId"].(string); ok {
			roleAgentID = v
		}
	}

	requests := make([]*artifact.CreateArtifactRequest, 0, len(items))
	for _, item := range items {
		entry := item

		kind := artifact.ArtifactKind(stringValueFromMap(entry, "kind"))
		if kind == "" {
			return nil, &artifact.ArtifactManifestInvalidError{Message: "artifact kind is required"}
		}
		unitID := stringValueFromMap(entry, "unitId")
		if unitID == "" {
			unitID = stringValueFromMap(entry, "unitID")
		}
		if unitID == "" {
			return nil, &artifact.ArtifactManifestInvalidError{Message: "artifact unitId is required"}
		}
		name := stringValueFromMap(entry, "name")
		if name == "" {
			name = string(kind)
		}
		storageRef := stringValueFromMap(entry, "storageRef")
		mimeType := stringValueFromMap(entry, "mimeType")
		sizeBytes := int64ValueFromMap(entry, "sizeBytes")

		producedByNode := ""
		if node != nil {
			producedByNode = node.ID
		}
		// Collect metadata
		meta := map[string]interface{}{
			"status":         stringValueFromMap(entry, "status"),
			"humanApproved":  boolValueFromMap(entry, "humanApproved"),
			"producedByTool": stringValueFromMap(entry, "producedByTool"),
			"producedByRole": stringValueFromMap(entry, "producedByRole"),
			"taskId":         taskID,
			"roleAgentId":    roleAgentID,
			"producedByNode": producedByNode,
		}
		if meta["status"] == "" {
			meta["status"] = "valid"
		}

		// Carry depends_on from the executor output.
		if deps, ok := entry["dependsOn"]; ok {
			switch v := deps.(type) {
			case []interface{}:
				depStrings := make([]string, 0, len(v))
				for _, d := range v {
					if s, ok := d.(string); ok {
						depStrings = append(depStrings, s)
					}
				}
				meta["dependsOn"] = depStrings
			case []string:
				meta["dependsOn"] = v
			}
		}

		// Merge any additional metadata from the executor.
		if execMeta, ok := entry["metadata"].(map[string]interface{}); ok {
			for k, v := range execMeta {
				if _, exists := meta[k]; !exists {
					meta[k] = v
				}
			}
		}

		requests = append(requests, &artifact.CreateArtifactRequest{
			ProjectID:     projectID,
			WorkflowRunID: workflowRunID,
			TaskID:        taskID,
			StageName:     stage,
			RoleAgentID:   roleAgentID,
			UnitID:        unitID,
			Kind:          kind,
			Name:          name,
			StorageType:   artifact.StorageLocal,
			StorageRef:    storageRef,
			MimeType:      mimeType,
			SizeBytes:     sizeBytes,
			ContentHash:   stringValueFromMap(entry, "contentHash"),
			Provider:      "local-job",
			Model:         toolName,
			Metadata:      meta,
		})
	}
	return requests, nil
}

func localJobArtifactItems(rawArtifacts interface{}) ([]map[string]interface{}, error) {
	switch typed := rawArtifacts.(type) {
	case []interface{}:
		items := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			entry, ok := item.(map[string]interface{})
			if !ok {
				return nil, &artifact.ArtifactManifestInvalidError{Message: "artifact entry must be an object"}
			}
			items = append(items, entry)
		}
		return items, nil
	case []map[string]interface{}:
		return typed, nil
	default:
		return nil, &artifact.ArtifactManifestInvalidError{Message: "artifacts must be an array"}
	}
}

func stageNameFromNodeInput(node *model.Node) string {
	if node == nil || node.Input == nil {
		return "artifact"
	}
	if stage, ok := node.Input["stage"].(string); ok && stage != "" {
		return stage
	}
	if params, ok := node.Input["parameters"].(map[string]interface{}); ok {
		if stage, ok := params["stage"].(string); ok && stage != "" {
			return stage
		}
	}
	stage := strings.TrimSuffix(node.ID, "_exec")
	if stage == "" {
		return "artifact"
	}
	return stage
}

func stringValueFromMap(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func boolValueFromMap(m map[string]interface{}, key string) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}

func int64ValueFromMap(m map[string]interface{}, key string) int64 {
	switch v := m[key].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	case json.Number:
		n, _ := v.Int64()
		return n
	}
	return 0
}

func stringSliceFromInterface(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []interface{}:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

// decisionLogAdapter bridges the workflow DecisionLogStore to the
// agentruntime.DecisionLogWriter interface, allowing the agent runtime
// handler to write audit-trail entries without importing the workflow package.
type decisionLogAdapter struct {
	store workflow.DecisionLogStore
}

// stageDirectorRegistry adapts videodirector.Registry to agentruntime.DirectorRegistry.
// Go's structural typing allows director.Director to satisfy agentruntime.StageDirector.
type stageDirectorRegistry struct {
	inner *videodirector.Registry
}

func (r *stageDirectorRegistry) Get(stageName string) agentruntime.StageDirector {
	d := r.inner.Get(stageName)
	if d == nil {
		return nil
	}
	// director.Director and agentruntime.StageDirector have the same method set;
	// Go's structural typing handles the conversion.
	return d
}

func (a *decisionLogAdapter) Save(ctx context.Context, r *agentruntime.DecisionLogRecord) error {
	return a.store.Save(ctx, &workflow.DecisionLogRecord{
		WorkflowRunID:  r.WorkflowRunID,
		TaskID:         r.TaskID,
		StageName:      r.StageName,
		DecisionType:   r.DecisionType,
		Selected:       r.Selected,
		ApprovedByUser: r.ApprovedByUser,
		ReviewerID:     r.ReviewerID,
		Comment:        r.Comment,
	})
}
