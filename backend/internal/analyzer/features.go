package analyzer

import (
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"senti/backend/internal/domain"
)

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
		case trimPrefixedContent(&message, "我:", "我："):
			message.Speaker = "user"
		case trimPrefixedLowerContent(&message, "user:"):
			message.Speaker = "user"
		case trimPrefixedContent(&message, "对方:", "对方："):
			message.Speaker = "target"
		case trimPrefixedLowerContent(&message, "target:"):
			message.Speaker = "target"
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

func ExtractFeatures(messages []domain.Message) domain.FactFeatures {
	features := domain.FactFeatures{
		UserLatencyMin:   5,
		TargetLatencyMin: 5,
	}

	positiveKeywords := []string{"哈哈", "好呀", "好啊", "可以", "行啊", "愿意", "期待", "有趣", "不错", "喜欢", "嗯嗯", "好耶"}
	negativeKeywords := []string{"算了", "忙", "再说", "不想", "别", "烦", "尴尬", "冷静", "没空", "改天"}
	inviteKeywords := []string{"见面", "吃饭", "喝咖啡", "电影", "周末", "哪天", "一起", "出去", "坐坐", "出来", "线下"}
	conflictKeywords := []string{"生气", "吵", "误会", "别联系", "拉黑", "烦死", "受不了", "滚", "结束吧"}
	disclosureKeywords := []string{"其实", "最近", "家里", "工作", "情绪", "压力", "有点累", "睡不着", "状态", "脑子", "心情", "烦"}
	complianceKeywords := []string{"可以", "行", "好呀", "好啊", "没问题", "可以啊", "可行", "这个倒是可以"}
	boundaryKeywords := []string{"别联系", "拉黑", "先这样吧", "到此为止", "不方便", "不要再", "别这样"}
	deflectionKeywords := []string{"忙", "改天", "再说", "看情况", "回头", "有机会", "先这样", "以后再说"}
	humorKeywords := []string{"哈哈", "哈哈哈", "笑死", "逗你", "233", "~"}
	warmthKeywords := []string{"想你", "想", "期待", "喜欢", "抱抱", "晚安", "陪你", "在意"}

	var userReplyGaps []float64
	var targetReplyGaps []float64
	for index, message := range messages {
		content := message.Content
		charCount := len([]rune(content))
		questionCount := strings.Count(content, "?") + strings.Count(content, "？")
		if message.Speaker == "user" {
			features.UserTurns++
			features.UserChars += charCount
			features.UserQuestions += questionCount
		} else {
			features.TargetTurns++
			features.TargetChars += charCount
			features.TargetQuestions += questionCount
		}

		features.EmojiCount += len(emojiPattern.FindAllString(content, -1))
		features.PositiveSignals += keywordHits(content, positiveKeywords)
		features.NegativeSignals += keywordHits(content, negativeKeywords)
		features.InviteSignals += keywordHits(content, inviteKeywords)
		features.ConflictSignals += keywordHits(content, conflictKeywords)
		features.DisclosureSignals += keywordHits(content, disclosureKeywords)
		features.ComplianceSignals += keywordHits(content, complianceKeywords)
		features.BoundarySignals += keywordHits(content, boundaryKeywords)
		features.DeflectionSignals += keywordHits(content, deflectionKeywords)
		features.HumorSignals += keywordHits(content, humorKeywords)
		features.WarmthSignals += keywordHits(content, warmthKeywords)

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

func trimPrefixedContent(message *domain.Message, prefixes ...string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(message.Content, prefix) {
			message.Content = strings.TrimSpace(strings.TrimPrefix(message.Content, prefix))
			return true
		}
	}
	return false
}

func trimPrefixedLowerContent(message *domain.Message, prefix string) bool {
	if strings.HasPrefix(strings.ToLower(message.Content), prefix) {
		message.Content = strings.TrimSpace(message.Content[len(prefix):])
		return true
	}
	return false
}
