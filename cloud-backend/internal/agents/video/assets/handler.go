package assets

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
	"github.com/tangying-ai/aios-core/internal/core/auth"
	"github.com/tangying-ai/aios-core/internal/core/common/httpx"
)

type Handler struct {
	sink       ArtifactSink
	projects   ProjectMaterialReader
	middleware []gin.HandlerFunc
}

// ProjectMaterialReader is deliberately narrow: material registration only
// needs the existing user-scoped project lookup for ownership verification.
type ProjectMaterialReader interface {
	GetProject(ctx context.Context, userID, projectID string) (*model.VideoProject, error)
}

type RegisterExternalGenerationResultRequest struct {
	Kind                AssetType `json:"kind"`
	StorageType         string    `json:"storageType,omitempty"`
	StorageRef          string    `json:"storageRef"`
	MimeType            string    `json:"mimeType,omitempty"`
	SizeBytes           int64     `json:"sizeBytes,omitempty"`
	ContentHash         string    `json:"contentHash,omitempty"`
	PromptHash          string    `json:"promptHash,omitempty"`
	DurationSec         int       `json:"durationSec,omitempty"`
	Description         string    `json:"description,omitempty"`
	Tags                []string  `json:"tags,omitempty"`
	RelatedShotID       string    `json:"relatedShotId,omitempty"`
	Source              string    `json:"source,omitempty"`
	GenerationRequestID string    `json:"generationRequestId,omitempty"`
	ExternalPlatform    string    `json:"externalPlatform,omitempty"`
	ReferenceAssetIDs   []string  `json:"referenceAssetIds,omitempty"`
}

// RegisterProjectMaterialRequest contains metadata only. It intentionally has
// no bytes, inline JSON, cloud URL, or storage type field.
type RegisterProjectMaterialRequest struct {
	Name        string    `json:"name"`
	Kind        AssetType `json:"kind"`
	StorageRef  string    `json:"storageRef"`
	MimeType    string    `json:"mimeType"`
	SizeBytes   int64     `json:"sizeBytes"`
	ContentHash string    `json:"contentHash"`
}

func NewHandler(sink ArtifactSink, projects ProjectMaterialReader, middleware ...gin.HandlerFunc) *Handler {
	return &Handler{sink: sink, projects: projects, middleware: middleware}
}

func (h *Handler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/video-projects", h.middleware...)
	api.POST("/:id/external-generation-results", h.RegisterExternalGenerationResult)
	api.POST("/:id/materials", h.RegisterProjectMaterial)
}

// RegisterProjectMaterial stores only an index to bytes retained by the local
// agent. Project lookup is user-scoped so forbidden projects are
// indistinguishable from missing projects.
func (h *Handler) RegisterProjectMaterial(c *gin.Context) {
	if h == nil || h.sink == nil || h.projects == nil {
		httpx.Fail(c, http.StatusInternalServerError, "material registration is unavailable")
		return
	}
	userID, ok := auth.UserIDFromContext(c.Request.Context())
	if !ok {
		httpx.Fail(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	projectID := strings.TrimSpace(c.Param("id"))
	if projectID == "" {
		httpx.Fail(c, http.StatusBadRequest, "project id is required")
		return
	}
	project, err := h.projects.GetProject(c.Request.Context(), userID, projectID)
	if err != nil || project == nil {
		httpx.Fail(c, http.StatusNotFound, "project not found")
		return
	}
	var request RegisterProjectMaterialRequest
	if err := decodeProjectMaterialRequest(c, &request); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid material registration request")
		return
	}
	material, artifactReq, err := BuildProjectMaterialArtifactRequest(projectID, ProjectMaterial{
		Name:        request.Name,
		Kind:        request.Kind,
		StorageRef:  request.StorageRef,
		MimeType:    request.MimeType,
		SizeBytes:   request.SizeBytes,
		ContentHash: request.ContentHash,
	})
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	record, err := h.sink.CreateArtifact(c.Request.Context(), artifactReq)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "failed to register material")
		return
	}
	httpx.OK(c, gin.H{"material": material, "artifact": record})
}

func decodeProjectMaterialRequest(c *gin.Context, target *RegisterProjectMaterialRequest) error {
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return io.ErrUnexpectedEOF
		}
		return err
	}
	return nil
}

func (h *Handler) RegisterExternalGenerationResult(c *gin.Context) {
	if h == nil || h.sink == nil {
		httpx.Fail(c, http.StatusInternalServerError, "artifact sink is required")
		return
	}
	projectID := strings.TrimSpace(c.Param("id"))
	if projectID == "" {
		httpx.Fail(c, http.StatusBadRequest, "project id is required")
		return
	}
	var req RegisterExternalGenerationResultRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	source := strings.TrimSpace(req.Source)
	if source == "" {
		source = "external_generation_upload"
	}
	manifest, err := BuildManualAssetManifest(ImportRequest{
		Type:                req.Kind,
		StorageType:         req.StorageType,
		StorageRef:          req.StorageRef,
		MimeType:            req.MimeType,
		SizeBytes:           req.SizeBytes,
		ContentHash:         req.ContentHash,
		PromptHash:          req.PromptHash,
		DurationSec:         req.DurationSec,
		Description:         req.Description,
		Tags:                req.Tags,
		RelatedShotID:       req.RelatedShotID,
		Source:              source,
		GenerationRequestID: req.GenerationRequestID,
		ExternalPlatform:    req.ExternalPlatform,
		ReferenceAssetIDs:   req.ReferenceAssetIDs,
	})
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	artifactReq, err := BuildExternalGenerationResultArtifactRequest(projectID, "", "", manifest)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	record, err := h.sink.CreateArtifact(c.Request.Context(), artifactReq)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(c, gin.H{
		"manifest": manifest,
		"artifact": record,
	})
}
