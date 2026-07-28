package artifact

import (
	"context"
	"errors"
	"fmt"
	"path"
	"reflect"
	"regexp"
	"strings"

	"go.uber.org/zap"
)

var ErrArtifactVersionConflict = errors.New("artifact version conflict")

// ArtifactStatus represents the lifecycle status of an artifact.
type ArtifactStatus string

const (
	ArtifactStatusValid    ArtifactStatus = "valid"
	ArtifactStatusPending  ArtifactStatus = "pending"
	ArtifactStatusStale    ArtifactStatus = "stale"
	ArtifactStatusRejected ArtifactStatus = "rejected"
	ArtifactStatusFailed   ArtifactStatus = "failed"
	ArtifactStatusDeleted  ArtifactStatus = "deleted"
)

// Service provides business logic for artifact management.
type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

type artifactCreationStore interface {
	FindByHash(context.Context, string, string, string, string) (*Artifact, error)
	FindCurrent(context.Context, string, string, string) (*Artifact, error)
	Save(context.Context, *Artifact) error
}

// CreateArtifact creates a new artifact version. If a version with the same
// content hash already exists for the same scope, it returns the existing one
// (idempotent). Otherwise, it auto-increments the version number and saves.
func (s *Service) CreateArtifact(ctx context.Context, req *CreateArtifactRequest) (*Artifact, error) {
	return createArtifactWithStore(ctx, s.repo, req)
}

func createArtifactWithStore(ctx context.Context, store artifactCreationStore, req *CreateArtifactRequest) (*Artifact, error) {
	if req.ContentHash == "" && len(req.Data) > 0 {
		req.ContentHash = HashContent(req.Data)
	}

	var (
		nextVersion int
		parentID    string
		err         error
	)
	if hasExpectedArtifactParent(req) {
		// Do not replace the authorized parent with a newly observed current
		// row. Repository.Save compares this candidate to the lineage again
		// while holding the lineage advisory lock.
		nextVersion, parentID, err = artifactLineageCandidate(req, nil)
	} else {
		// Check for idempotent duplicate (same content hash = same result).
		if contentHashDedupEnabled(req) {
			existing, findErr := store.FindByHash(ctx, req.ProjectID, req.StageName, req.UnitID, req.ContentHash)
			if findErr == nil && existing != nil {
				zap.L().Debug("Artifact already exists (idempotent)", zap.String("hash", req.ContentHash))
				return existing, nil
			}
		}
		current, _ := store.FindCurrent(ctx, req.ProjectID, req.StageName, req.UnitID)
		nextVersion, parentID, err = artifactLineageCandidate(req, current)
	}
	if err != nil {
		return nil, err
	}

	artifact := buildArtifactRecord(req, nextVersion, parentID)

	if err := store.Save(ctx, artifact); err != nil {
		return nil, fmt.Errorf("failed to create artifact: %w", err)
	}

	zap.L().Info("Artifact created",
		zap.String("id", artifact.ID),
		zap.String("stage", artifact.StageName),
		zap.Int("version", artifact.Version),
	)
	return artifact, nil
}

func contentHashDedupEnabled(req *CreateArtifactRequest) bool {
	return req != nil && !req.ForceNewVersion && !hasExpectedArtifactParent(req) && req.ContentHash != ""
}

func hasExpectedArtifactParent(req *CreateArtifactRequest) bool {
	return req != nil && (req.ExpectedParentID != "" || req.ExpectedParentVersion != 0)
}

func artifactLineageCandidate(req *CreateArtifactRequest, current *Artifact) (int, string, error) {
	if hasExpectedArtifactParent(req) {
		parentID := strings.TrimSpace(req.ExpectedParentID)
		if parentID == "" || parentID != req.ExpectedParentID || req.ExpectedParentVersion <= 0 {
			return 0, "", ErrArtifactVersionConflict
		}
		return req.ExpectedParentVersion + 1, parentID, nil
	}
	if current != nil {
		return current.Version + 1, current.ID, nil
	}
	return 1, "", nil
}

// GetCurrent returns the current version of an artifact for the given scope.
func (s *Service) GetCurrent(ctx context.Context, projectID, stageName, unitID string) (*Artifact, error) {
	return s.repo.FindCurrent(ctx, projectID, stageName, unitID)
}

// GetByID returns an artifact by ID.
func (s *Service) GetByID(ctx context.Context, id string) (*Artifact, error) {
	return s.repo.FindByID(ctx, id)
}

// GetHistory returns all versions of an artifact, newest first.
func (s *Service) GetHistory(ctx context.Context, projectID, stageName, unitID string) ([]*Artifact, error) {
	return s.repo.FindHistory(ctx, projectID, stageName, unitID)
}

type artifactProjectLister interface {
	ListByProject(context.Context, string) ([]*Artifact, error)
	ListAllVersionsByProject(context.Context, string) ([]*Artifact, error)
}

func listArtifactsByProject(ctx context.Context, store artifactProjectLister, projectID string, includeHistory bool) ([]*Artifact, error) {
	if includeHistory {
		return store.ListAllVersionsByProject(ctx, projectID)
	}
	return store.ListByProject(ctx, projectID)
}

// ListByProject returns all current artifacts for a project.
func (s *Service) ListByProject(ctx context.Context, projectID string) ([]*Artifact, error) {
	return listArtifactsByProject(ctx, s.repo, projectID, false)
}

// ListAllVersionsByProject returns current and historical artifact metadata for diagnostics.
func (s *Service) ListAllVersionsByProject(ctx context.Context, projectID string) ([]*Artifact, error) {
	return listArtifactsByProject(ctx, s.repo, projectID, true)
}

// ListUsableByProject returns only current, valid artifacts for a project.
// Artifacts with status=stale, rejected, failed, or deleted are excluded.
func (s *Service) ListUsableByProject(ctx context.Context, projectID string) ([]*Artifact, error) {
	return s.repo.ListUsableByProject(ctx, projectID)
}

// ListCurrentByProject is an alias for ListByProject (all current artifacts).
func (s *Service) ListCurrentByProject(ctx context.Context, projectID string) ([]*Artifact, error) {
	return s.repo.ListByProject(ctx, projectID)
}

// ApproveArtifact marks an artifact as human-approved.
func (s *Service) ApproveArtifact(ctx context.Context, artifactID string, reviewerID string) error {
	artifact, err := s.repo.FindByID(ctx, artifactID)
	if err != nil {
		return fmt.Errorf("approve artifact: %w", err)
	}
	if err := s.repo.UpdateHumanApproved(ctx, artifactID, true); err != nil {
		return fmt.Errorf("approve artifact: %w", err)
	}
	zap.L().Info("Artifact approved",
		zap.String("artifactId", artifactID),
		zap.String("kind", string(artifact.Kind)),
		zap.String("reviewerId", reviewerID),
	)
	return nil
}

// ApproveCurrentArtifactsByStageAndKinds marks current valid artifacts for a
// stage as human-approved. It is used as the review approval fallback when the
// review gate does not yet carry a concrete artifact ID.
func (s *Service) ApproveCurrentArtifactsByStageAndKinds(
	ctx context.Context,
	projectID string,
	stageName string,
	artifactKinds []string,
	reviewerID string,
) ([]string, error) {
	if projectID == "" || stageName == "" || len(artifactKinds) == 0 {
		return nil, nil
	}

	approvedIDs := make([]string, 0, len(artifactKinds))
	for _, kind := range artifactKinds {
		kind = strings.TrimSpace(kind)
		if kind == "" {
			continue
		}
		artifact, err := s.repo.FindCurrentByStageAndKind(ctx, projectID, stageName, kind)
		if err != nil {
			return approvedIDs, fmt.Errorf("approve current artifact %s/%s/%s: %w", projectID, stageName, kind, err)
		}
		if artifact == nil {
			continue
		}
		switch ArtifactStatus(artifact.Status) {
		case ArtifactStatusStale, ArtifactStatusRejected, ArtifactStatusFailed, ArtifactStatusDeleted:
			continue
		}
		if err := s.repo.UpdateHumanApproved(ctx, artifact.ID, true); err != nil {
			return approvedIDs, fmt.Errorf("approve current artifact %s: %w", artifact.ID, err)
		}
		approvedIDs = append(approvedIDs, artifact.ID)
	}

	zap.L().Info("Current stage artifacts approved",
		zap.String("projectId", projectID),
		zap.String("stageName", stageName),
		zap.Strings("artifactKinds", artifactKinds),
		zap.Strings("approvedIds", approvedIDs),
		zap.String("reviewerId", reviewerID),
	)
	return approvedIDs, nil
}

// RejectArtifact marks an artifact as rejected.
func (s *Service) RejectArtifact(ctx context.Context, artifactID string, reviewerID string, reason string) error {
	if err := s.repo.UpdateStatus(ctx, artifactID, string(ArtifactStatusRejected)); err != nil {
		return fmt.Errorf("reject artifact: %w", err)
	}
	zap.L().Info("Artifact rejected",
		zap.String("artifactId", artifactID),
		zap.String("reviewerId", reviewerID),
		zap.String("reason", reason),
	)
	return nil
}

// MarkArtifactStale marks a single artifact as stale with a reason.
func (s *Service) MarkArtifactStale(ctx context.Context, artifactID string, reason string) error {
	if err := s.repo.UpdateStatus(ctx, artifactID, string(ArtifactStatusStale)); err != nil {
		return fmt.Errorf("mark artifact stale: %w", err)
	}
	zap.L().Info("Artifact marked stale",
		zap.String("artifactId", artifactID),
		zap.String("reason", reason),
	)
	return nil
}

// MarkDownstreamStale marks all downstream artifacts as stale when an upstream
// artifact changes. Returns the list of stage names that were marked stale.
func (s *Service) MarkDownstreamStale(ctx context.Context, projectID string, changedArtifactID string, reason string) ([]string, error) {
	// Find the changed artifact to determine its stage
	changed, err := s.repo.FindByID(ctx, changedArtifactID)
	if err != nil {
		return nil, fmt.Errorf("mark downstream stale: %w", err)
	}

	// Get the downstream stage names to invalidate (from the artifact's stage_name)
	downstreamStages := DownstreamStageNamesForStage(changed.StageName)
	if len(downstreamStages) == 0 {
		zap.L().Debug("No downstream artifacts to stale",
			zap.String("changedArtifactId", changedArtifactID),
			zap.String("stageName", changed.StageName),
		)
		return nil, nil
	}

	// Mark them stale in the database
	affectedIDs, err := s.repo.MarkStaleByStageNames(ctx, projectID, downstreamStages, reason)
	if err != nil {
		return nil, fmt.Errorf("mark downstream stale: %w", err)
	}

	zap.L().Info("Downstream artifacts marked stale",
		zap.String("projectId", projectID),
		zap.String("changedArtifactId", changedArtifactID),
		zap.String("changedStage", changed.StageName),
		zap.Strings("downstreamStages", downstreamStages),
		zap.Int("affectedCount", len(affectedIDs)),
	)
	return downstreamStages, nil
}

// MarkDownstreamStaleByStageName marks all artifacts downstream of the given
// changed stage_name as stale. It is used when the review gate cannot carry a
// concrete artifact ID but does know the changed stage.
func (s *Service) MarkDownstreamStaleByStageName(ctx context.Context, projectID string, stageName string, reason string) ([]string, error) {
	downstreamStages := DownstreamStageNamesForStage(stageName)
	if len(downstreamStages) == 0 {
		return nil, nil
	}

	affectedIDs, err := s.repo.MarkStaleByStageNames(ctx, projectID, downstreamStages, reason)
	if err != nil {
		return nil, fmt.Errorf("mark downstream stale by stage name: %w", err)
	}

	zap.L().Info("Downstream artifacts marked stale by changed stage name",
		zap.String("projectId", projectID),
		zap.String("stageName", stageName),
		zap.Strings("downstreamStages", downstreamStages),
		zap.String("reason", reason),
		zap.Int("affectedCount", len(affectedIDs)),
	)
	return downstreamStages, nil
}

// FindCurrentByKind finds the current artifact of a specific stage kind for a project.
func (s *Service) FindCurrentByKind(ctx context.Context, projectID, stageName string) (*Artifact, error) {
	return s.repo.FindCurrentByKind(ctx, projectID, stageName)
}

// FindCurrentByStageAndKind finds the current artifact for an exact stage+kind pair.
func (s *Service) FindCurrentByStageAndKind(ctx context.Context, projectID, stageName, artifactKind string) (*Artifact, error) {
	return s.repo.FindCurrentByStageAndKind(ctx, projectID, stageName, artifactKind)
}

func buildArtifactRecord(req *CreateArtifactRequest, nextVersion int, parentID string) *Artifact {
	if req.ContentHash == "" && len(req.Data) > 0 {
		req.ContentHash = HashContent(req.Data)
	}

	metadata := cloneMetadata(req.Metadata)
	if req.RestoredFromID != "" {
		metadata["restoredFromArtifactId"] = req.RestoredFromID
	}
	if _, exists := metadata["requestedStorageType"]; !exists && !req.ForceNewVersion && req.StorageType != "" && req.StorageType != StorageLocal {
		metadata["requestedStorageType"] = req.StorageType
	}

	storageRef := req.StorageRef
	if strings.TrimSpace(storageRef) == "" {
		storageRef = LocalArtifactRef(req.ProjectID, req.StageName, req.UnitID, req.ContentHash, req.Name)
	}

	sizeBytes := req.SizeBytes
	if sizeBytes == 0 && len(req.Data) > 0 {
		sizeBytes = int64(len(req.Data))
	}

	storageType, inlineJSON := artifactStorageForCreate(req, metadata)

	// Determine beta index fields from metadata or default
	status := stringMetadata(metadata, "status")
	if status == "" {
		status = string(ArtifactStatusValid)
	}
	humanApproved := boolMetadata(metadata, "humanApproved")
	dependsOn := stringSliceMetadata(metadata, "dependsOn")
	producedByNode := stringMetadata(metadata, "producedByNode")
	producedByTool := stringMetadata(metadata, "producedByTool")
	producedByRole := stringMetadata(metadata, "producedByRole")

	return &Artifact{
		ID:             req.ID,
		ProjectID:      req.ProjectID,
		WorkflowRunID:  req.WorkflowRunID,
		TaskID:         req.TaskID,
		StageName:      req.StageName,
		RoleAgentID:    req.RoleAgentID,
		UnitID:         req.UnitID,
		Kind:           req.Kind,
		Name:           req.Name,
		Version:        nextVersion,
		ParentID:       parentID,
		StorageType:    storageType,
		StorageRef:     storageRef,
		InlineJSON:     inlineJSON,
		MimeType:       req.MimeType,
		SizeBytes:      sizeBytes,
		ContentHash:    req.ContentHash,
		PromptHash:     req.PromptHash,
		Provider:       req.Provider,
		Model:          req.Model,
		IsCurrent:      true,
		Status:         status,
		HumanApproved:  humanApproved,
		DependsOn:      dependsOn,
		ProducedByNode: producedByNode,
		ProducedByTool: producedByTool,
		ProducedByRole: producedByRole,
		Metadata:       metadata,
	}
}

func artifactStorageForCreate(req *CreateArtifactRequest, metadata map[string]interface{}) (string, string) {
	metadata["cloudPayloadStored"] = false
	metadata["localOnly"] = true

	// Restores are copies of an already-confirmed artifact, so retain the
	// exact storage representation instead of applying the normal new-artifact
	// local-storage normalization.
	if req.ForceNewVersion {
		storageType := req.StorageType
		if storageType == "" {
			storageType = StorageLocal
		}
		switch storageType {
		case StorageInline:
			metadata["cloudPayloadStored"] = true
			metadata["localOnly"] = false
			metadata["contentAvailability"] = "inline"
			return storageType, string(req.Data)
		case StorageLocal:
			if len(req.Data) > 0 {
				metadata["contentAvailability"] = "local-agent"
			}
		default:
			metadata["cloudPayloadStored"] = true
			metadata["localOnly"] = false
			metadata["contentAvailability"] = "remote"
		}
		return storageType, ""
	}

	if isReviewableInlineProvider(req.Provider) && req.StorageType == StorageInline && len(req.Data) > 0 {
		metadata["cloudPayloadStored"] = true
		metadata["localOnly"] = false
		metadata["contentAvailability"] = "inline"
		return StorageInline, string(req.Data)
	}
	if len(req.Data) > 0 {
		metadata["contentAvailability"] = "local-agent"
	}
	return StorageLocal, ""
}

func isReviewableInlineProvider(provider string) bool {
	switch provider {
	case "artifact-revision", "external-generation-request", "video-creation-profile", "time-window-plan", "audio-master-timeline", "broll-manifest", "visual-alignment-plan":
		return true
	default:
		return false
	}
}

// promoteArtifactIndexFields ensures backward compatibility by syncing
// between dedicated columns and metadata. Dedicated columns are the
// primary source; metadata is updated to match.
func promoteArtifactIndexFields(a *Artifact) *Artifact {
	if a == nil {
		return nil
	}
	if a.Metadata == nil {
		a.Metadata = map[string]interface{}{}
	}
	// If dedicated column is empty, fall back to metadata (backward compat)
	if a.Status == "" {
		a.Status = stringMetadata(a.Metadata, "status")
		if a.Status == "" {
			a.Status = "valid"
		}
	}
	if !a.HumanApproved {
		a.HumanApproved = boolMetadata(a.Metadata, "humanApproved")
	}
	if a.TaskID == "" {
		a.TaskID = stringMetadata(a.Metadata, "taskId")
	}
	if a.RoleAgentID == "" {
		a.RoleAgentID = stringMetadata(a.Metadata, "roleAgentId")
	}
	if len(a.DependsOn) == 0 {
		a.DependsOn = stringSliceMetadata(a.Metadata, "dependsOn")
	}
	if a.ProducedByNode == "" {
		a.ProducedByNode = stringMetadata(a.Metadata, "producedByNode")
	}
	if a.ProducedByTool == "" {
		a.ProducedByTool = stringMetadata(a.Metadata, "producedByTool")
	}
	if a.ProducedByRole == "" {
		a.ProducedByRole = stringMetadata(a.Metadata, "producedByRole")
	}
	if !a.CreatedAt.IsZero() && a.UpdatedAt.IsZero() {
		a.UpdatedAt = a.CreatedAt
	}
	// Sync back to metadata for backward compat
	a.Metadata["status"] = a.Status
	a.Metadata["humanApproved"] = a.HumanApproved
	if a.TaskID != "" {
		a.Metadata["taskId"] = a.TaskID
	}
	if a.RoleAgentID != "" {
		a.Metadata["roleAgentId"] = a.RoleAgentID
	}
	if len(a.DependsOn) > 0 {
		a.Metadata["dependsOn"] = a.DependsOn
	}
	if a.ProducedByNode != "" {
		a.Metadata["producedByNode"] = a.ProducedByNode
	}
	if a.ProducedByTool != "" {
		a.Metadata["producedByTool"] = a.ProducedByTool
	}
	if a.ProducedByRole != "" {
		a.Metadata["producedByRole"] = a.ProducedByRole
	}
	return a
}

func stringMetadata(metadata map[string]interface{}, key string) string {
	if value, ok := metadata[key].(string); ok {
		return value
	}
	return ""
}

func boolMetadata(metadata map[string]interface{}, key string) bool {
	if value, ok := metadata[key].(bool); ok {
		return value
	}
	return false
}

func stringSliceMetadata(metadata map[string]interface{}, key string) []string {
	switch typed := metadata[key].(type) {
	case []string:
		return typed
	case []interface{}:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if value, ok := item.(string); ok {
				result = append(result, value)
			}
		}
		return result
	default:
		return nil
	}
}

func LocalArtifactRef(projectID, stageName, unitID, contentHash, name string) string {
	hash := safeStorageSegment(contentHash)
	if hash == "" {
		hash = "pending"
	}
	fileName := safeStorageSegment(name)
	if fileName == "" {
		fileName = "artifact"
	}
	return "local://projects/" + path.Join(
		safeStorageSegment(projectID),
		"artifacts",
		safeStorageSegment(stageName),
		safeStorageSegment(unitID),
		hash,
		fileName,
	)
}

func cloneMetadata(metadata map[string]interface{}) map[string]interface{} {
	cloned := map[string]interface{}{}
	for key, value := range metadata {
		cloned[key] = deepCloneMetadataValue(value)
	}
	return cloned
}

func deepCloneMetadataValue(value interface{}) interface{} {
	if value == nil {
		return nil
	}
	cloned := cloneMetadataReflectValue(reflect.ValueOf(value), map[metadataCloneVisit]reflect.Value{})
	if !cloned.IsValid() || !cloned.CanInterface() {
		return value
	}
	return cloned.Interface()
}

type metadataCloneVisit struct {
	typ  reflect.Type
	kind reflect.Kind
	ptr  uintptr
}

// cloneMetadataReflectValue recursively preserves concrete metadata types.
// Metadata normally contains JSON-shaped values, but revision callers can
// supply typed maps/slices/pointers too; cloning those avoids historical
// provenance being changed by later edits. The visited table also preserves
// cyclic pointer/map/slice graphs without unbounded recursion.
func cloneMetadataReflectValue(value reflect.Value, visited map[metadataCloneVisit]reflect.Value) reflect.Value {
	if !value.IsValid() || !value.CanInterface() {
		return value
	}
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		cloned := reflect.New(value.Type()).Elem()
		cloned.Set(cloneMetadataReflectValue(value.Elem(), visited))
		return cloned
	case reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		key := metadataCloneVisit{typ: value.Type(), kind: value.Kind(), ptr: uintptr(value.UnsafePointer())}
		if existing, ok := visited[key]; ok {
			return existing
		}
		cloned := reflect.New(value.Type().Elem())
		visited[key] = cloned
		cloned.Elem().Set(cloneMetadataReflectValue(value.Elem(), visited))
		return cloned
	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		key := metadataCloneVisit{typ: value.Type(), kind: value.Kind(), ptr: uintptr(value.UnsafePointer())}
		if existing, ok := visited[key]; ok {
			return existing
		}
		cloned := reflect.MakeMapWithSize(value.Type(), value.Len())
		visited[key] = cloned
		iterator := value.MapRange()
		for iterator.Next() {
			cloned.SetMapIndex(
				cloneMetadataReflectValue(iterator.Key(), visited),
				cloneMetadataReflectValue(iterator.Value(), visited),
			)
		}
		return cloned
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		key := metadataCloneVisit{typ: value.Type(), kind: value.Kind(), ptr: uintptr(value.UnsafePointer())}
		if key.ptr != 0 {
			if existing, ok := visited[key]; ok {
				return existing
			}
		}
		cloned := reflect.MakeSlice(value.Type(), value.Len(), value.Cap())
		if key.ptr != 0 {
			visited[key] = cloned
		}
		for i := 0; i < value.Len(); i++ {
			cloned.Index(i).Set(cloneMetadataReflectValue(value.Index(i), visited))
		}
		return cloned
	case reflect.Array:
		cloned := reflect.New(value.Type()).Elem()
		for i := 0; i < value.Len(); i++ {
			cloned.Index(i).Set(cloneMetadataReflectValue(value.Index(i), visited))
		}
		return cloned
	case reflect.Struct:
		// Start from a value copy so unexported implementation fields (for
		// example in standard-library structs) retain their exact values.
		cloned := reflect.New(value.Type()).Elem()
		cloned.Set(value)
		for i := 0; i < value.NumField(); i++ {
			if cloned.Field(i).CanSet() && value.Field(i).CanInterface() {
				cloned.Field(i).Set(cloneMetadataReflectValue(value.Field(i), visited))
			}
		}
		return cloned
	default:
		return value
	}
}

var unsafeStorageSegmentPattern = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func safeStorageSegment(value string) string {
	cleaned := unsafeStorageSegmentPattern.ReplaceAllString(strings.TrimSpace(value), "-")
	cleaned = strings.Trim(cleaned, ".-_")
	if cleaned == "" {
		return ""
	}
	return cleaned
}
