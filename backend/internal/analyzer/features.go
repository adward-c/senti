package analyzer

import (
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"senti/backend/internal/domain"
)

type FeatureVector struct {
	UserTurns         int
	TargetTurns       int
	UserChars         int
	TargetChars       int
	QuestionCount     int
	EmojiCount        int
	PositiveSignals   int
	NegativeSignals   int
	InviteSignals     int
	ConflictSignals   int
	DisclosureSignals int
	UserLatencyMin    float64
	TargetLatencyMin  float64
}

var (
	timePrefixPattern = regexp.MustCompile(`^(\[?\d{1,2}:\d{2}\]?)[\s-]*`)
	emojiPattern      = regexp.MustCompile(`[\x{1F300}-\x{1FAFF}]`)
)

func ParseConversation(input string) []domain.Message {
	lines := strings.Split(input, "\n")
	messages := make([]domain.Message, 0, len(lines))
	defaultSpeaker := "user"

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}

		message := domain.Message{Speaker: defaultSpeaker, Content: line}
		if match := timePrefixPattern.FindStringSubmatch(line); len(match) == 2 {
			if parsedTime, err := time.Parse("15:04", strings.Trim(match[1], "[]")); err == nil {
				timestamp := parsedTime
				message.Timestamp = &timestamp
			}
			message.Content = strings.TrimSpace(timePrefixPattern.ReplaceAllString(line, ""))
		}

		switch {
		case strings.HasPrefix(message.Content, "我:") || strings.HasPrefix(message.Content, "我："):
			message.Speaker = "user"
			message.Content = strings.TrimSpace(message.Content[2:])
		case strings.HasPrefix(strings.ToLower(message.Content), "user:"):
			message.Speaker = "user"
			message.Content = strings.TrimSpace(message.Content[5:])
		case strings.HasPrefix(message.Content, "对方:") || strings.HasPrefix(message.Content, "对方："):
			message.Speaker = "target"
			message.Content = strings.TrimSpace(message.Content[3:])
		case strings.HasPrefix(strings.ToLower(message.Content), "target:"):
			message.Speaker = "target"
			message.Content = strings.TrimSpace(message.Content[7:])
		default:
			message.Speaker = defaultSpeaker
			if defaultSpeaker == "user" {
				defaultSpeaker = "target"
			} else {
				defaultSpeaker = "user"
			}
		}

		messages = append(messages, message)
	}

	return messages
}

func ExtractFeatures(messages []domain.Message) FeatureVector {
	features := FeatureVector{
		UserLatencyMin:   3,
		TargetLatencyMin: 5,
	}

	positiveKeywords := []string{"哈哈", "好呀", "可以", "想", "愿意", "期待", "有趣", "不错", "喜欢", "嗯嗯"}
	negativeKeywords := []string{"算了", "忙", "再说", "不想", "别", "烦", "尴尬", "冷静", "没空"}
	inviteKeywords := []string{"见面", "吃饭", "喝咖啡", "电影", "周末", "哪天", "一起", "出去"}
	conflictKeywords := []string{"生气", "吵", "误会", "别联系", "拉黑", "烦死", "受不了"}
	disclosureKeywords := []string{"其实", "最近", "家里", "工作", "情绪", "压力", "有点累", "睡不着"}

	var userReplyGaps []float64
	var targetReplyGaps []float64
	for index, message := range messages {
		content := message.Content
		charCount := len([]rune(content))
		if message.Speaker == "user" {
			features.UserTurns++
			features.UserChars += charCount
		} else {
			features.TargetTurns++
			features.TargetChars += charCount
		}

		features.QuestionCount += strings.Count(content, "?") + strings.Count(content, "？")
		features.EmojiCount += len(emojiPattern.FindAllString(content, -1))
		features.PositiveSignals += keywordHits(content, positiveKeywords)
		features.NegativeSignals += keywordHits(content, negativeKeywords)
		features.InviteSignals += keywordHits(content, inviteKeywords)
		features.ConflictSignals += keywordHits(content, conflictKeywords)
		features.DisclosureSignals += keywordHits(content, disclosureKeywords)

		if index == 0 || message.Timestamp == nil || messages[index-1].Timestamp == nil {
			continue
		}

		gap := math.Abs(message.Timestamp.Sub(*messages[index-1].Timestamp).Minutes())
		if gap == 0 {
			gap = 1
		}
		if message.Speaker == "user" {
			userReplyGaps = append(userReplyGaps, gap)
		} else {
			targetReplyGaps = append(targetReplyGaps, gap)
		}
	}

	if len(userReplyGaps) > 0 {
		features.UserLatencyMin = median(userReplyGaps)
	}
	if len(targetReplyGaps) > 0 {
		features.TargetLatencyMin = median(targetReplyGaps)
	}

	return features
}

func DetectStage(features FeatureVector) string {
	switch {
	case features.ConflictSignals >= 1 || features.NegativeSignals >= 3:
		return "冲突/冷淡"
	case features.InviteSignals >= 2 && features.PositiveSignals >= 1:
		return "邀约窗口"
	case features.DisclosureSignals >= 2 && features.TargetChars > features.UserChars/2:
		return "舒适建立"
	case features.PositiveSignals >= 2 || features.EmojiCount >= 2:
		return "轻松升温"
	default:
		return "初识试探"
	}
}

func Quantize(features FeatureVector) map[string]float64 {
	totalTurns := maxFloat(float64(features.UserTurns+features.TargetTurns), 1)
	totalChars := maxFloat(float64(features.UserChars+features.TargetChars), 1)
	targetEngagement := float64(features.TargetTurns)/totalTurns + float64(features.TargetChars)/totalChars
	userInvestment := float64(features.UserTurns)/totalTurns + float64(features.UserChars)/totalChars

	sp := quantizeScore(0.2 + targetEngagement*0.35 + float64(features.DisclosureSignals)*0.05 + float64(features.InviteSignals)*0.05)
	fback := quantizeScore(0.2 + float64(features.TargetTurns)/totalTurns*0.45 + float64(features.PositiveSignals)*0.05)
	pface := quantizeScore(0.6 + float64(features.NegativeSignals)*0.05 - float64(features.PositiveSignals)*0.05 - float64(features.DisclosureSignals)*0.05)
	userDdepth := quantizeScore(0.5 + minFloat((features.TargetLatencyMin-features.UserLatencyMin)/20, 0.2))
	targetDdepth := quantizeScore(0.5 + minFloat((features.UserLatencyMin-features.TargetLatencyMin)/20, 0.2))
	cpIndex := quantizeScore(0.25 + float64(features.InviteSignals)*0.1 + float64(features.PositiveSignals)*0.05)
	gapEffect := quantizeCentered(float64(features.PositiveSignals-features.NegativeSignals) * 0.12)
	eev := quantizeCentered(float64(features.InviteSignals+features.PositiveSignals-features.ConflictSignals) * 0.1)
	risk := quantizeScore(0.2 + float64(features.ConflictSignals)*0.15 + float64(features.NegativeSignals)*0.05)

	return map[string]float64{
		"Sp":             sp,
		"Fback":          fback,
		"Pface":          pface,
		"UserInvestment": quantizeScore(0.2 + userInvestment*0.4),
		"UserDdepth":     userDdepth,
		"TargetDdepth":   targetDdepth,
		"CpIndex":        cpIndex,
		"GapEffect":      gapEffect,
		"EEV":            eev,
		"Risk":           risk,
	}
}

func BuildMetrics(features FeatureVector, params map[string]float64) domain.AnalysisMetrics {
	iviRaw := (params["Sp"] * math.Log(params["Fback"]+1)) / maxFloat(params["UserInvestment"]*maxFloat(params["Pface"], 0.1), 0.1)
	speRaw := (params["UserDdepth"] / maxFloat(params["TargetDdepth"], 0.1)) * (maxFloat(features.TargetLatencyMin, 1) / maxFloat(features.UserLatencyMin, 1))
	ewsRaw := (params["GapEffect"] * params["CpIndex"]) + params["EEV"] - 0.1

	return domain.AnalysisMetrics{
		IVI:    metric("IVI", iviRaw, scoreIndex(iviRaw, 0, 1.4), "对方真实投入度", iviLabel(iviRaw)),
		SPE:    metric("SPE", speRaw, scoreIndex(speRaw, 0.2, 1.8), "当前互动势能", speLabel(speRaw)),
		EWS:    metric("EWS", ewsRaw, scoreIndex(ewsRaw, -0.2, 1.2), "升温窗口强度", ewsLabel(ewsRaw)),
		Params: params,
		Signals: map[string]float64{
			"userTurns":       float64(features.UserTurns),
			"targetTurns":     float64(features.TargetTurns),
			"userChars":       float64(features.UserChars),
			"targetChars":     float64(features.TargetChars),
			"questionCount":   float64(features.QuestionCount),
			"positiveSignals": float64(features.PositiveSignals),
			"negativeSignals": float64(features.NegativeSignals),
			"inviteSignals":   float64(features.InviteSignals),
			"conflictSignals": float64(features.ConflictSignals),
			"disclosure":      float64(features.DisclosureSignals),
		},
	}
}

func keywordHits(content string, keywords []string) int {
	count := 0
	for _, keyword := range keywords {
		if strings.Contains(content, keyword) {
			count++
		}
	}
	return count
}

func median(values []float64) float64 {
	cloned := append([]float64(nil), values...)
	sort.Float64s(cloned)
	mid := len(cloned) / 2
	if len(cloned)%2 == 0 {
		return (cloned[mid-1] + cloned[mid]) / 2
	}
	return cloned[mid]
}

func metric(name string, raw, score float64, explanation, label string) domain.MetricValue {
	return domain.MetricValue{
		Name:        name,
		Raw:         round(raw),
		Score:       round(score),
		Label:       label,
		Explanation: explanation,
	}
}

func iviLabel(raw float64) string {
	switch {
	case raw >= 1.0:
		return "真实投入偏高"
	case raw >= 0.6:
		return "有兴趣但仍在观察"
	default:
		return "反馈保守，需先降温观察"
	}
}

func speLabel(raw float64) string {
	switch {
	case raw < 0.6:
		return "你方势能偏低"
	case raw <= 1.5:
		return "势能基本均衡"
	default:
		return "你方仍有主动空间"
	}
}

func ewsLabel(raw float64) string {
	switch {
	case raw > 0.8:
		return "可以轻推下一步"
	case raw > 0.3:
		return "先维持节奏再观察"
	default:
		return "窗口不足，不宜硬推"
	}
}

func scoreIndex(raw, min, max float64) float64 {
	return clamp(round(((raw-min)/(max-min))*10), 0, 10)
}

func quantizeScore(value float64) float64 {
	return clamp(roundToStep(value, 0.05), 0.1, 0.9)
}

func quantizeCentered(value float64) float64 {
	return clamp(roundToStep(value, 0.05), -0.9, 0.9)
}

func roundToStep(value, step float64) float64 {
	return math.Round(value/step) * step
}

func round(value float64) float64 {
	return math.Round(value*100) / 100
}

func clamp(value, low, high float64) float64 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
