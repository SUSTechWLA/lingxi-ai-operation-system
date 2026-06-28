package agentruntime

import "strings"

type RetrievalPolicy string

const (
	RetrievalNone      RetrievalPolicy = "none"
	RetrievalOptional  RetrievalPolicy = "optional"
	RetrievalRequired  RetrievalPolicy = "required"
	RetrievalForbidden RetrievalPolicy = "forbidden"
)

type FreshnessLevel string

const (
	FreshnessNone   FreshnessLevel = "none"
	FreshnessLow    FreshnessLevel = "low"
	FreshnessMedium FreshnessLevel = "medium"
	FreshnessHigh   FreshnessLevel = "high"
)

type KnowledgePolicy struct {
	ContentType           string          `json:"contentType"`
	FreshnessLevel        FreshnessLevel  `json:"freshnessLevel"`
	RetrievalPolicy       RetrievalPolicy `json:"retrievalPolicy"`
	KnowledgeType         string          `json:"knowledgeType"`
	AllowedTools          []string        `json:"allowedTools,omitempty"`
	ForbiddenTools        []string        `json:"forbiddenTools,omitempty"`
	RequiredCapabilities  []string        `json:"requiredCapabilities,omitempty"`
	ForbiddenCapabilities []string        `json:"forbiddenCapabilities,omitempty"`
	SearchQueries         []string        `json:"searchQueries,omitempty"`
	FreshnessDays         int             `json:"freshnessDays,omitempty"`
	MaxSearchResults      int             `json:"maxSearchResults,omitempty"`
	MustUseFacts          bool            `json:"mustUseFacts"`
	MustCiteFacts         bool            `json:"mustCiteFacts"`
	BlockOnEmptyFacts     bool            `json:"blockOnEmptyFacts"`
}

var highFreshnessKeywords = []string{
	"最新", "最近", "今天", "昨天", "刚刚", "实时", "现在",
	"出线", "夺冠", "晋级", "比赛结果", "世界杯", "奥运会",
	"发布", "上线", "政策", "法规", "价格", "票房", "榜单",
	"2026", "今年", "本届", "现任",
}

var freshKnowledgeCapabilities = []string{
	"fresh_knowledge",
	"news_search",
	"web_search",
	"current_event_retrieval",
	"fact_retrieval",
	"knowledge_research",
	"fact_gathering",
	"realtime_knowledge",
	"current_events",
	"freshness",
}

func FreshKnowledgeCapabilities() []string {
	return append([]string(nil), freshKnowledgeCapabilities...)
}

// DefaultKnowledgePolicy returns a conservative rule-based policy used as the
// backend fallback when the planner omits a policy.
func DefaultKnowledgePolicy(message, domain string) *KnowledgePolicy {
	message = strings.TrimSpace(message)
	if message == "" {
		return nil
	}
	if domain != "" && domain != "video_creation" {
		return nil
	}
	if RequiresFreshKnowledge(message) {
		return &KnowledgePolicy{
			ContentType:          inferHighFreshnessContentType(message),
			FreshnessLevel:       FreshnessHigh,
			RetrievalPolicy:      RetrievalRequired,
			KnowledgeType:        "latest_news",
			RequiredCapabilities: FreshKnowledgeCapabilities(),
			SearchQueries:        defaultSearchQueries(message),
			FreshnessDays:        60,
			MaxSearchResults:     8,
			MustUseFacts:         true,
			MustCiteFacts:        true,
			BlockOnEmptyFacts:    true,
		}
	}
	return &KnowledgePolicy{
		ContentType:           "opinion",
		FreshnessLevel:        FreshnessNone,
		RetrievalPolicy:       RetrievalNone,
		KnowledgeType:         "none",
		ForbiddenCapabilities: FreshKnowledgeCapabilities(),
		MustUseFacts:          false,
		MustCiteFacts:         false,
		BlockOnEmptyFacts:     false,
	}
}

func RequiresFreshKnowledge(message string) bool {
	return containsAnyKeyword(message, highFreshnessKeywords)
}

func containsAnyKeyword(message string, keywords []string) bool {
	lowered := strings.ToLower(message)
	for _, keyword := range keywords {
		if strings.Contains(lowered, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

func inferHighFreshnessContentType(message string) string {
	if strings.Contains(message, "世界杯") || strings.Contains(message, "奥运会") ||
		strings.Contains(message, "比赛") || strings.Contains(message, "出线") ||
		strings.Contains(message, "夺冠") || strings.Contains(message, "晋级") {
		return "sports_event"
	}
	return "current_event"
}

func defaultSearchQueries(message string) []string {
	return []string{message + " 最新"}
}

func containsString(items []string, needle string) bool {
	for _, item := range items {
		if item == needle {
			return true
		}
	}
	return false
}
