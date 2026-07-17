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
	Reason                string          `json:"reason,omitempty"`
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
	"2026", "今年", "本届", "现任", "新闻", "时事", "突发",
	"发生了什么", "动态", "近况", "核验", "查证", "引用来源", "数据更新",
}

var noWebKeywords = []string{
	"不要联网", "不用联网", "别联网", "不联网", "不要搜索", "不用搜索",
	"不要查资料", "不查资料", "无需查资料", "不要检索", "不用检索",
	"offline", "no web", "do not search", "without search",
}

var negativeFreshnessKeywords = []string{
	"虚构", "架空", "脑洞", "纯创意", "创意口播", "情绪类", "文案",
	"风格化脚本", "改写", "润色", "扩写", "产品宣传", "故事短片",
	"cyberpunk fiction", "fiction", "rewrite", "polish",
}

var factVerificationKeywords = []string{
	"事实核验", "核验", "查证", "引用来源", "引用", "出处", "数据更新",
	"对比", "榜单", "价格", "法规", "政策",
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
	signals := analyzeKnowledgeSignals(message)
	if signals.noWeb {
		return noRetrievalKnowledgePolicy(signals.contentType, "user explicitly disabled web/search access", true)
	}
	if signals.positive && !signals.negative {
		return &KnowledgePolicy{
			ContentType:          inferHighFreshnessContentType(message),
			FreshnessLevel:       FreshnessHigh,
			RetrievalPolicy:      RetrievalRequired,
			KnowledgeType:        "latest_news",
			Reason:               signals.reason,
			RequiredCapabilities: FreshKnowledgeCapabilities(),
			SearchQueries:        defaultSearchQueries(message),
			FreshnessDays:        60,
			MaxSearchResults:     8,
			MustUseFacts:         true,
			MustCiteFacts:        true,
			BlockOnEmptyFacts:    true,
		}
	}
	return noRetrievalKnowledgePolicy(signals.contentType, signals.reason, false)
}

func RequiresFreshKnowledge(message string) bool {
	signals := analyzeKnowledgeSignals(message)
	return signals.positive && !signals.negative && !signals.noWeb
}

type knowledgeSignals struct {
	positive    bool
	negative    bool
	noWeb       bool
	contentType string
	reason      string
}

func analyzeKnowledgeSignals(message string) knowledgeSignals {
	noWeb := containsAnyKeyword(message, noWebKeywords)
	negative := containsAnyKeyword(message, negativeFreshnessKeywords)
	positive := containsAnyKeyword(message, highFreshnessKeywords) || containsAnyKeyword(message, factVerificationKeywords)
	contentType := "opinion"
	reason := "no fresh-knowledge signal matched"
	switch {
	case noWeb:
		contentType = "offline_request"
		reason = "user explicitly disabled web/search access"
	case negative:
		contentType = inferNegativeFreshnessContentType(message)
		reason = "creative, fiction, rewrite, or stable-content signal suppresses fresh retrieval"
	case positive:
		contentType = inferHighFreshnessContentType(message)
		reason = "fresh/current-event/fact-verification signal matched"
	}
	return knowledgeSignals{
		positive:    positive,
		negative:    negative,
		noWeb:       noWeb,
		contentType: contentType,
		reason:      reason,
	}
}

func noRetrievalKnowledgePolicy(contentType, reason string, forbidExternal bool) *KnowledgePolicy {
	forbiddenCapabilities := FreshKnowledgeCapabilities()
	forbiddenTools := []string{"news_search", "web_search"}
	if forbidExternal {
		forbiddenCapabilities = append(forbiddenCapabilities, "external_api")
		forbiddenTools = append(forbiddenTools, "external_api")
	}
	if contentType == "" {
		contentType = "opinion"
	}
	if reason == "" {
		reason = "fresh knowledge is not required"
	}
	return &KnowledgePolicy{
		ContentType:           contentType,
		FreshnessLevel:        FreshnessNone,
		RetrievalPolicy:       RetrievalNone,
		KnowledgeType:         "none",
		Reason:                reason,
		ForbiddenTools:        forbiddenTools,
		ForbiddenCapabilities: forbiddenCapabilities,
		MustUseFacts:          false,
		MustCiteFacts:         false,
		BlockOnEmptyFacts:     false,
	}
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
	if strings.Contains(message, "新闻") || strings.Contains(message, "今天") ||
		strings.Contains(message, "最近") || strings.Contains(message, "发生了什么") {
		return "news_video"
	}
	return "current_event"
}

func inferNegativeFreshnessContentType(message string) string {
	switch {
	case containsAnyKeyword(message, []string{"虚构", "架空", "脑洞", "故事短片", "fiction"}):
		return "fiction"
	case containsAnyKeyword(message, []string{"改写", "润色", "扩写", "rewrite", "polish"}):
		return "rewrite"
	case containsAnyKeyword(message, []string{"产品宣传", "文案"}):
		return "marketing_script"
	default:
		return "creative"
	}
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
