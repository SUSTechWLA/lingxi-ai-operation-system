package auth

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/core/common/httpx"
)

type Handler struct {
	svc *Service
	mw  *Middleware
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc, mw: NewMiddleware(svc)}
}

func (h *Handler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/auth")
	{
		api.POST("/register", h.Register)
		api.POST("/login", h.Login)
		api.POST("/refresh", h.Refresh)
		api.POST("/logout", h.mw.RequireAuth(), h.Logout)
		api.GET("/me", h.mw.RequireAuth(), h.Me)
	}
}

func (h *Handler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	resp, err := h.svc.Register(c.Request.Context(), req, clientInfo(c))
	if err != nil {
		writeAuthError(c, err)
		return
	}
	httpx.OK(c, resp)
}

func (h *Handler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	resp, err := h.svc.Login(c.Request.Context(), req, clientInfo(c))
	if err != nil {
		writeAuthError(c, err)
		return
	}
	httpx.OK(c, resp)
}

func (h *Handler) Refresh(c *gin.Context) {
	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	resp, err := h.svc.Refresh(c.Request.Context(), req, clientInfo(c))
	if err != nil {
		writeAuthError(c, err)
		return
	}
	httpx.OK(c, resp)
}

func (h *Handler) Logout(c *gin.Context) {
	var req LogoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	userID, ok := UserIDFromContext(c.Request.Context())
	if !ok {
		httpx.Fail(c, http.StatusUnauthorized, ErrUnauthorized.Error())
		return
	}
	if err := h.svc.Logout(c.Request.Context(), userID, req.RefreshToken); err != nil {
		writeAuthError(c, err)
		return
	}
	httpx.OK(c, gin.H{"message": "logged out"})
}

func (h *Handler) Me(c *gin.Context) {
	userID, ok := UserIDFromContext(c.Request.Context())
	if !ok {
		httpx.Fail(c, http.StatusUnauthorized, ErrUnauthorized.Error())
		return
	}
	user, err := h.svc.GetUser(c.Request.Context(), userID)
	if err != nil {
		writeAuthError(c, err)
		return
	}
	httpx.OK(c, user)
}

func clientInfo(c *gin.Context) ClientInfo {
	return ClientInfo{
		DeviceID:  c.GetHeader("DeviceID"),
		UserAgent: c.GetHeader("User-Agent"),
		IPAddress: c.ClientIP(),
	}
}

func writeAuthError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrEmailAlreadyRegistered):
		httpx.Fail(c, http.StatusConflict, err.Error())
	case errors.Is(err, ErrInvalidCredentials), errors.Is(err, ErrInvalidToken), errors.Is(err, ErrRefreshTokenNotFound), errors.Is(err, ErrUnauthorized):
		httpx.Fail(c, http.StatusUnauthorized, err.Error())
	case errors.Is(err, ErrUserDisabled):
		httpx.Fail(c, http.StatusForbidden, err.Error())
	case errors.Is(err, ErrWeakPassword):
		httpx.Fail(c, http.StatusBadRequest, err.Error())
	default:
		httpx.Fail(c, http.StatusInternalServerError, err.Error())
	}
}
