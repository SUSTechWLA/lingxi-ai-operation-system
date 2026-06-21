package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestHandlerRegisterMeRefreshAndLogout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := NewService(newMemoryRepository(), NewTokenIssuer(TokenConfig{
		Secret:          "handler-secret",
		AccessTokenTTL:  time.Hour,
		RefreshTokenTTL: 30 * 24 * time.Hour,
	}))
	router := gin.New()
	NewHandler(svc).RegisterRoutes(router)

	register := postJSON(router, "/api/auth/register", `{"email":"user@example.com","password":"Password123456","nickname":"User"}`, "")
	if register.Code != http.StatusOK {
		t.Fatalf("register status = %d body=%s", register.Code, register.Body.String())
	}
	authData := decodeAuthEnvelope(t, register.Body.Bytes())
	if authData.User.Email != "user@example.com" || authData.AccessToken == "" || authData.RefreshToken == "" {
		t.Fatalf("unexpected register response: %+v", authData)
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+authData.AccessToken)
	meRec := httptest.NewRecorder()
	router.ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusOK {
		t.Fatalf("me status = %d body=%s", meRec.Code, meRec.Body.String())
	}
	userData := decodeUserEnvelope(t, meRec.Body.Bytes())
	if userData.Email != "user@example.com" {
		t.Fatalf("me email = %q", userData.Email)
	}

	refresh := postJSON(router, "/api/auth/refresh", `{"refresh_token":"`+authData.RefreshToken+`"}`, "")
	if refresh.Code != http.StatusOK {
		t.Fatalf("refresh status = %d body=%s", refresh.Code, refresh.Body.String())
	}
	refreshed := decodeAuthEnvelope(t, refresh.Body.Bytes())
	if refreshed.RefreshToken == "" || refreshed.RefreshToken == authData.RefreshToken {
		t.Fatalf("refresh should rotate token: old=%q new=%q", authData.RefreshToken, refreshed.RefreshToken)
	}

	reuse := postJSON(router, "/api/auth/refresh", `{"refresh_token":"`+authData.RefreshToken+`"}`, "")
	if reuse.Code != http.StatusUnauthorized {
		t.Fatalf("reused refresh token status = %d body=%s", reuse.Code, reuse.Body.String())
	}

	logout := postJSON(router, "/api/auth/logout", `{"refresh_token":"`+refreshed.RefreshToken+`"}`, refreshed.AccessToken)
	if logout.Code != http.StatusOK {
		t.Fatalf("logout status = %d body=%s", logout.Code, logout.Body.String())
	}

	afterLogout := postJSON(router, "/api/auth/refresh", `{"refresh_token":"`+refreshed.RefreshToken+`"}`, "")
	if afterLogout.Code != http.StatusUnauthorized {
		t.Fatalf("refresh after logout status = %d body=%s", afterLogout.Code, afterLogout.Body.String())
	}
}

func TestMiddlewareRequireAuthInjectsUserContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := NewService(newMemoryRepository(), NewTokenIssuer(TokenConfig{
		Secret:          "middleware-secret",
		AccessTokenTTL:  time.Hour,
		RefreshTokenTTL: 30 * 24 * time.Hour,
	}))
	registered, err := svc.Register(t.Context(), RegisterRequest{
		Email:    "user@example.com",
		Password: "Password123456",
	}, ClientInfo{DeviceID: "device-1"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	router := gin.New()
	middleware := NewMiddleware(svc)
	router.GET("/protected", middleware.RequireAuth(), func(c *gin.Context) {
		userID, ok := UserIDFromContext(c.Request.Context())
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "missing user"})
			return
		}
		deviceID, _ := DeviceIDFromContext(c.Request.Context())
		c.JSON(http.StatusOK, gin.H{"userId": userID, "deviceId": deviceID})
	})

	missingReq := httptest.NewRequest(http.MethodGet, "/protected", nil)
	missingRec := httptest.NewRecorder()
	router.ServeHTTP(missingRec, missingReq)
	if missingRec.Code != http.StatusUnauthorized {
		t.Fatalf("missing token status = %d body=%s", missingRec.Code, missingRec.Body.String())
	}

	validReq := httptest.NewRequest(http.MethodGet, "/protected", nil)
	validReq.Header.Set("Authorization", "Bearer "+registered.AccessToken)
	validReq.Header.Set("DeviceID", "device-1")
	validRec := httptest.NewRecorder()
	router.ServeHTTP(validRec, validReq)
	if validRec.Code != http.StatusOK {
		t.Fatalf("valid token status = %d body=%s", validRec.Code, validRec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(validRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid body: %v", err)
	}
	if body["userId"] != registered.User.ID {
		t.Fatalf("userId = %q, want %q", body["userId"], registered.User.ID)
	}
	if body["deviceId"] != "device-1" {
		t.Fatalf("deviceId = %q", body["deviceId"])
	}
}

func postJSON(router http.Handler, path string, body string, accessToken string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func decodeAuthEnvelope(t *testing.T, body []byte) AuthResponse {
	t.Helper()
	var envelope struct {
		Data AuthResponse `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode auth envelope: %v body=%s", err, string(body))
	}
	return envelope.Data
}

func decodeUserEnvelope(t *testing.T, body []byte) UserResponse {
	t.Helper()
	var envelope struct {
		Data UserResponse `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode user envelope: %v body=%s", err, string(body))
	}
	return envelope.Data
}
