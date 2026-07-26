package localagent

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tangying-ai/tangying-ai-operation-system/local-backend/internal/localmcp"
)

func TestHealthAndPathsUseLocalDataDir(t *testing.T) {
	root := t.TempDir()
	server := NewServer(Config{DataDir: root, CloudAPIBase: "https://cloud.example.com/api"})

	req := httptest.NewRequest(http.MethodGet, "/api/local/health", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("health status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var health map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &health); err != nil {
		t.Fatalf("invalid health response: %v", err)
	}
	if health["service"] != "tangying-local-agent" {
		t.Fatalf("unexpected service: %#v", health)
	}
	if health["cloudApiBase"] != "https://cloud.example.com/api" {
		t.Fatalf("cloud API base not exposed: %#v", health)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/local/paths", nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("paths status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var paths Paths
	if err := json.Unmarshal(rec.Body.Bytes(), &paths); err != nil {
		t.Fatalf("invalid paths response: %v", err)
	}
	if paths.DataDir != root {
		t.Fatalf("data dir = %q, want %q", paths.DataDir, root)
	}
	for _, dir := range []string{paths.CacheDir, paths.ProjectDir, paths.ArtifactDir, paths.LogDir, paths.DiagnosticsDir} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("expected directory %q to exist, stat=%v err=%v", dir, info, err)
		}
	}
}

func TestLocalProjectMediaServesOnlyFilesInsideProject(t *testing.T) {
	root := t.TempDir()
	server := NewServer(Config{DataDir: root})
	videoPath := filepath.Join(root, "projects", "vp-1", "renders", "final.mp4")
	if err := os.MkdirAll(filepath.Dir(videoPath), 0o755); err != nil {
		t.Fatalf("create render dir: %v", err)
	}
	if err := os.WriteFile(videoPath, []byte("demo-video"), 0o644); err != nil {
		t.Fatalf("write demo video: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/local/media?projectId=vp-1&path="+url.QueryEscape(videoPath), nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "demo-video" {
		t.Fatalf("media response status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "video/mp4" {
		t.Fatalf("media content type=%q", rec.Header().Get("Content-Type"))
	}

	outsidePath := filepath.Join(root, "outside.mp4")
	if err := os.WriteFile(outsidePath, []byte("private"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/local/media?projectId=vp-1&path="+url.QueryEscape(outsidePath), nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("outside media status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestLocalProjectMediaResolvesCanonicalArtifactStorageRef(t *testing.T) {
	root := t.TempDir()
	server := NewServer(Config{DataDir: root})
	content := []byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 'm', 'p', '4', '2'}
	storageRef := "local://projects/vp-1/artifacts/video-1/hash/final.mp4"
	body := bytes.NewBufferString(`{
		"id":"video-1",
		"projectId":"vp-1",
		"storageRef":"` + storageRef + `",
		"mimeType":"video/mp4",
		"contentBase64":"` + base64.StdEncoding.EncodeToString(content) + `"
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/local/artifacts", body)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("store status = %d, body = %s", rec.Code, rec.Body.String())
	}

	mediaURL := "/api/local/media?projectId=vp-1&storageRef=" + url.QueryEscape(storageRef)
	req = httptest.NewRequest(http.MethodHead, mediaURL, nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("HEAD media status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "video/mp4" {
		t.Fatalf("HEAD content type = %q, want video/mp4", got)
	}
	if got := rec.Header().Get("Accept-Ranges"); got != "bytes" {
		t.Fatalf("HEAD accept ranges = %q, want bytes", got)
	}
	if got := rec.Header().Get("Content-Length"); got != "12" {
		t.Fatalf("HEAD content length = %q, want 12", got)
	}

	req = httptest.NewRequest(http.MethodGet, mediaURL, nil)
	req.Header.Set("Range", "bytes=0-3")
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusPartialContent {
		t.Fatalf("range status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !bytes.Equal(rec.Body.Bytes(), content[:4]) {
		t.Fatalf("range content = %v, want %v", rec.Body.Bytes(), content[:4])
	}

	mismatchedURL := "/api/local/media?projectId=vp-2&storageRef=" + url.QueryEscape(storageRef)
	req = httptest.NewRequest(http.MethodHead, mismatchedURL, nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("mismatched project status = %d, body = %s", rec.Code, rec.Body.String())
	}

	unknownRef := "local://projects/vp-1/artifacts/missing/hash/final.mp4"
	req = httptest.NewRequest(http.MethodHead, "/api/local/media?projectId=vp-1&storageRef="+url.QueryEscape(unknownRef), nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown artifact status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestLocalProjectMediaRejectsSymlinkEscapesAndAmbiguousReferences(t *testing.T) {
	root := t.TempDir()
	server := NewServer(Config{DataDir: root})
	outsidePath := filepath.Join(root, "outside-secret.mp4")
	if err := os.WriteFile(outsidePath, []byte("outside-secret"), 0o644); err != nil {
		t.Fatalf("write outside media: %v", err)
	}

	renderPath := filepath.Join(root, "projects", "vp-1", "renders", "final.mp4")
	if err := os.MkdirAll(filepath.Dir(renderPath), 0o755); err != nil {
		t.Fatalf("create render dir: %v", err)
	}
	if err := os.Symlink(outsidePath, renderPath); err != nil {
		t.Fatalf("create render symlink: %v", err)
	}
	renderRef := "local://projects/vp-1/renders/final.mp4"
	req := httptest.NewRequest(http.MethodGet, "/api/local/media?projectId=vp-1&storageRef="+url.QueryEscape(renderRef), nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	assertRejectedLocalMediaDoesNotLeak(t, rec, outsidePath, "outside-secret")

	artifactDir := filepath.Join(root, "artifacts", "vp-1", "video-1")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatalf("create artifact dir: %v", err)
	}
	if err := os.Symlink(outsidePath, filepath.Join(artifactDir, "content")); err != nil {
		t.Fatalf("create artifact content symlink: %v", err)
	}
	if err := os.WriteFile(filepath.Join(artifactDir, "metadata.json"), []byte(`{"mimeType":"video/mp4"}`), 0o644); err != nil {
		t.Fatalf("write artifact metadata: %v", err)
	}
	artifactRef := "local://projects/vp-1/artifacts/video-1/hash/final.mp4"
	req = httptest.NewRequest(http.MethodGet, "/api/local/media?projectId=vp-1&storageRef="+url.QueryEscape(artifactRef), nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	assertRejectedLocalMediaDoesNotLeak(t, rec, outsidePath, "outside-secret")

	encodedTraversalRef := "local://projects/vp-1/renders/%2e%2e/outside-secret.mp4"
	req = httptest.NewRequest(http.MethodGet, "/api/local/media?projectId=vp-1&storageRef="+url.QueryEscape(encodedTraversalRef), nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("encoded traversal status = %d, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(
		http.MethodGet,
		"/api/local/media?projectId=vp-1&path="+url.QueryEscape(renderPath)+"&storageRef="+url.QueryEscape(renderRef),
		nil,
	)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("simultaneous reference status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestLocalProjectMediaRejectsCrossProjectSymlinks(t *testing.T) {
	root := t.TempDir()
	server := NewServer(Config{DataDir: root})

	vp2RenderDir := filepath.Join(root, "projects", "vp-2", "renders")
	if err := os.MkdirAll(vp2RenderDir, 0o755); err != nil {
		t.Fatalf("create vp-2 render dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vp2RenderDir, "final.mp4"), []byte("vp-2-render-secret"), 0o644); err != nil {
		t.Fatalf("write vp-2 render: %v", err)
	}
	vp1ProjectDir := filepath.Join(root, "projects", "vp-1")
	if err := os.MkdirAll(vp1ProjectDir, 0o755); err != nil {
		t.Fatalf("create vp-1 project dir: %v", err)
	}
	if err := os.Symlink(filepath.Join("..", "vp-2", "renders"), filepath.Join(vp1ProjectDir, "renders")); err != nil {
		t.Fatalf("create cross-project render symlink: %v", err)
	}
	renderRef := "local://projects/vp-1/renders/final.mp4"
	req := httptest.NewRequest(http.MethodGet, "/api/local/media?projectId=vp-1&storageRef="+url.QueryEscape(renderRef), nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	assertRejectedLocalMediaDoesNotLeak(t, rec, vp2RenderDir, "vp-2-render-secret")

	vp2ArtifactDir := filepath.Join(root, "artifacts", "vp-2", "video-2")
	if err := os.MkdirAll(vp2ArtifactDir, 0o755); err != nil {
		t.Fatalf("create vp-2 artifact dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vp2ArtifactDir, "content"), []byte("vp-2-artifact-secret"), 0o644); err != nil {
		t.Fatalf("write vp-2 artifact content: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vp2ArtifactDir, "metadata.json"), []byte(`{"mimeType":"video/vp-2-secret"}`), 0o644); err != nil {
		t.Fatalf("write vp-2 artifact metadata: %v", err)
	}

	vp1ArtifactDir := filepath.Join(root, "artifacts", "vp-1", "video-1")
	if err := os.MkdirAll(vp1ArtifactDir, 0o755); err != nil {
		t.Fatalf("create vp-1 artifact dir: %v", err)
	}
	if err := os.Symlink(filepath.Join("..", "..", "vp-2", "video-2", "content"), filepath.Join(vp1ArtifactDir, "content")); err != nil {
		t.Fatalf("create cross-project artifact content symlink: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vp1ArtifactDir, "metadata.json"), []byte(`{"mimeType":"video/mp4"}`), 0o644); err != nil {
		t.Fatalf("write vp-1 artifact metadata: %v", err)
	}
	artifactRef := "local://projects/vp-1/artifacts/video-1/hash/final.mp4"
	req = httptest.NewRequest(http.MethodGet, "/api/local/media?projectId=vp-1&storageRef="+url.QueryEscape(artifactRef), nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	assertRejectedLocalMediaDoesNotLeak(t, rec, vp2ArtifactDir, "vp-2-artifact-secret")

	metadataArtifactDir := filepath.Join(root, "artifacts", "vp-1", "video-metadata")
	if err := os.MkdirAll(metadataArtifactDir, 0o755); err != nil {
		t.Fatalf("create metadata artifact dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(metadataArtifactDir, "content"), []byte("vp-1-content"), 0o644); err != nil {
		t.Fatalf("write metadata artifact content: %v", err)
	}
	if err := os.Symlink(filepath.Join("..", "..", "vp-2", "video-2", "metadata.json"), filepath.Join(metadataArtifactDir, "metadata.json")); err != nil {
		t.Fatalf("create cross-project metadata symlink: %v", err)
	}
	metadataRef := "local://projects/vp-1/artifacts/video-metadata/hash/final.mp4"
	req = httptest.NewRequest(http.MethodGet, "/api/local/media?projectId=vp-1&storageRef="+url.QueryEscape(metadataRef), nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	assertRejectedLocalMediaDoesNotLeak(t, rec, vp2ArtifactDir, "video/vp-2-secret", "vp-1-content")

	vp1ArtifactProjectDir := filepath.Join(root, "artifacts", "vp-1")
	if err := os.Symlink(filepath.Join("..", "vp-2", "video-2"), filepath.Join(vp1ArtifactProjectDir, "video-link")); err != nil {
		t.Fatalf("create artifact directory symlink: %v", err)
	}
	intermediateRef := "local://projects/vp-1/artifacts/video-link/hash/final.mp4"
	req = httptest.NewRequest(http.MethodGet, "/api/local/media?projectId=vp-1&storageRef="+url.QueryEscape(intermediateRef), nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	assertRejectedLocalMediaDoesNotLeak(t, rec, vp2ArtifactDir, "vp-2-artifact-secret", "video/vp-2-secret")
}

func TestLocalProjectMediaRejectsSymlinkedCategoryDirectories(t *testing.T) {
	t.Run("projects", func(t *testing.T) {
		dataDir := t.TempDir()
		outsideProjects := t.TempDir()
		renderPath := filepath.Join(outsideProjects, "vp-1", "renders", "final.mp4")
		if err := os.MkdirAll(filepath.Dir(renderPath), 0o755); err != nil {
			t.Fatalf("create outside render dir: %v", err)
		}
		if err := os.WriteFile(renderPath, []byte("outside-project-category-secret"), 0o644); err != nil {
			t.Fatalf("write outside render: %v", err)
		}
		if err := os.Symlink(outsideProjects, filepath.Join(dataDir, "projects")); err != nil {
			t.Fatalf("create projects category symlink: %v", err)
		}

		server := NewServer(Config{DataDir: dataDir})
		renderRef := "local://projects/vp-1/renders/final.mp4"
		req := httptest.NewRequest(http.MethodGet, "/api/local/media?projectId=vp-1&storageRef="+url.QueryEscape(renderRef), nil)
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		assertRejectedLocalMediaDoesNotLeak(t, rec, outsideProjects, "outside-project-category-secret")
	})

	t.Run("artifacts", func(t *testing.T) {
		dataDir := t.TempDir()
		outsideArtifacts := t.TempDir()
		artifactDir := filepath.Join(outsideArtifacts, "vp-1", "video-1")
		if err := os.MkdirAll(artifactDir, 0o755); err != nil {
			t.Fatalf("create outside artifact dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(artifactDir, "content"), []byte("outside-artifact-category-secret"), 0o644); err != nil {
			t.Fatalf("write outside artifact content: %v", err)
		}
		if err := os.WriteFile(filepath.Join(artifactDir, "metadata.json"), []byte(`{"mimeType":"video/outside-secret"}`), 0o644); err != nil {
			t.Fatalf("write outside artifact metadata: %v", err)
		}
		if err := os.Symlink(outsideArtifacts, filepath.Join(dataDir, "artifacts")); err != nil {
			t.Fatalf("create artifacts category symlink: %v", err)
		}

		server := NewServer(Config{DataDir: dataDir})
		artifactRef := "local://projects/vp-1/artifacts/video-1/hash/final.mp4"
		req := httptest.NewRequest(http.MethodGet, "/api/local/media?projectId=vp-1&storageRef="+url.QueryEscape(artifactRef), nil)
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		assertRejectedLocalMediaDoesNotLeak(t, rec, outsideArtifacts, "outside-artifact-category-secret", "video/outside-secret")
	})
}

func TestLocalProjectMediaRejectsCategoryIdentitySwapWhileOpening(t *testing.T) {
	dataDir := t.TempDir()
	projectsDir := filepath.Join(dataDir, "projects")
	if err := os.MkdirAll(filepath.Join(projectsDir, "vp-1", "renders"), 0o755); err != nil {
		t.Fatalf("create original project render dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectsDir, "vp-1", "renders", "final.mp4"), []byte("original-project-content"), 0o644); err != nil {
		t.Fatalf("write original project render: %v", err)
	}

	replacementProjects := filepath.Join(dataDir, "projects-replacement")
	if err := os.MkdirAll(filepath.Join(replacementProjects, "vp-1", "renders"), 0o755); err != nil {
		t.Fatalf("create replacement project render dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(replacementProjects, "vp-1", "renders", "final.mp4"), []byte("replacement-category-secret"), 0o644); err != nil {
		t.Fatalf("write replacement project render: %v", err)
	}

	server := NewServer(Config{DataDir: dataDir})
	var categorySwapErr error
	categorySwapped := false
	server.localMediaIdentityHook = func(stage, name string) {
		if categorySwapped || stage != "before_open_root_component" || name != "projects" {
			return
		}
		categorySwapped = true
		if err := os.Rename(projectsDir, filepath.Join(dataDir, "projects-original")); err != nil {
			categorySwapErr = err
			return
		}
		categorySwapErr = os.Symlink("projects-replacement", projectsDir)
	}

	renderRef := "local://projects/vp-1/renders/final.mp4"
	req := httptest.NewRequest(http.MethodGet, "/api/local/media?projectId=vp-1&storageRef="+url.QueryEscape(renderRef), nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if categorySwapErr != nil {
		t.Fatalf("swap projects category identity: %v", categorySwapErr)
	}
	if !categorySwapped {
		t.Fatal("projects category identity hook was not called")
	}
	assertRejectedLocalMediaDoesNotLeak(t, rec, replacementProjects, "replacement-category-secret")
}

func TestLocalProjectMediaRejectsIdentitySwapsWhileOpening(t *testing.T) {
	root := t.TempDir()
	server := NewServer(Config{DataDir: root})

	vp1Dir := filepath.Join(root, "projects", "vp-1")
	vp2Dir := filepath.Join(root, "projects", "vp-2")
	if err := os.MkdirAll(filepath.Join(vp1Dir, "renders"), 0o755); err != nil {
		t.Fatalf("create vp-1 render dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vp1Dir, "renders", "final.mp4"), []byte("vp-1-original"), 0o644); err != nil {
		t.Fatalf("write vp-1 render: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(vp2Dir, "renders"), 0o755); err != nil {
		t.Fatalf("create vp-2 render dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vp2Dir, "renders", "final.mp4"), []byte("vp-2-swap-secret"), 0o644); err != nil {
		t.Fatalf("write vp-2 render: %v", err)
	}

	var rootSwapErr error
	rootSwapped := false
	server.localMediaIdentityHook = func(stage, name string) {
		if rootSwapped || stage != "before_open_root_component" || name != "vp-1" {
			return
		}
		rootSwapped = true
		original := filepath.Join(root, "projects", "vp-1-original")
		if err := os.Rename(vp1Dir, original); err != nil {
			rootSwapErr = err
			return
		}
		rootSwapErr = os.Symlink("vp-2", vp1Dir)
	}
	renderRef := "local://projects/vp-1/renders/final.mp4"
	req := httptest.NewRequest(http.MethodGet, "/api/local/media?projectId=vp-1&storageRef="+url.QueryEscape(renderRef), nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rootSwapErr != nil {
		t.Fatalf("swap project root identity: %v", rootSwapErr)
	}
	if !rootSwapped {
		t.Fatal("project root identity hook was not called")
	}
	assertRejectedLocalMediaDoesNotLeak(t, rec, vp2Dir, "vp-2-swap-secret")

	artifactDir := filepath.Join(root, "artifacts", "vp-file", "video-1")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatalf("create artifact dir: %v", err)
	}
	contentPath := filepath.Join(artifactDir, "content")
	if err := os.WriteFile(contentPath, []byte("original-content"), 0o644); err != nil {
		t.Fatalf("write original content: %v", err)
	}
	replacementPath := filepath.Join(artifactDir, "replacement")
	if err := os.WriteFile(replacementPath, []byte("replacement-secret"), 0o644); err != nil {
		t.Fatalf("write replacement content: %v", err)
	}
	if err := os.WriteFile(filepath.Join(artifactDir, "metadata.json"), []byte(`{"mimeType":"video/mp4"}`), 0o644); err != nil {
		t.Fatalf("write artifact metadata: %v", err)
	}

	var fileSwapErr error
	fileSwapped := false
	server.localMediaIdentityHook = func(stage, name string) {
		if fileSwapped || stage != "before_open_regular_file" || name != "content" {
			return
		}
		fileSwapped = true
		if err := os.Rename(contentPath, filepath.Join(artifactDir, "content-original")); err != nil {
			fileSwapErr = err
			return
		}
		fileSwapErr = os.Rename(replacementPath, contentPath)
	}
	artifactRef := "local://projects/vp-file/artifacts/video-1/hash/final.mp4"
	req = httptest.NewRequest(http.MethodGet, "/api/local/media?projectId=vp-file&storageRef="+url.QueryEscape(artifactRef), nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if fileSwapErr != nil {
		t.Fatalf("swap content file identity: %v", fileSwapErr)
	}
	if !fileSwapped {
		t.Fatal("regular file identity hook was not called")
	}
	assertRejectedLocalMediaDoesNotLeak(t, rec, replacementPath, "replacement-secret")
}

func assertRejectedLocalMediaDoesNotLeak(t *testing.T, rec *httptest.ResponseRecorder, forbidden ...string) {
	t.Helper()
	if rec.Code == http.StatusOK {
		t.Fatalf("symlink escape unexpectedly served scoped media: %q", rec.Body.String())
	}
	for _, value := range forbidden {
		if strings.Contains(rec.Body.String(), value) {
			t.Fatalf("rejected media response leaked forbidden target %q: %s", value, rec.Body.String())
		}
	}
}

func TestLocalArtifactStoreSupportsBinaryPayloads(t *testing.T) {
	root := t.TempDir()
	server := NewServer(Config{DataDir: root})
	payload := []byte{0x00, 0x01, 0x02, 0xff}
	body := bytes.NewBufferString(`{
		"id":"img-1",
		"projectId":"vp-1",
		"storageRef":"local://projects/vp-1/artifacts/keyframe/image/hash/keyframe.png",
		"mimeType":"image/png",
		"contentBase64":"` + base64.StdEncoding.EncodeToString(payload) + `",
		"metadata":{"kind":"IMAGE","cloudPayloadStored":false}
	}`)

	req := httptest.NewRequest(http.MethodPost, "/api/local/artifacts", body)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("store status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var stored LocalArtifactResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &stored); err != nil {
		t.Fatalf("invalid store response: %v", err)
	}
	content, err := os.ReadFile(stored.Path)
	if err != nil {
		t.Fatalf("expected local binary artifact: %v", err)
	}
	if !bytes.Equal(content, payload) {
		t.Fatalf("binary payload mismatch: %#v", content)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/local/artifacts/img-1?projectId=vp-1", nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("read status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var loaded LocalArtifactResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &loaded); err != nil {
		t.Fatalf("invalid read response: %v", err)
	}
	if loaded.Content != "" {
		t.Fatalf("binary response should not use text content: %q", loaded.Content)
	}
	if loaded.ContentBase64 != base64.StdEncoding.EncodeToString(payload) {
		t.Fatalf("binary response base64 mismatch: %q", loaded.ContentBase64)
	}
}

func TestLocalArtifactStoreAcceptsMultipartUpload(t *testing.T) {
	root := t.TempDir()
	server := NewServer(Config{DataDir: root})
	payload := []byte{0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p', 'm', 'p', '4', '2'}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("id", "extgen-video-1"); err != nil {
		t.Fatalf("write id field: %v", err)
	}
	if err := writer.WriteField("projectId", "vp-1"); err != nil {
		t.Fatalf("write project field: %v", err)
	}
	if err := writer.WriteField("mimeType", "video/mp4"); err != nil {
		t.Fatalf("write mime field: %v", err)
	}
	if err := writer.WriteField("metadata", `{"artifactType":"external_generation_result","externalGenerationRequestId":"extgen_123","cloudPayloadStored":false}`); err != nil {
		t.Fatalf("write metadata field: %v", err)
	}
	part, err := writer.CreateFormFile("file", "shot-1.mp4")
	if err != nil {
		t.Fatalf("create upload part: %v", err)
	}
	if _, err := part.Write(payload); err != nil {
		t.Fatalf("write upload payload: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/local/artifacts", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("upload status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var stored LocalArtifactResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &stored); err != nil {
		t.Fatalf("invalid upload response: %v", err)
	}
	if !strings.HasPrefix(stored.StorageRef, "local://projects/vp-1/artifacts/extgen-video-1/") {
		t.Fatalf("unexpected storageRef: %q", stored.StorageRef)
	}
	if !strings.HasPrefix(stored.ContentHash, "sha256:") {
		t.Fatalf("expected sha256 content hash, got %q", stored.ContentHash)
	}
	if stored.SizeBytes != int64(len(payload)) {
		t.Fatalf("sizeBytes = %d, want %d", stored.SizeBytes, len(payload))
	}
	content, err := os.ReadFile(stored.Path)
	if err != nil {
		t.Fatalf("expected uploaded artifact payload: %v", err)
	}
	if !bytes.Equal(content, payload) {
		t.Fatalf("uploaded payload mismatch: %#v", content)
	}
	if stored.Metadata["externalGenerationRequestId"] != "extgen_123" {
		t.Fatalf("metadata should preserve request link: %+v", stored.Metadata)
	}
	if stored.Metadata["contentHash"] != stored.ContentHash {
		t.Fatalf("metadata should include returned hash: %+v", stored.Metadata)
	}
	if stored.Metadata["sourceType"] != "uploaded" || stored.Metadata["providerName"] != "local-upload" || stored.Metadata["isFallback"] != false {
		t.Fatalf("uploaded artifact should get default provenance fields: %+v", stored.Metadata)
	}
	provenance, ok := stored.Metadata["provenance"].(map[string]interface{})
	if !ok {
		t.Fatalf("uploaded artifact should include nested provenance: %+v", stored.Metadata)
	}
	for _, key := range []string{"schemaVersion", "sourceType", "providerName", "providerJobId", "fallbackReason", "isFallback", "generatedAt", "inputPromptHash", "sourceArtifactIds"} {
		if _, exists := provenance[key]; !exists {
			t.Fatalf("uploaded provenance missing %s: %+v", key, provenance)
		}
	}
	if provenance["sourceType"] != "uploaded" || provenance["providerName"] != "local-upload" {
		t.Fatalf("unexpected uploaded provenance: %+v", provenance)
	}
}

func TestModelProviderSettingsSaveListAndPreserveSecrets(t *testing.T) {
	root := t.TempDir()
	server := NewServer(Config{DataDir: root})
	body := bytes.NewBufferString(`{
		"providers":{
			"text_to_text":{"baseUrl":"https://api.openai.com/v1","model":"gpt-4.1","apiKey":"sk-text-secret"},
			"text_to_image":{"baseUrl":"https://api.example.com/v1","model":"gpt-image-1","apiKey":"sk-image-secret"},
			"text_to_video":{"baseUrl":"https://video.example.com/v1","model":"seedance-v1","apiKey":"sk-video-secret"}
		}
	}`)

	req := httptest.NewRequest(http.MethodPut, "/api/local/model-providers", body)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("save status = %d, body = %s", rec.Code, rec.Body.String())
	}

	configFile := filepath.Join(root, "config", "model-providers.json")
	raw, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("expected provider config file: %v", err)
	}
	if !bytes.Contains(raw, []byte("sk-text-secret")) {
		t.Fatalf("local config should store the full local token")
	}

	req = httptest.NewRequest(http.MethodGet, "/api/local/model-providers", nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("sk-text-secret")) || bytes.Contains(rec.Body.Bytes(), []byte("sk-image-secret")) {
		t.Fatalf("provider settings GET should not expose full api keys: %s", rec.Body.String())
	}
	var listed ModelProviderSettingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("invalid provider response: %v", err)
	}
	if !listed.Providers[CapabilityTextToText].HasAPIKey {
		t.Fatalf("text provider should report existing api key: %+v", listed.Providers[CapabilityTextToText])
	}
	if listed.Providers[CapabilityTextToText].APIKeyPreview == "" {
		t.Fatalf("text provider should include masked key preview")
	}

	body = bytes.NewBufferString(`{
		"providers":{
			"text_to_text":{"baseUrl":"https://proxy.example.com/v1","model":"gpt-4.1-mini","apiKey":""}
		}
	}`)
	req = httptest.NewRequest(http.MethodPut, "/api/local/model-providers", body)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", rec.Code, rec.Body.String())
	}
	raw, err = os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("read updated config: %v", err)
	}
	if !bytes.Contains(raw, []byte("sk-text-secret")) {
		t.Fatalf("blank apiKey should preserve existing local token: %s", string(raw))
	}
	if !bytes.Contains(raw, []byte("https://proxy.example.com/v1")) {
		t.Fatalf("baseUrl should be updated: %s", string(raw))
	}
}

func TestModelProviderSettingsClearAPIKey(t *testing.T) {
	root := t.TempDir()
	server := NewServer(Config{DataDir: root})
	body := bytes.NewBufferString(`{
		"providers":{
			"text_to_text":{"apiKey":"sk-text-secret"},
			"text_to_image":{"apiKey":"sk-image-secret"}
		}
	}`)

	req := httptest.NewRequest(http.MethodPut, "/api/local/model-providers", body)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("save status = %d, body = %s", rec.Code, rec.Body.String())
	}

	body = bytes.NewBufferString(`{
		"providers":{
			"text_to_text":{"apiKey":"","clearApiKey":true}
		}
	}`)
	req = httptest.NewRequest(http.MethodPut, "/api/local/model-providers", body)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("clear status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("clearApiKey")) {
		t.Fatalf("response must not include clearApiKey control field: %s", rec.Body.String())
	}
	var updated ModelProviderSettingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("invalid clear response: %v", err)
	}
	if updated.Providers[CapabilityTextToText].HasAPIKey {
		t.Fatalf("cleared provider should report no api key: %+v", updated.Providers[CapabilityTextToText])
	}

	raw, err := os.ReadFile(filepath.Join(root, "config", "model-providers.json"))
	if err != nil {
		t.Fatalf("read updated config: %v", err)
	}
	if bytes.Contains(raw, []byte("sk-text-secret")) {
		t.Fatalf("cleared api key should not remain in local config: %s", string(raw))
	}
	if !bytes.Contains(raw, []byte("sk-image-secret")) {
		t.Fatalf("clearing one capability should preserve other api keys: %s", string(raw))
	}
	if bytes.Contains(raw, []byte("clearApiKey")) {
		t.Fatalf("local config must not persist clearApiKey control field: %s", string(raw))
	}
}

func TestModelProviderSettingsIncludeKeyRequiresNonBrowserLocalRequest(t *testing.T) {
	root := t.TempDir()
	server := NewServer(Config{DataDir: root})
	body := bytes.NewBufferString(`{
		"providers":{
			"text_to_text":{"baseUrl":"https://api.openai.com/v1","model":"gpt-4.1","apiKey":"sk-text-secret"}
		}
	}`)

	req := httptest.NewRequest(http.MethodPut, "/api/local/model-providers", body)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("save status = %d, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/local/model-providers?include_key=true", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("local include_key status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("sk-text-secret")) {
		t.Fatalf("non-browser localhost request should include full key: %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/local/model-providers?include_key=true", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	req.Header.Set("Origin", "http://localhost:3000")
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("browser include_key status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("sk-text-secret")) {
		t.Fatalf("browser-origin request must not expose full api key: %s", rec.Body.String())
	}
}

type fakeAgentCommandRunner struct {
	calls []fakeAgentCommandCall
	out   CommandOutput
	err   error
}

type fakeAgentCommandCall struct {
	name string
	args []string
}

func (r *fakeAgentCommandRunner) Run(_ context.Context, name string, args ...string) (CommandOutput, error) {
	r.calls = append(r.calls, fakeAgentCommandCall{name: name, args: append([]string(nil), args...)})
	return r.out, r.err
}

func TestReadMCPProvidersBootstrapsBundledIPAvatarWhenConfigIsMissing(t *testing.T) {
	root := t.TempDir()
	script := filepath.Join(root, "repo", "mcp", "ip_avatar_3d", "server.py")
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatalf("mkdir MCP script dir: %v", err)
	}
	if err := os.WriteFile(script, []byte("# test MCP server\n"), 0o644); err != nil {
		t.Fatalf("write MCP script: %v", err)
	}
	t.Setenv("TANGYING_IP_AVATAR_MCP_SCRIPT", script)

	server := NewServer(Config{DataDir: filepath.Join(root, "data")})
	providers, err := server.ReadMCPProviders()
	if err != nil {
		t.Fatalf("ReadMCPProviders: %v", err)
	}
	if len(providers) != 1 {
		t.Fatalf("providers = %#v, want bundled IP avatar provider", providers)
	}
	provider := providers[0]
	if provider.ID != "ip_avatar_3d" || provider.Transport != "stdio" || provider.ToolPrefix != "ip_avatar_3d." || !provider.Enabled {
		t.Fatalf("unexpected IP avatar provider: %+v", provider)
	}
	if len(provider.Args) != 1 || provider.Args[0] != script || provider.TimeoutSec != 3600 {
		t.Fatalf("unexpected IP avatar command contract: %+v", provider)
	}
}

func TestReadMCPProvidersAddsBundledIPAvatarToExistingConfig(t *testing.T) {
	root := t.TempDir()
	script := filepath.Join(root, "repo", "mcp", "ip_avatar_3d", "server.py")
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatalf("mkdir MCP script dir: %v", err)
	}
	if err := os.WriteFile(script, []byte("# test MCP server\n"), 0o644); err != nil {
		t.Fatalf("write MCP script: %v", err)
	}
	t.Setenv("TANGYING_IP_AVATAR_MCP_SCRIPT", script)
	configDir := filepath.Join(root, "data", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "mcp-providers.json"), []byte(`{
  "providers": [{"id":"jimeng","label":"JiMeng","transport":"stdio","command":"python3","args":["jimeng.py"],"enabled":false}]
}`), 0o600); err != nil {
		t.Fatalf("write provider config: %v", err)
	}

	server := NewServer(Config{DataDir: filepath.Join(root, "data")})
	providers, err := server.ReadMCPProviders()
	if err != nil {
		t.Fatalf("ReadMCPProviders: %v", err)
	}
	if len(providers) != 2 {
		t.Fatalf("providers = %#v, want existing plus bundled IP avatar", providers)
	}
	provider, ok := localMCPProviderByID(providers, "ip_avatar_3d")
	if !ok || !provider.Enabled || len(provider.Args) != 1 || provider.Args[0] != script {
		t.Fatalf("bundled IP avatar provider = %+v, found=%v", provider, ok)
	}
}

func TestLocalMCPProviderSettingsSaveAndStatus(t *testing.T) {
	root := t.TempDir()
	mcp := newMCPProtocolTestServer(t, func(toolName string, _ map[string]interface{}) map[string]interface{} {
		t.Fatalf("unexpected tool call during discovery: %s", toolName)
		return nil
	}, "jimeng.generate_video")
	defer mcp.Close()

	server := NewServer(Config{DataDir: root})
	body := bytes.NewBufferString(`{"providers":[{"id":"jimeng","label":"JiMeng","endpoint":"` + mcp.URL + `","enabled":true}]}`)
	req := httptest.NewRequest(http.MethodPut, "/api/local/mcp-providers", body)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("save status = %d, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/local/mcp-providers/status", nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, body = %s", rec.Code, rec.Body.String())
	}
	var status LocalMCPProviderStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	var jimengStatus *LocalMCPProviderStatus
	for index := range status.Providers {
		if status.Providers[index].ID == "jimeng" {
			jimengStatus = &status.Providers[index]
			break
		}
	}
	if jimengStatus == nil {
		t.Fatalf("saved JiMeng provider is missing from status: %+v", status.Providers)
	}
	if !jimengStatus.Reachable {
		t.Fatalf("provider should be reachable: %+v", jimengStatus)
	}
	if len(jimengStatus.Tools) != 1 || jimengStatus.Tools[0].Name != "jimeng.generate_video" {
		t.Fatalf("tools = %+v", jimengStatus.Tools)
	}
}

func TestMCPProviderStatusBoundsHangingSessionClose(t *testing.T) {
	t.Setenv("TANGYING_IP_AVATAR_MCP_SCRIPT", filepath.Join(t.TempDir(), "missing.py"))
	sdkServer := mcp.NewServer(&mcp.Implementation{Name: "slow-close", Version: "1.0.0"}, nil)
	sdkServer.AddTool(&mcp.Tool{Name: "healthy", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{}, nil
	})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return sdkServer }, nil)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(10 * time.Second):
				w.WriteHeader(http.StatusOK)
				return
			}
		}
		handler.ServeHTTP(w, r)
	}))
	defer httpServer.Close()

	server := NewServer(Config{DataDir: t.TempDir()})
	if err := server.writeMCPProviders([]localmcp.ProviderConfig{{ID: "slow-close", Endpoint: httpServer.URL, Enabled: true}}); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	statuses, err := server.mcpProviderStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > 1500*time.Millisecond {
		t.Fatalf("provider status hung during session close: %v", elapsed)
	}
	var slowCloseStatus *LocalMCPProviderStatus
	for index := range statuses {
		if statuses[index].ID == "slow-close" {
			slowCloseStatus = &statuses[index]
			break
		}
	}
	if slowCloseStatus == nil || slowCloseStatus.Error == "" {
		t.Fatalf("bounded close failure should be reported: %+v", statuses)
	}
}

func TestLocalMCPProviderSettingsAcceptsStandardStdioProvider(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "mcp-providers.json")
	if err := os.WriteFile(configPath, []byte(`{"providers":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(configPath, 0o644); err != nil {
		t.Fatal(err)
	}
	server := NewServer(Config{DataDir: root})
	body := bytes.NewBufferString(`{
		"providers":[{
			"id":"echo",
			"label":"Echo MCP",
				"transport":"stdio",
				"command":"python3",
				"args":["/opt/mcp/echo_server.py"],
				"env":{"ECHO_MODE":"test"},
				"headers":{" Authorization ":"Bearer test-token"},
				"toolPrefix":"echo.",
				"toolNameMap":{"echo.health":"health"},
				"approvalMode":" BEFORE_EXECUTE ",
				"enabled":true
			}]
	}`)

	req := httptest.NewRequest(http.MethodPut, "/api/local/mcp-providers", body)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("save stdio provider status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var listed LocalMCPProviderSettingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("invalid mcp provider response: %v", err)
	}
	if len(listed.Providers) != 1 {
		t.Fatalf("providers = %#v, want one", listed.Providers)
	}
	provider := listed.Providers[0]
	if provider.Transport != "stdio" || provider.Command != "python3" || provider.Endpoint != "" {
		t.Fatalf("stdio provider not normalized as expected: %+v", provider)
	}
	if len(provider.Args) != 1 || provider.Args[0] != "/opt/mcp/echo_server.py" {
		t.Fatalf("stdio args not preserved: %+v", provider.Args)
	}
	if provider.Env != nil || !provider.HasEnv || len(provider.EnvKeys) != 1 || provider.EnvKeys[0] != "ECHO_MODE" {
		t.Fatalf("provider response should expose only environment metadata: %+v", provider)
	}
	if provider.Headers != nil || !provider.HasHeaders || len(provider.HeaderKeys) != 1 || provider.HeaderKeys[0] != "Authorization" {
		t.Fatalf("provider response should expose only header metadata: %+v", provider)
	}
	if strings.Contains(rec.Body.String(), "Bearer test-token") || strings.Contains(rec.Body.String(), `"ECHO_MODE":"test"`) {
		t.Fatalf("provider response leaked a configured secret: %s", rec.Body.String())
	}
	stored, err := server.ReadMCPProviders()
	if err != nil {
		t.Fatalf("read stored providers: %v", err)
	}
	storedProvider, ok := localMCPProviderByID(stored, "echo")
	if !ok || storedProvider.Headers["Authorization"] != "Bearer test-token" || storedProvider.Env["ECHO_MODE"] != "test" {
		t.Fatalf("provider secrets were not persisted internally: %+v", storedProvider)
	}
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("stat MCP provider config: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("MCP provider config mode = %o, want 600", got)
	}
	if provider.ToolPrefix != "echo." || provider.ToolNameMap["echo.health"] != "health" {
		t.Fatalf("stdio tool mapping not preserved: %+v", provider)
	}
	if provider.ApprovalMode != "before_execute" || storedProvider.ApprovalMode != "before_execute" {
		t.Fatalf("approval mode not normalized and persisted: response=%q stored=%q", provider.ApprovalMode, storedProvider.ApprovalMode)
	}
}

func TestLocalMCPProviderSettingsRejectsInvalidApprovalModeWithoutPersisting(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "mcp-providers.json")
	if err := os.WriteFile(configPath, []byte(`{
		"providers":[{
			"id":"existing",
			"transport":"stdio",
			"command":"python3",
			"approvalMode":"none",
			"enabled":true
		}]
	}`), 0o600); err != nil {
		t.Fatal(err)
	}

	server := NewServer(Config{DataDir: root})
	req := httptest.NewRequest(http.MethodPut, "/api/local/mcp-providers", bytes.NewBufferString(`{
		"providers":[{
			"id":"unsafe",
			"transport":"stdio",
			"command":"python3",
			"approvalMode":"after_artifact",
			"enabled":true
		}]
	}`))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid approval mode status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "approvalMode") {
		t.Fatalf("invalid approval mode error must identify approvalMode: %s", rec.Body.String())
	}

	providers, err := server.ReadMCPProviders()
	if err != nil {
		t.Fatalf("read providers after rejected update: %v", err)
	}
	if existing, ok := localMCPProviderByID(providers, "existing"); !ok || existing.ApprovalMode != "none" {
		t.Fatalf("existing provider was not preserved: %+v", providers)
	}
	if _, ok := localMCPProviderByID(providers, "unsafe"); ok {
		t.Fatalf("invalid provider was persisted: %+v", providers)
	}
}

func TestNormalizeMCPProvidersApprovalModes(t *testing.T) {
	for _, test := range []struct {
		name string
		mode string
		want string
	}{
		{name: "empty defaults to review", mode: "", want: localmcp.ApprovalModeBeforeExecute},
		{name: "none", mode: " NONE ", want: localmcp.ApprovalModeNone},
		{name: "before execute", mode: "Before_Execute", want: localmcp.ApprovalModeBeforeExecute},
		{name: "always", mode: " ALWAYS ", want: localmcp.ApprovalModeAlways},
	} {
		t.Run(test.name, func(t *testing.T) {
			providers, err := normalizeMCPProviders([]localmcp.ProviderConfig{{
				ID:           "test",
				Transport:    "stdio",
				Command:      "python3",
				ApprovalMode: test.mode,
			}})
			if err != nil {
				t.Fatalf("normalizeMCPProviders returned error: %v", err)
			}
			if len(providers) != 1 || providers[0].ApprovalMode != test.want {
				t.Fatalf("normalized approvalMode = %#v, want %q", providers, test.want)
			}
		})
	}
}

func TestLocalMCPProviderSettingsSecretMergeDistinguishesAbsentAndEmpty(t *testing.T) {
	root := t.TempDir()
	server := NewServer(Config{DataDir: root})

	put := func(payload string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPut, "/api/local/mcp-providers", bytes.NewBufferString(payload))
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("save status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "Bearer private-token") || strings.Contains(rec.Body.String(), "private-env-token") {
			t.Fatalf("PUT response leaked provider secret: %s", rec.Body.String())
		}
		return rec
	}

	put(`{"providers":[{"id":"secure","transport":"stdio","command":"mcp-server","env":{"PRIVATE_TOKEN":"private-env-token"},"headers":{"Authorization":"Bearer private-token"},"enabled":true}]}`)
	put(`{"providers":[{"id":"secure","label":"renamed","transport":"stdio","command":"mcp-server","enabled":true}]}`)
	providers, err := server.ReadMCPProviders()
	if err != nil {
		t.Fatal(err)
	}
	provider, ok := localMCPProviderByID(providers, "secure")
	if !ok || provider.Headers["Authorization"] != "Bearer private-token" || provider.Env["PRIVATE_TOKEN"] != "private-env-token" {
		t.Fatalf("absent secret maps must preserve existing values: %+v", provider)
	}

	get := httptest.NewRequest(http.MethodGet, "/api/local/mcp-providers", nil)
	getRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(getRec, get)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body = %s", getRec.Code, getRec.Body.String())
	}
	put(getRec.Body.String())
	providers, err = server.ReadMCPProviders()
	if err != nil {
		t.Fatal(err)
	}
	provider, ok = localMCPProviderByID(providers, "secure")
	if !ok || provider.Headers["Authorization"] != "Bearer private-token" || provider.Env["PRIVATE_TOKEN"] != "private-env-token" {
		t.Fatalf("round-tripping a sanitized GET payload must preserve stored secrets: %+v", provider)
	}

	put(`{"providers":[{"id":"secure","label":"renamed","transport":"stdio","command":"mcp-server","env":{},"headers":{},"enabled":true}]}`)
	providers, err = server.ReadMCPProviders()
	if err != nil {
		t.Fatal(err)
	}
	provider, ok = localMCPProviderByID(providers, "secure")
	if !ok || len(provider.Headers) != 0 || len(provider.Env) != 0 {
		t.Fatalf("explicit empty secret maps must clear existing values: %+v", provider)
	}
}

func TestLocalMCPProviderPublicEndpointsNeverExposeSecretValues(t *testing.T) {
	root := t.TempDir()
	mcp := newMCPProtocolTestServer(t, func(_ string, _ map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"structuredContent": map[string]interface{}{"available": true}}
	}, "jimeng.check_status")
	defer mcp.Close()
	server := NewServer(Config{DataDir: root})
	headerSecret := "Bearer public-endpoint-secret"
	envSecret := "public-env-secret-canary"

	register := httptest.NewRequest(http.MethodPost, "/api/local/jimeng/setup/register-mcp", bytes.NewBufferString(`{"endpoint":"`+mcp.URL+`","env":{"DREAMINA_TOKEN":"`+envSecret+`"},"headers":{"Authorization":"`+headerSecret+`"}}`))
	registerRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(registerRec, register)
	if registerRec.Code != http.StatusOK || strings.Contains(registerRec.Body.String(), headerSecret) || strings.Contains(registerRec.Body.String(), envSecret) {
		t.Fatalf("register response status=%d leaked secret: %s", registerRec.Code, registerRec.Body.String())
	}

	for _, path := range []string{"/api/local/mcp-providers", "/api/local/mcp-providers/status", "/api/local/jimeng/setup/status"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s status=%d body=%s", path, rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), headerSecret) || strings.Contains(rec.Body.String(), envSecret) {
			t.Fatalf("GET %s leaked secret: %s", path, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"hasHeaders":true`) || !strings.Contains(rec.Body.String(), `"Authorization"`) {
			t.Fatalf("GET %s omitted safe header metadata: %s", path, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"hasEnv":true`) || !strings.Contains(rec.Body.String(), `"DREAMINA_TOKEN"`) {
			t.Fatalf("GET %s omitted safe environment metadata: %s", path, rec.Body.String())
		}
	}
}

func TestJiMengSetupStatusReadsDreaminaStatusThroughMCP(t *testing.T) {
	root := t.TempDir()
	runner := &fakeAgentCommandRunner{}
	mcp := newMCPProtocolTestServer(t, func(toolName string, _ map[string]interface{}) map[string]interface{} {
		if toolName != "jimeng.check_status" {
			t.Fatalf("tool = %v, want jimeng.check_status", toolName)
		}
		return map[string]interface{}{
			"structuredContent": map[string]interface{}{
				"available": true,
				"version":   "dreamina-from-mcp",
			},
		}
	}, "jimeng.check_status", "jimeng.generate_video")
	defer mcp.Close()

	server := NewServer(Config{DataDir: root, CommandRunner: runner})
	body := bytes.NewBufferString(`{"providers":[{"id":"jimeng","label":"JiMeng","endpoint":"` + mcp.URL + `","enabled":true}]}`)
	req := httptest.NewRequest(http.MethodPut, "/api/local/mcp-providers", body)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("save status = %d, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/local/jimeng/setup/status", nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("setup status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var status JiMengSetupStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode setup status: %v", err)
	}
	if !status.DreaminaAvailable || status.DreaminaVersion != "dreamina-from-mcp" {
		t.Fatalf("dreamina status = available:%v version:%q, want MCP result", status.DreaminaAvailable, status.DreaminaVersion)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("setup status must not call Dreamina CLI directly: %+v", runner.calls)
	}
}

func TestJiMengInstallCLIRequiresExplicitConfirmation(t *testing.T) {
	root := t.TempDir()
	runner := &fakeAgentCommandRunner{out: CommandOutput{Stdout: "installed"}}
	server := NewServer(Config{DataDir: root, CommandRunner: runner})

	req := httptest.NewRequest(http.MethodPost, "/api/local/jimeng/setup/install-cli", bytes.NewBufferString(`{"confirm":false}`))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("install should not run without confirmation: %+v", runner.calls)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/local/jimeng/setup/install-cli", bytes.NewBufferString(`{"confirm":true}`))
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("install status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(runner.calls) != 1 {
		t.Fatalf("install calls = %d, want 1", len(runner.calls))
	}
	if runner.calls[0].name != "sh" || runner.calls[0].args[0] != "-c" {
		t.Fatalf("install command = %s %#v", runner.calls[0].name, runner.calls[0].args)
	}
	if !strings.Contains(runner.calls[0].args[1], "https://jimeng.jianying.com/cli") {
		t.Fatalf("install script URL missing: %#v", runner.calls[0].args)
	}
}

func TestJiMengRegisterMCPStoresDefaultProvider(t *testing.T) {
	root := t.TempDir()
	server := NewServer(Config{DataDir: root})

	req := httptest.NewRequest(http.MethodPost, "/api/local/jimeng/setup/register-mcp", bytes.NewBufferString(`{"endpoint":"http://127.0.0.1:18180"}`))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("register status = %d, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/local/mcp-providers", nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get providers status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var listed LocalMCPProviderSettingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode providers: %v", err)
	}
	provider, ok := localMCPProviderByID(listed.Providers, "jimeng")
	if !ok || !provider.Enabled {
		t.Fatalf("providers = %+v", listed.Providers)
	}
}

func TestJiMengRegisterMCPStoresDefaultStandardStdioProvider(t *testing.T) {
	root := t.TempDir()
	server := NewServer(Config{DataDir: root})

	req := httptest.NewRequest(http.MethodPost, "/api/local/jimeng/setup/register-mcp", bytes.NewBufferString(`{"transport":"stdio"}`))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("register stdio status = %d, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/local/mcp-providers", nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get providers status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var listed LocalMCPProviderSettingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode providers: %v", err)
	}
	provider, ok := localMCPProviderByID(listed.Providers, "jimeng")
	if !ok {
		t.Fatalf("providers = %+v", listed.Providers)
	}
	if provider.ID != "jimeng" || provider.Transport != "stdio" || provider.Command == "" || len(provider.Args) == 0 {
		t.Fatalf("stdio provider not registered as expected: %+v", provider)
	}
	if provider.ToolPrefix != "jimeng." {
		t.Fatalf("tool prefix = %q, want jimeng.", provider.ToolPrefix)
	}
	if provider.Endpoint != "" {
		t.Fatalf("stdio provider endpoint = %q, want empty", provider.Endpoint)
	}
	if !strings.HasSuffix(provider.Args[0], filepath.Join("mcp", "jimeng", "server.py")) {
		t.Fatalf("stdio script arg = %q, want jimeng server.py", provider.Args[0])
	}
}

func localMCPProviderByID(providers []localmcp.ProviderConfig, id string) (localmcp.ProviderConfig, bool) {
	for _, provider := range providers {
		if provider.ID == id {
			return provider, true
		}
	}
	return localmcp.ProviderConfig{}, false
}

func TestJiMengLoginHeadlessCallsRegisteredMCPProvider(t *testing.T) {
	root := t.TempDir()
	mcp := newMCPProtocolTestServer(t, func(toolName string, _ map[string]interface{}) map[string]interface{} {
		if toolName != "jimeng.login_headless" {
			t.Fatalf("tool = %v, want jimeng.login_headless", toolName)
		}
		return map[string]interface{}{
			"structuredContent": map[string]interface{}{
				"verification_uri": "https://example.com/device",
				"user_code":        "ABCD-EFGH",
				"device_code":      "device-1",
			},
		}
	}, "jimeng.login_headless")
	defer mcp.Close()

	server := NewServer(Config{DataDir: root})
	body := bytes.NewBufferString(`{"providers":[{"id":"jimeng","label":"JiMeng","endpoint":"` + mcp.URL + `","enabled":true}]}`)
	req := httptest.NewRequest(http.MethodPut, "/api/local/mcp-providers", body)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("save status = %d, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/local/jimeng/setup/login-headless", nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	structured := result["structuredContent"].(map[string]interface{})
	if structured["user_code"] != "ABCD-EFGH" {
		t.Fatalf("user_code = %#v", structured["user_code"])
	}
	if result["providerId"] != "jimeng" || result["toolName"] != "jimeng.login_headless" {
		t.Fatalf("JiMeng endpoint did not use generic MCP output: %#v", result)
	}
}

func TestModelProviderSettingsRejectUnsupportedCapability(t *testing.T) {
	server := NewServer(Config{DataDir: t.TempDir()})
	body := bytes.NewBufferString(`{
		"providers":{
			"image_to_video":{"baseUrl":"https://example.com/v1","model":"bad","apiKey":"sk"}
		}
	}`)

	req := httptest.NewRequest(http.MethodPut, "/api/local/model-providers", body)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unsupported capability should return 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestWriteLogAndCreateDiagnostics(t *testing.T) {
	root := t.TempDir()
	server := NewServer(Config{DataDir: root})
	t.Setenv("TANGYING_APP_VERSION", "9.9.9-beta")
	t.Setenv("TANGYING_GIT_COMMIT", "abc1234-test")

	body := bytes.NewBufferString(`{"source":"desktop","level":"info","message":"render complete","fields":{"project":"demo","taskId":"task-beta-123","authorization":"Bearer live-token-should-redact","cookie":"session-cookie-should-redact"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/local/logs", body)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("log status = %d, body = %s", rec.Code, rec.Body.String())
	}
	logFile := filepath.Join(root, "logs", "local-agent.jsonl")
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("expected log file: %v", err)
	}
	if !bytes.Contains(content, []byte("render complete")) {
		t.Fatalf("log file missing message: %s", string(content))
	}
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "mcp-providers.json"), []byte(`{"providers":[{"id":"jimeng","label":"JiMeng MCP","transport":"stdio","command":"python3","args":["mcp/jimeng/server.py"],"env":{"DREAMINA_TOKEN":"secret-token","ACCOUNT_SESSION":"second-env-secret-should-redact"},"headers":{"Authorization":"Bearer mcp-header-token-should-redact","X-Workspace":"custom-header-secret-should-redact"},"enabled":true}]}`), 0o644); err != nil {
		t.Fatalf("write mcp providers: %v", err)
	}
	artifactDir := filepath.Join(root, "artifacts", "vp-1", "video-1")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatalf("create artifact dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(artifactDir, "content"), []byte("raw-video-bytes-should-not-be-zipped"), 0o644); err != nil {
		t.Fatalf("write artifact content: %v", err)
	}
	if err := os.WriteFile(filepath.Join(artifactDir, "metadata.json"), []byte(`{"id":"video-1","projectId":"vp-1","kind":"VIDEO","sourceType":"fallback_preview","isFallback":true,"fallbackReason":"no_ready_aigc_video","storageRef":"local://projects/vp-1/artifacts/video-1/hash/final.mp4"}`), 0o644); err != nil {
		t.Fatalf("write artifact metadata: %v", err)
	}
	reportDir := filepath.Join(root, "projects", "vp-1", "reports", "video_frame_qa")
	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		t.Fatalf("create report dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(reportDir, "shot_qa_reports.json"), []byte(`{"schemaVersion":1,"shotReports":[{"shotId":"SHOT_01","decision":"HUMAN_REVIEW"}]}`), 0o644); err != nil {
		t.Fatalf("write QA report: %v", err)
	}
	pipelineReports := map[string]string{
		"shot_list.json":                `{"shots":[{"shotId":"SHOT_01"}]}`,
		"shot_split_report.json":        `{"policy":{"minShotDurationSec":3,"maxShotDurationSec":15}}`,
		"shot_duration_validation.json": `{"valid":true}`,
		"shot_candidates.json":          `{"candidates":[{"candidateId":"cand-1"}]}`,
		"repair_plans.json":             `{"repairPlans":[{"action":"RERENDER_HTML"}]}`,
		"accepted_shots.json":           `{"acceptedShots":[{"shotId":"SHOT_01","candidateId":"cand-2"}]}`,
		"assembly_plan.json":            `{"steps":["FFMPEG_CONCAT","GLOBAL_SUBTITLE_RENDER"]}`,
		"subtitle_timeline.json":        `{"scope":"global","cues":[]}`,
		"audio_mix_plan.json":           `{"scope":"global","bgmDucking":true}`,
		"final_qa_report.json":          `{"passed":true}`,
		"provenance_summary.json":       `{"fallbackCount":1}`,
	}
	for name, body := range pipelineReports {
		if err := os.WriteFile(filepath.Join(reportDir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write pipeline report %s: %v", name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "logs", "failure-stack.txt"), []byte("panic: render failed\nsk-test-secret-should-redact\n"), 0o644); err != nil {
		t.Fatalf("write failure stack: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "logs", "provider-failure.json"), []byte(`{"authorization":"Bearer json-bearer-should-redact","cookie":"json-cookie-should-redact","message":"provider failed"}`), 0o644); err != nil {
		t.Fatalf("write json failure stack: %v", err)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/local/diagnostics", bytes.NewBufferString(`{"reason":"support-request"}`))
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("diagnostics status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp DiagnosticResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid diagnostics response: %v", err)
	}
	if resp.Path == "" {
		t.Fatalf("diagnostics path is empty")
	}
	if filepath.Base(resp.Path) != "beta-diagnostics.zip" {
		t.Fatalf("diagnostics filename = %q, want beta-diagnostics.zip", filepath.Base(resp.Path))
	}
	zr, err := zip.OpenReader(resp.Path)
	if err != nil {
		t.Fatalf("diagnostics zip not readable: %v", err)
	}
	defer zr.Close()
	entries := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open zip entry %s: %v", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatalf("read zip entry %s: %v", f.Name, err)
		}
		entries[f.Name] = string(data)
	}
	for _, want := range []string{
		"manifest.json",
		"environment.json",
		"env-redacted.json",
		"mcp/provider-status.json",
		"artifacts/manifest.json",
		"qa/shot_qa_reports.json",
		"pipeline/vp-1/reports/video_frame_qa/shot_list.json",
		"pipeline/vp-1/reports/video_frame_qa/shot_split_report.json",
		"pipeline/vp-1/reports/video_frame_qa/shot_duration_validation.json",
		"pipeline/vp-1/reports/video_frame_qa/shot_candidates.json",
		"pipeline/vp-1/reports/video_frame_qa/repair_plans.json",
		"pipeline/vp-1/reports/video_frame_qa/accepted_shots.json",
		"pipeline/vp-1/reports/video_frame_qa/assembly_plan.json",
		"pipeline/vp-1/reports/video_frame_qa/subtitle_timeline.json",
		"pipeline/vp-1/reports/video_frame_qa/audio_mix_plan.json",
		"pipeline/vp-1/reports/video_frame_qa/final_qa_report.json",
		"pipeline/vp-1/reports/video_frame_qa/provenance_summary.json",
		"failures/failure-stack.txt",
		"logs/local-agent.jsonl",
	} {
		if _, ok := entries[want]; !ok {
			t.Fatalf("diagnostics zip missing %s; entries=%v", want, entries)
		}
	}
	if _, ok := entries["artifacts/vp-1/video-1/content"]; ok {
		t.Fatalf("diagnostics zip must not include raw artifact content")
	}
	var manifest map[string]interface{}
	if err := json.Unmarshal([]byte(entries["manifest.json"]), &manifest); err != nil {
		t.Fatalf("invalid diagnostics manifest: %v", err)
	}
	if manifest["appVersion"] != "9.9.9-beta" || manifest["gitCommit"] != "abc1234-test" {
		t.Fatalf("diagnostics manifest should include version and git commit: %#v", manifest)
	}
	var mcpStatus struct {
		Providers []map[string]interface{} `json:"providers"`
	}
	if err := json.Unmarshal([]byte(entries["mcp/provider-status.json"]), &mcpStatus); err != nil {
		t.Fatalf("invalid MCP provider diagnostics: %v", err)
	}
	var diagnosticProvider map[string]interface{}
	for _, provider := range mcpStatus.Providers {
		if provider["id"] == "jimeng" {
			diagnosticProvider = provider
			break
		}
	}
	if diagnosticProvider == nil {
		t.Fatalf("MCP provider diagnostics missing jimeng provider: %#v", mcpStatus.Providers)
	}
	if _, ok := diagnosticProvider["headers"]; ok {
		t.Fatalf("MCP provider diagnostics must omit header values: %#v", diagnosticProvider)
	}
	if diagnosticProvider["hasHeaders"] != true {
		t.Fatalf("MCP provider diagnostics should preserve hasHeaders metadata: %#v", diagnosticProvider)
	}
	headerKeys, ok := diagnosticProvider["headerKeys"].([]interface{})
	if !ok || len(headerKeys) != 2 || headerKeys[0] != "Authorization" || headerKeys[1] != "X-Workspace" {
		t.Fatalf("MCP provider diagnostics should preserve sorted headerKeys metadata: %#v", diagnosticProvider["headerKeys"])
	}
	if _, ok := diagnosticProvider["env"]; ok {
		t.Fatalf("MCP provider diagnostics must omit environment values: %#v", diagnosticProvider)
	}
	if diagnosticProvider["hasEnv"] != true {
		t.Fatalf("MCP provider diagnostics should preserve hasEnv metadata: %#v", diagnosticProvider)
	}
	envKeys, ok := diagnosticProvider["envKeys"].([]interface{})
	if !ok || len(envKeys) != 2 || envKeys[0] != "ACCOUNT_SESSION" || envKeys[1] != "DREAMINA_TOKEN" {
		t.Fatalf("MCP provider diagnostics should preserve sorted envKeys metadata: %#v", diagnosticProvider["envKeys"])
	}
	recentTaskIDs, ok := manifest["recentTaskIds"].([]interface{})
	if !ok || len(recentTaskIDs) != 1 || recentTaskIDs[0] != "task-beta-123" {
		t.Fatalf("diagnostics manifest should include recent task ids, got %#v", manifest["recentTaskIds"])
	}
	allEntries := strings.Join(mapValues(entries), "\n")
	if strings.Contains(allEntries, "secret-token") ||
		strings.Contains(allEntries, "second-env-secret-should-redact") ||
		strings.Contains(allEntries, "sk-test-secret-should-redact") ||
		strings.Contains(allEntries, "mcp-header-token-should-redact") ||
		strings.Contains(allEntries, "custom-header-secret-should-redact") ||
		strings.Contains(allEntries, "live-token-should-redact") ||
		strings.Contains(allEntries, "session-cookie-should-redact") ||
		strings.Contains(allEntries, "json-bearer-should-redact") ||
		strings.Contains(allEntries, "json-cookie-should-redact") {
		t.Fatalf("diagnostics zip leaked a secret: %#v", entries)
	}
}

func TestHandlerAllowsLocalFrontendCORS(t *testing.T) {
	server := NewServer(Config{DataDir: t.TempDir()})

	req := httptest.NewRequest(http.MethodOptions, "/api/local/diagnostics", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", http.MethodPut)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Fatalf("allow-origin = %q, want local frontend origin", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, http.MethodPut) {
		t.Fatalf("allow-methods = %q, want PUT", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "Range") {
		t.Fatalf("allow-headers = %q, want Range", got)
	}
}

func TestHandlerRejectsUntrustedOriginForArtifactWrite(t *testing.T) {
	server := NewServer(Config{DataDir: t.TempDir()})
	body := bytes.NewBufferString(`{
		"id":"art-evil",
		"projectId":"vp-evil",
		"content":"blocked"
	}`)

	req := httptest.NewRequest(http.MethodPost, "/api/local/artifacts", body)
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("untrusted origin status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandlerAllowsLocalOriginForArtifactWrite(t *testing.T) {
	server := NewServer(Config{DataDir: t.TempDir()})
	body := bytes.NewBufferString(`{
		"id":"art-local",
		"projectId":"vp-local",
		"content":"allowed"
	}`)

	req := httptest.NewRequest(http.MethodPost, "/api/local/artifacts", body)
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("local origin status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestLocalArtifactStoreWritesAndReadsUserPayload(t *testing.T) {
	root := t.TempDir()
	server := NewServer(Config{DataDir: root})
	body := bytes.NewBufferString(`{
		"id":"art-1",
		"projectId":"vp-1",
		"storageRef":"local://projects/vp-1/artifacts/script/content/hash/script.md",
		"mimeType":"text/markdown; charset=utf-8",
		"content":"## 本地脚本\n用户资产只保存在本地。",
		"metadata":{"stageName":"script","cloudPayloadStored":false}
	}`)

	req := httptest.NewRequest(http.MethodPost, "/api/local/artifacts", body)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("store status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var stored LocalArtifactResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &stored); err != nil {
		t.Fatalf("invalid store response: %v", err)
	}
	if stored.Path == "" || stored.MetadataPath == "" {
		t.Fatalf("expected local paths in response: %+v", stored)
	}
	content, err := os.ReadFile(stored.Path)
	if err != nil {
		t.Fatalf("expected local artifact payload: %v", err)
	}
	if !bytes.Contains(content, []byte("用户资产只保存在本地")) {
		t.Fatalf("artifact payload mismatch: %s", string(content))
	}

	req = httptest.NewRequest(http.MethodGet, "/api/local/artifacts/art-1?projectId=vp-1", nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("read status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var loaded LocalArtifactResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &loaded); err != nil {
		t.Fatalf("invalid read response: %v", err)
	}
	if loaded.Content != "## 本地脚本\n用户资产只保存在本地。" {
		t.Fatalf("loaded content mismatch: %q", loaded.Content)
	}
	if loaded.Metadata["cloudPayloadStored"] != false {
		t.Fatalf("metadata should preserve cloudPayloadStored=false: %+v", loaded.Metadata)
	}
}

func TestLocalArtifactRawReturnsMediaBytes(t *testing.T) {
	root := t.TempDir()
	server := NewServer(Config{DataDir: root})
	artifactDir := filepath.Join(root, "artifacts", "vp-1", "video-1")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatalf("mkdir artifact: %v", err)
	}
	content := []byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 'm', 'p', '4', '2'}
	if err := os.WriteFile(filepath.Join(artifactDir, "content"), content, 0o644); err != nil {
		t.Fatalf("write content: %v", err)
	}
	metadata := []byte(`{"id":"video-1","projectId":"vp-1","mimeType":"video/mp4","storageRef":"local://projects/vp-1/artifacts/video-1/hash/final.mp4"}`)
	if err := os.WriteFile(filepath.Join(artifactDir, "metadata.json"), metadata, 0o644); err != nil {
		t.Fatalf("write metadata: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/local/artifacts/video-1?projectId=vp-1&raw=1", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("raw status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "video/mp4") {
		t.Fatalf("raw content-type = %q", contentType)
	}
	if !bytes.Equal(rec.Body.Bytes(), content) {
		t.Fatalf("raw content mismatch: %v", rec.Body.Bytes())
	}

	req = httptest.NewRequest(http.MethodHead, "/api/local/artifacts/video-1?projectId=vp-1&raw=1", nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("raw head status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "video/mp4") {
		t.Fatalf("raw head content-type = %q", contentType)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("raw head should not write body, got %d bytes", rec.Body.Len())
	}
}

func TestLocalArtifactDeleteRemovesPayloadAndMetadata(t *testing.T) {
	root := t.TempDir()
	server := NewServer(Config{DataDir: root})
	body := bytes.NewBufferString(`{
		"id":"art-delete",
		"projectId":"vp-1",
		"storageRef":"local://projects/vp-1/artifacts/script/content/hash/script.md",
		"mimeType":"text/markdown",
		"content":"delete me"
	}`)

	req := httptest.NewRequest(http.MethodPost, "/api/local/artifacts", body)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("store status = %d, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/local/artifacts/art-delete?projectId=vp-1", nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", rec.Code, rec.Body.String())
	}

	if _, err := os.Stat(filepath.Join(root, "artifacts", "vp-1", "art-delete")); !os.IsNotExist(err) {
		t.Fatalf("artifact directory should be deleted, err=%v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/local/artifacts/art-delete?projectId=vp-1", nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("deleted artifact should return 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestLocalProjectDeleteRemovesProjectArtifactsAndCache(t *testing.T) {
	root := t.TempDir()
	server := NewServer(Config{DataDir: root})
	for _, dir := range []string{
		filepath.Join(root, "projects", "vp-1"),
		filepath.Join(root, "artifacts", "vp-1", "art-1"),
		filepath.Join(root, "cache", "vp-1"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "data"), []byte("payload"), 0o644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/local/projects/vp-1", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("project delete status = %d, body = %s", rec.Code, rec.Body.String())
	}
	for _, dir := range []string{
		filepath.Join(root, "projects", "vp-1"),
		filepath.Join(root, "artifacts", "vp-1"),
		filepath.Join(root, "cache", "vp-1"),
	} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("project delete should remove %s, err=%v", dir, err)
		}
	}
}

func mapValues(values map[string]string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}
