package analyzer

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"senti/backend/internal/domain"
	"senti/backend/internal/store"
)

type Service struct {
	repo   store.Repository
	rules  Rules
	ocr    OCRProvider
	kimi   *KimiClient
	logger *slog.Logger
}

func NewService(repo store.Repository, rules Rules, ocr OCRProvider, kimi *KimiClient, logger *slog.Logger) *Service {
	return &Service{repo: repo, rules: rules, ocr: ocr, kimi: kimi, logger: logger}
}

func (s *Service) StoreUpload(uploadDir string, fileName string, data []byte) (string, error) {
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		return "", err
	}
	ext := filepath.Ext(fileName)
	if ext == "" {
		ext = ".png"
	}
	path := filepath.Join(uploadDir, fmt.Sprintf("%s%s", uuid.NewString(), ext))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func (s *Service) AnalyzeText(ctx context.Context, sourceText string) (domain.AnalysisRecord, error) {
	return s.analyze(ctx, "text", sourceText, "", "")
}

func (s *Service) AnalyzeImage(ctx context.Context, imagePath string) (domain.AnalysisRecord, error) {
	ocrText, err := s.ocr.ExtractText(ctx, imagePath)
	if err != nil {
		return domain.AnalysisRecord{}, err
	}
	return s.analyze(ctx, "image", ocrText, imagePath, ocrText)
}

func (s *Service) analyze(ctx context.Context, inputType, sourceText, imagePath, rawOCRText string) (domain.AnalysisRecord, error) {
	messages := ParseConversation(sourceText)
	if len(messages) == 0 {
		return domain.AnalysisRecord{}, fmt.Errorf("unable to parse any conversation content")
	}

	features := ExtractFeatures(messages)
	stage := DetectStage(features)
	params := Quantize(features)
	metrics := BuildMetrics(features, params)

	record := domain.AnalysisRecord{
		ID:                 uuid.NewString(),
		InputType:          inputType,
		SourceText:         strings.TrimSpace(sourceText),
		ImagePath:          imagePath,
		StructuredMessages: messages,
		CreatedAt:          time.Now().UTC(),
		Result: domain.AnalysisResult{
			Stage:        stage,
			Metrics:      metrics,
			Conversation: messages,
			Disclaimer:   "仅供沟通参考，不构成心理诊断或关系结论。",
			RawOCRText:   rawOCRText,
		},
	}

	narrative, err := s.kimi.Generate(ctx, s.rules, record)
	if err != nil {
		s.logger.Warn("kimi generation failed, using fallback narrative", "error", err)
		narrative = fallbackNarrative(record)
	}

	record.Result.Summary = safeText(narrative.Summary, fallbackSummary(record))
	record.Result.Attitude = safeText(narrative.Attitude, fallbackAttitude(record))
	record.Result.Psychology = safeText(narrative.Psychology, fallbackPsychology(record))
	record.Result.Suggestions = normalizeList(narrative.Suggestions, fallbackSuggestions(record))
	record.Result.ReplyOptions = normalizeList(narrative.ReplyOptions, fallbackReplies(record))
	record.Result.Rationale = safeText(narrative.Rationale, fallbackRationale(record))
	record.Result.RiskNote = safeText(narrative.RiskNote, fallbackRisk(record))

	if err := s.repo.CreateAnalysis(ctx, record); err != nil {
		return domain.AnalysisRecord{}, err
	}
	return record, nil
}

func safeText(value, fallback string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return fallback
}

func normalizeList(items, fallback []string) []string {
	filtered := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			filtered = append(filtered, item)
		}
	}
	if len(filtered) == 0 {
		return fallback
	}
	if len(filtered) > 3 {
		return filtered[:3]
	}
	return filtered
}

func fallbackNarrative(record domain.AnalysisRecord) generatedNarrative {
	return generatedNarrative{
		Summary:      fallbackSummary(record),
		Attitude:     fallbackAttitude(record),
		Psychology:   fallbackPsychology(record),
		Suggestions:  fallbackSuggestions(record),
		ReplyOptions: fallbackReplies(record),
		Rationale:    fallbackRationale(record),
		RiskNote:     fallbackRisk(record),
	}
}

func fallbackSummary(record domain.AnalysisRecord) string {
	switch record.Result.Stage {
	case "邀约窗口":
		return "对方已经给出可继续推进的信号，但更适合轻推，不适合压迫式推进。"
	case "冲突/冷淡":
		return "当前更像是情绪或节奏失衡阶段，先稳住关系温度比继续施压更重要。"
	default:
		return "对话仍处在观察和试探阶段，重点是维持节奏感与自然感。"
	}
}

func fallbackAttitude(record domain.AnalysisRecord) string {
	switch {
	case record.Result.Metrics.IVI.Score >= 7:
		return "对方有一定真实投入，但还保留观察空间。"
	case record.Result.Metrics.IVI.Score >= 4:
		return "对方态度偏中性，愿意互动，但不算明显打开。"
	default:
		return "对方反馈偏保守，当前更需要降低期待、重新校准节奏。"
	}
}

func fallbackPsychology(record domain.AnalysisRecord) string {
	switch record.Result.Stage {
	case "舒适建立":
		return "对方更像是在确认安全感和稳定互动体验，愿意透露一些真实状态。"
	case "轻松升温":
		return "对方对互动有反应，但还在看你会不会过度推进。"
	case "冲突/冷淡":
		return "对方可能处于防御或疲惫状态，优先级是恢复舒适感而不是说服。"
	default:
		return "对方更多在做低风险观察，愿意接触，但尚未完全放下顾虑。"
	}
}

func fallbackSuggestions(record domain.AnalysisRecord) []string {
	switch record.Result.Stage {
	case "邀约窗口":
		return []string{
			"先用轻量提议试探，不要直接把行程压满。",
			"保持语气自然，给对方留选择空间。",
			"如果对方顺势接话，再往具体时间推进一步。",
		}
	case "冲突/冷淡":
		return []string{
			"先降低输出密度，别连发解释。",
			"用短句承接情绪，不急着证明自己。",
			"等关系温度回稳后，再讨论分歧点。",
		}
	default:
		return []string{
			"继续围绕对方有回应的话题延展，不要突然切大话题。",
			"保持一问一答之外的轻反馈，让节奏更像真实聊天。",
			"暂时不要急着索取结论，先看对方是否持续投入。",
		}
	}
}

func fallbackReplies(record domain.AnalysisRecord) []string {
	switch record.Result.Stage {
	case "邀约窗口":
		return []string{
			"这周找个轻松点的地方坐坐？别太正式，舒服点就行。",
			"你这个点子还挺适合线下聊，哪天顺路出来喝杯东西。",
			"先不搞复杂，找个你方便的时间见一面就行。",
		}
	case "冲突/冷淡":
		return []string{
			"我收到了，先不硬聊，等你状态舒服点我们再接着说。",
			"这会儿先别把话顶满，晚点你想说的时候再继续。",
			"我先退一步，不给你添堵，后面我们再慢慢捋。",
		}
	default:
		return []string{
			"你这个反应还挺有意思，我大概知道你在想什么了。",
			"行，那先按你这个节奏来，别一下子聊太满。",
			"你继续说，我想先听你这边更真实一点的想法。",
		}
	}
}

func fallbackRationale(record domain.AnalysisRecord) string {
	return "这组建议优先保护节奏和互动舒适度，避免在窗口不够成熟时过度推进，同时保留后续继续升温的空间。"
}

func fallbackRisk(record domain.AnalysisRecord) string {
	text := record.SourceText
	switch {
	case strings.Contains(text, "自杀") || strings.Contains(text, "伤害自己"):
		return "检测到高风险情绪表达，聊天建议不能替代专业支持，建议尽快联系可信任的人或专业援助资源。"
	case strings.Contains(text, "拉黑") || strings.Contains(text, "别联系"):
		return "当前存在明显边界信号，任何进一步推进都应以尊重对方意愿为前提。"
	default:
		return "如果对方连续降低回应密度，优先收缩投入，不要通过高压追问换取确定感。"
	}
}
