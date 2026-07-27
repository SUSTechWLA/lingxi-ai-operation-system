package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/core/auth"
	"github.com/tangying-ai/aios-core/internal/core/common/httpx"
)

const maxAckBodyBytes = 64 * 1024

type RelayRepository interface {
	Pull(context.Context, string, string, int) (EventPage, error)
	Acknowledge(context.Context, string, []string) (int64, error)
	RunSummary(context.Context, string, string) (RunSummary, error)
}

type Handler struct{ repository RelayRepository }

func NewHandler(repository RelayRepository) *Handler {
	return &Handler{repository: repository}
}

func (handler *Handler) RegisterRoutes(router *gin.Engine, requireAuth gin.HandlerFunc) {
	group := router.Group("/api/observability", requireAuth)
	group.GET("/events", handler.Pull)
	group.POST("/events/ack", handler.Acknowledge)
	group.GET("/runs/:runId/summary", handler.RunSummary)
}

func (handler *Handler) Pull(c *gin.Context) {
	userID, ok := auth.UserIDFromContext(c.Request.Context())
	if !ok {
		httpx.Fail(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	limit := defaultPullLimit
	if raw := c.Query("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			httpx.Fail(c, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = parsed
	}
	limit = normalizedPullLimit(limit)
	cursor := c.Query("cursor")
	if cursor != "" {
		if _, err := decodeCursor(cursor); err != nil {
			httpx.Fail(c, http.StatusBadRequest, "invalid cursor")
			return
		}
	}
	page, err := handler.repository.Pull(c.Request.Context(), userID, cursor, limit)
	if err != nil {
		if errors.Is(err, ErrInvalidCursor) {
			httpx.Fail(c, http.StatusBadRequest, "invalid cursor")
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, "observability relay unavailable")
		return
	}
	httpx.OK(c, page)
}

type AcknowledgeRequest struct {
	EventIDs []string `json:"eventIds"`
}

type AcknowledgeResponse struct {
	Acknowledged int64 `json:"acknowledged"`
}

func (handler *Handler) Acknowledge(c *gin.Context) {
	userID, ok := auth.UserIDFromContext(c.Request.Context())
	if !ok {
		httpx.Fail(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAckBodyBytes)
	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid acknowledgement request")
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var request AcknowledgeRequest
	if err := decoder.Decode(&request); err != nil || requireJSONEOF(decoder) != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid acknowledgement request")
		return
	}
	if err := validateAcknowledgementIDs(request.EventIDs); err != nil {
		httpx.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	count, err := handler.repository.Acknowledge(c.Request.Context(), userID, request.EventIDs)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "observability relay unavailable")
		return
	}
	httpx.OK(c, AcknowledgeResponse{Acknowledged: count})
}

func validateAcknowledgementIDs(eventIDs []string) error {
	if len(eventIDs) == 0 {
		return errors.New("eventIds must not be empty")
	}
	if len(eventIDs) > maxAckEventIDs {
		return ErrTooManyEventIDs
	}
	for _, eventID := range eventIDs {
		if !validRelayEventID(eventID) {
			return errors.New("invalid event ID")
		}
	}
	return nil
}

func (handler *Handler) RunSummary(c *gin.Context) {
	userID, ok := auth.UserIDFromContext(c.Request.Context())
	if !ok {
		httpx.Fail(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	runID := c.Param("runId")
	if !validRunID(runID) {
		httpx.Fail(c, http.StatusBadRequest, "invalid run ID")
		return
	}
	summary, err := handler.repository.RunSummary(c.Request.Context(), userID, runID)
	if errors.Is(err, ErrSummaryNotFound) {
		httpx.Fail(c, http.StatusNotFound, "run summary not found")
		return
	}
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "observability relay unavailable")
		return
	}
	httpx.OK(c, summary)
}
