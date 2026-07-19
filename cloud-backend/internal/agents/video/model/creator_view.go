package model

// ShotPageQuery scopes a creator-facing Shot review list without returning full media payloads.
type ShotPageQuery struct {
	Cursor  string
	Limit   int
	Status  string
	Chapter string
	Query   string
}

type ShotListItem struct {
	ID                  string `json:"id"`
	SequenceIndex       int    `json:"sequenceIndex"`
	Title               string `json:"title"`
	Chapter             string `json:"chapter,omitempty"`
	DurationSec         int    `json:"durationSec"`
	Version             int    `json:"version"`
	ReviewStatus        string `json:"reviewStatus"`
	QAStatus            string `json:"qaStatus,omitempty"`
	GenerationStatus    string `json:"generationStatus"`
	AcceptedCandidateID string `json:"acceptedCandidateId,omitempty"`
	ThumbnailRef        string `json:"thumbnailRef,omitempty"`
}

type ShotPage struct {
	Items      []ShotListItem `json:"items"`
	NextCursor string         `json:"nextCursor,omitempty"`
	Total      int            `json:"total"`
}

type ShotSummary struct {
	Total          int `json:"total"`
	Confirmed      int `json:"confirmed"`
	AwaitingReview int `json:"awaitingReview"`
	Generating     int `json:"generating"`
	NeedsAction    int `json:"needsAction"`
}

type ShotImpact struct {
	ShotID                   string   `json:"shotId"`
	AffectedShotIDs          []string `json:"affectedShotIds"`
	InvalidatesFinalAssembly bool     `json:"invalidatesFinalAssembly"`
	RegeneratesOtherShots    bool     `json:"regeneratesOtherShots"`
	EstimatedDurationSec     int      `json:"estimatedDurationSec"`
	RequiresConfirmation     bool     `json:"requiresConfirmation"`
}

// ShotWorkspace keeps one Shot's durable review state together for the creator workspace.
type ShotWorkspace struct {
	Shot    ShotUnit       `json:"shot"`
	History []ShotRevision `json:"history"`
	Impact  ShotImpact     `json:"impact"`
}
