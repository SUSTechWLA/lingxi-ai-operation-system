package inputresolver

import "strings"

const (
	VideoTypeVoiceVisual = "voice_visual"
	VideoTypeAIGCShot    = "aigc_shot"
)

type Request struct {
	Topic       string   `json:"topic,omitempty"`
	VideoType   string   `json:"videoType,omitempty"`
	Platforms   []string `json:"platforms,omitempty"`
	Platform    string   `json:"platform,omitempty"`
	DurationSec int      `json:"durationSec,omitempty"`
	Language    string   `json:"language,omitempty"`
}

type Resolution struct {
	Topic              string   `json:"topic"`
	VideoType          string   `json:"videoType,omitempty"`
	Platforms          []string `json:"platforms"`
	DurationSec        int      `json:"durationSec"`
	Language           string   `json:"language"`
	NeedsClarification bool     `json:"needsClarification"`
	MissingFields      []string `json:"missingFields,omitempty"`
	Questions          []string `json:"questions,omitempty"`
}

func Resolve(req Request) Resolution {
	out := Resolution{
		Topic:       strings.TrimSpace(req.Topic),
		VideoType:   strings.TrimSpace(req.VideoType),
		Platforms:   normalizePlatforms(req),
		DurationSec: req.DurationSec,
		Language:    strings.TrimSpace(req.Language),
	}
	if len(out.Platforms) == 0 {
		out.Platforms = []string{"xiaohongshu"}
	}
	if out.DurationSec <= 0 {
		out.DurationSec = 90
	}
	if out.Language == "" {
		out.Language = "zh-CN"
	}
	if out.Topic == "" {
		out.NeedsClarification = true
		out.MissingFields = append(out.MissingFields, "topic")
		out.Questions = append(out.Questions, "请补充这条视频要表达的主题或核心观点。")
	}
	if out.VideoType != "" && out.VideoType != VideoTypeVoiceVisual && out.VideoType != VideoTypeAIGCShot {
		out.NeedsClarification = true
		out.MissingFields = append(out.MissingFields, "videoType")
		out.Questions = append(out.Questions, "请选择 voice_visual 或 aigc_shot。")
	}
	return out
}

func normalizePlatforms(req Request) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(req.Platforms)+1)
	add := func(value string) {
		value = normalizePlatform(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		out = append(out, value)
	}
	for _, platform := range req.Platforms {
		add(platform)
	}
	add(req.Platform)
	return out
}

func normalizePlatform(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "default":
		return ""
	case "小红书", "rednote", "xhs", "xiaohongshu":
		return "xiaohongshu"
	case "b站", "bilibili", "bili":
		return "bilibili"
	case "抖音", "douyin", "tiktok_cn":
		return "douyin"
	case "视频号", "wechat_channels", "wechat":
		return "wechat_channels"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}
