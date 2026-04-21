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

func (s *Service) KimiAvailability(ctx context.Context) (AvailabilityResult, error) {
	return s.kimi.CheckAvailability(ctx)
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
	semantic, err := s.kimi.GenerateSemanticLabels(ctx, s.rules, messages, features)
	if err != nil {
		s.logger.Warn("kimi semantic labeling failed", "error", err)
		return domain.AnalysisRecord{}, err
	}
	stage, stageCandidates, stageReason := DetectStage(features, semantic)
	params, paramTraces := Quantize(features, semantic, stage)
	metrics, metricInputs := BuildMetrics(features, params)
	strategy := DecideStrategy(stage, metrics, params, semantic, sourceText)

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
			Semantic:     semantic,
			Strategy:     strategy,
			Debug: domain.AnalysisDebug{
				FactFeatures:    features,
				SemanticLabels:  semantic,
				StageCandidates: stageCandidates,
				StageReason:     stageReason,
				ParamTraces:     paramTraces,
				MetricInputs:    metricInputs,
				Strategy:        strategy,
			},
			Disclaimer: "仅供沟通参考，不构成心理诊断或关系结论。",
			RawOCRText: rawOCRText,
		},
	}

	narrative, err := s.kimi.GenerateNarrative(ctx, s.rules, record)
	if err != nil {
		s.logger.Warn("kimi generation failed", "error", err)
		return domain.AnalysisRecord{}, err
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

func fallbackSummary(record domain.AnalysisRecord) string {
	ivi := record.Result.Metrics.IVI.Score
	spe := record.Result.Metrics.SPE.Score
	ews := record.Result.Metrics.EWS.Score
	switch record.Result.Stage {
	case "邀约窗口":
		if ews >= 7 {
			return "对方已经给出了比较明确的顺势信号，可以轻量推进到更具体的安排。"
		}
		return "窗口已经出现，但更适合顺势推进，不适合突然把节奏拉得太满。"
	case "冲突/冷淡":
		if spe < 4 {
			return "当前是明显的失衡阶段，你这边继续追问只会把局面压得更僵。"
		}
		return "当前更像是情绪或节奏失衡阶段，先稳住关系温度比继续施压更重要。"
	default:
		if ivi >= 7 && ews >= 5 {
			return "对方并不是没反应，只是还在留观察空间，继续顺着当前节奏推进会更稳。"
		}
		if ivi <= 4 {
			return "现在更像是低风险接触期，对方会回，但还没有给出足够强的投入信号。"
		}
		return "对话仍处在观察和试探阶段，重点是维持节奏感与自然感。"
	}
}

func fallbackAttitude(record domain.AnalysisRecord) string {
	stage := record.Result.Stage
	switch {
	case stage == "冲突/冷淡" && record.Result.Metrics.SPE.Score < 4:
		return "对方当前更偏防御或抽离，短时间内不太适合继续施压。"
	case record.Result.Metrics.IVI.Score >= 7:
		return "对方有一定真实投入，但还保留观察空间。"
	case record.Result.Metrics.EWS.Score >= 6:
		return "对方态度正在变得更配合，已经开始给你留推进的缝隙。"
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
		if record.Result.Metrics.EWS.Score >= 6 {
			return "对方已经开始把你放进更轻松的互动区间，对推进不会像之前那么敏感。"
		}
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
		if record.Result.Metrics.EWS.Score >= 7 {
			return []string{
				"把提议收敛成一个具体但轻量的选项，不要同时抛出太多安排。",
				"语气保持自然，重点是让对方容易答应，而不是显得你很急。",
				"如果对方继续配合，就把时间和地点再往前推半步。",
			}
		}
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
		if record.Result.Metrics.IVI.Score <= 4 {
			return []string{
				"先减少证明和解释，观察对方是否愿意继续给反馈。",
				"优先聊对方已经接住的话题，不要硬切到邀约或关系判断。",
				"把输出收短一点，先把互动压回更舒服的节奏。",
			}
		}
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
		if record.Result.Metrics.EWS.Score >= 7 {
			return []string{
				"那就别只停在嘴上了，这周挑个你方便的时间，我们轻松坐会儿。",
				"你这个状态挺适合出来透口气，找个安静点的地方就行。",
				"先不折腾复杂安排，挑个顺手的时间见一面。",
			}
		}
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
		if record.Result.Metrics.IVI.Score <= 4 {
			return []string{
				"行，那先顺着你现在这个节奏聊，不急着往前赶。",
				"我先不把话说太满，你有空再接着说也行。",
				"那先这样，等你这边状态松一点我们再往下聊。",
			}
		}
		return []string{
			"你这个反应还挺有意思，我大概知道你在想什么了。",
			"行，那先按你这个节奏来，别一下子聊太满。",
			"你继续说，我想先听你这边更真实一点的想法。",
		}
	}
}

func fallbackRationale(record domain.AnalysisRecord) string {
	if record.Result.Metrics.SPE.Score < 4 {
		return "当前建议偏收缩，是为了先修复互动里的失衡感，避免你这边继续加码后把局势推向更低位。"
	}
	if record.Result.Metrics.EWS.Score >= 7 {
		return "当前建议偏轻推进，是因为对方已经给出配合信号，此时用低压力方式落到具体行动，成功率更高。"
	}
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
