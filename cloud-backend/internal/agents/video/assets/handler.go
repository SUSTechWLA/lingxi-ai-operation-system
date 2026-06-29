package assets

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/core/common/httpx"
)

type Handler struct {
	sink       ArtifactSink
	middleware []gin.HandlerFunc
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

func NewHandler(sink ArtifactSink, middleware ...gin.HandlerFunc) *Handler {
	return &Handler{sink: sink, middleware: middleware}
}

func (h *Handler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/video-projects", h.middleware...)
	api.POST("/:id/external-generation-results", h.RegisterExternalGenerationResult)
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
