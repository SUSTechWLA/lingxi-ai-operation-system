package localtool

import (
	"math"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

type LipSyncTimelineBuilder struct{}

func NewLipSyncTimelineBuilder() *LipSyncTimelineBuilder {
	return &LipSyncTimelineBuilder{}
}

func (b *LipSyncTimelineBuilder) Build(frames []AudioFrame) []LipSyncFrame {
	result := make([]LipSyncFrame, 0, len(frames))
	prevOpen := 0.0
	for i, frame := range frames {
		mouth := "mouth_closed"
		targetOpen := 0.0
		switch {
		case frame.Silent || frame.RMS < 0.025:
			mouth = "mouth_closed"
			targetOpen = 0
		case frame.RMS < 0.12:
			if i%2 == 0 {
				mouth = "mouth_e"
			} else {
				mouth = "mouth_i"
			}
			targetOpen = 0.25
		case frame.RMS < 0.35:
			mouth = "mouth_a"
			targetOpen = 0.55
		default:
			if i%3 == 0 {
				mouth = "mouth_o"
			} else {
				mouth = "mouth_a"
			}
			targetOpen = 0.8
		}
		open := prevOpen*0.62 + targetOpen*0.38
		if targetOpen == 0 && open < 0.08 {
			open = 0
		}
		prevOpen = open
		result = append(result, LipSyncFrame{
			TimeSec: frame.TimeSec,
			Mouth:   mouth,
			Open:    roundFloat(open, 3),
		})
	}
	return result
}

func WriteLipSyncTimeline(outputDir string, frames []LipSyncFrame) (string, error) {
	path := filepath.Join(outputDir, "lip_sync_timeline.json")
	return path, writeJSONFile(path, frames)
}

type MotionTimelineBuilder struct{}

func NewMotionTimelineBuilder() *MotionTimelineBuilder {
	return &MotionTimelineBuilder{}
}

func (b *MotionTimelineBuilder) Build(script string, durationSec float64, policy MotionPolicy) []MotionEvent {
	if durationSec <= 0 {
		durationSec = 1
	}
	var events []MotionEvent
	if policy.AutoBreath {
		events = append(events, MotionEvent{TimeSec: 0, Motion: "idle_breath", Duration: roundFloat(durationSec, 3), Strength: 0.35})
	}
	if policy.AutoBlink {
		t := 1.1
		pattern := []float64{3.2, 4.1, 3.7, 4.8}
		i := 0
		for t < durationSec-0.2 {
			events = append(events, MotionEvent{TimeSec: roundFloat(t, 3), Motion: "blink", Duration: 0.16, Strength: 1})
			t += pattern[i%len(pattern)]
			i++
		}
	}
	if policy.SentenceNod {
		for _, pos := range punctuationPositions(script) {
			t := scriptPositionToTime(pos, script, durationSec)
			if t > 0.2 && t < durationSec-0.1 {
				events = append(events, MotionEvent{TimeSec: roundFloat(t, 3), Motion: "head_nod", Duration: 0.42, Strength: 0.55})
			}
		}
	}
	if policy.KeywordGesture {
		keywords := []string{"重点", "注意", "第一", "第二", "但是", "所以", "结论", "记住", "开源", "项目", "流程", "工具", "视频", "创作"}
		seen := map[string]bool{}
		for order, keyword := range keywords {
			idx := strings.Index(script, keyword)
			if idx < 0 || seen[keyword] {
				continue
			}
			seen[keyword] = true
			t := scriptPositionToTime(idx, script, durationSec)
			if t < 0.1 {
				t = 0.1
			}
			if t > durationSec-0.3 {
				t = durationSec - 0.3
			}
			motion := "gesture_point"
			if order%2 == 1 {
				motion = "gesture_present"
			}
			events = append(events, MotionEvent{TimeSec: roundFloat(t, 3), Motion: motion, Duration: 0.82, Strength: 0.72})
		}
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].TimeSec == events[j].TimeSec {
			return events[i].Motion < events[j].Motion
		}
		return events[i].TimeSec < events[j].TimeSec
	})
	return events
}

func WriteMotionTimeline(outputDir string, events []MotionEvent) (string, error) {
	path := filepath.Join(outputDir, "motion_timeline.json")
	return path, writeJSONFile(path, events)
}

func punctuationPositions(script string) []int {
	var positions []int
	for idx, r := range script {
		switch r {
		case '。', '！', '？', '；', '.', '!', '?', ';':
			positions = append(positions, idx)
		}
	}
	if len(positions) == 0 && strings.TrimSpace(script) != "" {
		positions = append(positions, len(script))
	}
	return positions
}

func scriptPositionToTime(byteIndex int, script string, durationSec float64) float64 {
	if durationSec <= 0 {
		return 0
	}
	total := utf8.RuneCountInString(script)
	if total <= 0 {
		return durationSec * 0.75
	}
	prefix := script
	if byteIndex >= 0 && byteIndex < len(script) {
		prefix = script[:byteIndex]
	}
	ratio := float64(utf8.RuneCountInString(prefix)+1) / float64(total+1)
	return math.Max(0.08, math.Min(durationSec-0.08, durationSec*ratio))
}

func WriteTimelineBundle(outputDir string, bundle avatarTimelineBundle) (string, error) {
	path := filepath.Join(outputDir, "avatar_timeline.json")
	return path, writeJSONFile(path, bundle)
}
