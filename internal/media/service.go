package media

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// MediaAsset represents a stored media file with analysis metadata.
type MediaAsset struct {
	ID           string   `json:"id"`
	UserID       string   `json:"userId"`
	OriginalName string   `json:"originalName"`
	MimeType     string   `json:"mimeType"`
	Size         int64    `json:"size"`
	MinioPath    string   `json:"minioPath"`
	Tags         []string `json:"tags"`
	EmbeddingID  string   `json:"embeddingId,omitempty"`
	CreatedAt    string   `json:"createdAt"`
	UpdatedAt    string   `json:"updatedAt"`
}

type MediaService struct {
	pool    *pgxpool.Pool
	storage *StorageService
}

func NewMediaService(pool *pgxpool.Pool, storage *StorageService) *MediaService {
	return &MediaService{pool: pool, storage: storage}
}

func (s *MediaService) Upload(ctx context.Context, userID string, files []*multipart.FileHeader) ([]*MediaAsset, error) {
	var assets []*MediaAsset

	for _, fh := range files {
		id := fmt.Sprintf("media-%d-%s", time.Now().UnixMilli(), strings.TrimSuffix(fh.Filename, filepath.Ext(fh.Filename)))
		objectName := fmt.Sprintf("%s/%s/%s", userID, time.Now().Format("2006/01/02"), id+filepath.Ext(fh.Filename))
		mimeType := fh.Header.Get("Content-Type")
		if mimeType == "" {
			mimeType = detectMimeType(fh.Filename)
		}

		file, err := fh.Open()
		if err != nil {
			zap.L().Warn("failed to open uploaded file", zap.String("name", fh.Filename), zap.Error(err))
			continue
		}

		data, err := io.ReadAll(file)
		file.Close()
		if err != nil {
			zap.L().Warn("failed to read uploaded file", zap.String("name", fh.Filename), zap.Error(err))
			continue
		}

		path, err := s.storage.Upload(ctx, objectName, strings.NewReader(string(data)), int64(len(data)), mimeType)
		if err != nil {
			zap.L().Warn("failed to upload to minio", zap.String("name", fh.Filename), zap.Error(err))
			continue
		}

		now := time.Now().UTC().Format(time.RFC3339)
		asset := &MediaAsset{
			ID:           id,
			UserID:       userID,
			OriginalName: fh.Filename,
			MimeType:     mimeType,
			Size:         fh.Size,
			MinioPath:    path,
			Tags:         []string{},
			CreatedAt:    now,
			UpdatedAt:    now,
		}

		_, err = s.pool.Exec(ctx,
			`INSERT INTO media_assets (id, user_id, original_name, mime_type, size, minio_path, tags, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			asset.ID, asset.UserID, asset.OriginalName, asset.MimeType, asset.Size, asset.MinioPath,
			toJSONB(asset.Tags), asset.CreatedAt, asset.UpdatedAt)
		if err != nil {
			zap.L().Warn("failed to save media asset", zap.String("id", id), zap.Error(err))
			continue
		}

		assets = append(assets, asset)
	}

	return assets, nil
}

func (s *MediaService) List(ctx context.Context, userID string, offset, limit int, tag string) ([]*MediaAsset, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	var total int
	countQuery := `SELECT COUNT(*) FROM media_assets WHERE user_id = $1`
	args := []interface{}{userID}

	if tag != "" {
		countQuery += ` AND tags @> $2`
		args = append(args, fmt.Sprintf(`["%s"]`, tag))
	}
	if err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count media assets: %w", err)
	}

	query := `SELECT id, user_id, original_name, mime_type, size, minio_path, tags, embedding_id, created_at, updated_at
	           FROM media_assets WHERE user_id = $1`
	queryArgs := []interface{}{userID}
	paramIdx := 2
	if tag != "" {
		query += fmt.Sprintf(` AND tags @> $%d`, paramIdx)
		queryArgs = append(queryArgs, fmt.Sprintf(`["%s"]`, tag))
		paramIdx++
	}
	query += ` ORDER BY created_at DESC LIMIT $` + fmt.Sprintf("%d", paramIdx)
	queryArgs = append(queryArgs, limit)
	paramIdx++
	query += ` OFFSET $` + fmt.Sprintf("%d", paramIdx)
	queryArgs = append(queryArgs, offset)

	rows, err := s.pool.Query(ctx, query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query media assets: %w", err)
	}
	defer rows.Close()

	var assets []*MediaAsset
	for rows.Next() {
		a := &MediaAsset{}
		var tagsJSON []byte
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&a.ID, &a.UserID, &a.OriginalName, &a.MimeType, &a.Size,
			&a.MinioPath, &tagsJSON, &a.EmbeddingID, &createdAt, &updatedAt); err != nil {
			return nil, 0, fmt.Errorf("failed to scan media asset: %w", err)
		}
		a.Tags = fromJSONB(tagsJSON)
		a.CreatedAt = createdAt.Format(time.RFC3339)
		a.UpdatedAt = updatedAt.Format(time.RFC3339)
		assets = append(assets, a)
	}

	return assets, total, nil
}

func (s *MediaService) Get(ctx context.Context, id string) (*MediaAsset, error) {
	a := &MediaAsset{}
	var tagsJSON []byte
	var createdAt, updatedAt time.Time

	err := s.pool.QueryRow(ctx,
		`SELECT id, user_id, original_name, mime_type, size, minio_path, tags, embedding_id, created_at, updated_at
		 FROM media_assets WHERE id = $1`, id).
		Scan(&a.ID, &a.UserID, &a.OriginalName, &a.MimeType, &a.Size,
			&a.MinioPath, &tagsJSON, &a.EmbeddingID, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("media asset not found: %w", err)
	}
	a.Tags = fromJSONB(tagsJSON)
	a.CreatedAt = createdAt.Format(time.RFC3339)
	a.UpdatedAt = updatedAt.Format(time.RFC3339)
	return a, nil
}

func (s *MediaService) UpdateTags(ctx context.Context, id string, tags []string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE media_assets SET tags = $1, updated_at = NOW() WHERE id = $2`,
		toJSONB(tags), id)
	return err
}

func (s *MediaService) GetURL(ctx context.Context, id string) (string, error) {
	asset, err := s.Get(ctx, id)
	if err != nil {
		return "", err
	}
	return s.storage.GetURL(ctx, asset.MinioPath)
}

func detectMimeType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	case ".mp4":
		return "video/mp4"
	case ".mov":
		return "video/quicktime"
	case ".avi":
		return "video/x-msvideo"
	case ".mkv":
		return "video/x-matroska"
	default:
		return "application/octet-stream"
	}
}

func toJSONB(tags []string) []byte {
	b := []byte("[")
	for i, t := range tags {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, '"')
		b = append(b, []byte(t)...)
		b = append(b, '"')
	}
	b = append(b, ']')
	return b
}

func fromJSONB(data []byte) []string {
	if len(data) < 2 {
		return nil
	}
	s := string(data[1 : len(data)-1])
	if s == "" {
		return nil
	}
	var tags []string
	for _, p := range strings.Split(s, ",") {
		p = strings.Trim(p, "\" ")
		if p != "" {
			tags = append(tags, p)
		}
	}
	return tags
}
