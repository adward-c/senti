package analyzer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"senti/backend/internal/config"
	"senti/backend/internal/domain"
)

type KimiClient struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
	logger  *slog.Logger
	mu      sync.RWMutex
}

type AvailabilityResult struct {
	OK              bool     `json:"ok"`
	BaseURL         string   `json:"baseUrl"`
	ResolvedBaseURL string   `json:"resolvedBaseUrl"`
	Model           string   `json:"model"`
	ModelAvailable  bool     `json:"modelAvailable"`
	AvailableModels []string `json:"availableModels"`
	Message         string   `json:"message"`
}

type semanticLabelResponse struct {
	StageCandidates []string `json:"stage_candidates"`
	TopicType       string   `json:"topic_type"`
	Signals         struct {
		WindowSignal      float64 `json:"window_signal"`
		Defensiveness     float64 `json:"defensiveness"`
		BackstageExposure float64 `json:"backstage_exposure"`
		ComplianceSignal  float64 `json:"compliance_signal"`
		EmotionalValence  float64 `json:"emotional_valence"`
		ConflictRisk      float64 `json:"conflict_risk"`
		Receptiveness     float64 `json:"receptiveness"`
	} `json:"signals"`
	Evidence []struct {
		Type    string  `json:"type"`
		Quote   string  `json:"quote"`
		Speaker string  `json:"speaker"`
		Score   float64 `json:"score"`
	} `json:"evidence"`
}

type generatedNarrative struct {
	Summary      string   `json:"summary"`
	Attitude     string   `json:"attitude"`
	Psychology   string   `json:"psychology"`
	Suggestions  []string `json:"suggestions"`
	ReplyOptions []string `json:"reply_options"`
	Rationale    string   `json:"rationale"`
	RiskNote     string   `json:"risk_note"`
}

func NewKimiClient(cfg config.Config, logger *slog.Logger) *KimiClient {
	return &KimiClient{
		baseURL: strings.TrimRight(cfg.KimiBaseURL, "/"),
		apiKey:  cfg.KimiAPIKey,
		model:   cfg.KimiModel,
		client:  &http.Client{Timeout: 45 * time.Second},
		logger:  logger,
	}
}

func (c *KimiClient) CheckAvailability(ctx context.Context) (AvailabilityResult, error) {
	if c.apiKey == "" {
		return AvailabilityResult{
			OK:      false,
			BaseURL: c.currentBaseURL(),
			Model:   c.model,
			Message: "Kimi API key not configured",
		}, fmt.Errorf("kimi api key not configured")
	}

	baseURL := c.currentBaseURL()
	result, err := c.checkModels(ctx, baseURL)
	if err == nil {
		c.setBaseURL(baseURL)
		result.BaseURL = baseURL
		result.ResolvedBaseURL = baseURL
		result.Model = c.model
		result.ModelAvailable = contains(result.AvailableModels, c.model)
		if result.ModelAvailable {
			result.Message = "Kimi API is available"
		} else {
			result.Message = "Kimi API is reachable, but configured model is not listed"
		}
		return result, nil
	}

	return AvailabilityResult{
		OK:              false,
		BaseURL:         baseURL,
		ResolvedBaseURL: baseURL,
		Model:           c.model,
		Message:         err.Error(),
	}, err
}

func (c *KimiClient) GenerateSemanticLabels(ctx context.Context, rules Rules, messages []domain.Message, features domain.FactFeatures) (domain.SemanticLabels, error) {
	if _, err := c.CheckAvailability(ctx); err != nil {
		return domain.SemanticLabels{}, err
	}

	systemPrompt := strings.Join([]string{
		"你是一个严谨的聊天语义标签器，需要结合 chat-skills 规则做阶段候选与语义信号提取。",
		"你只能输出结构化标签，不能直接给建议，不能输出最终结论。",
		"必须输出 JSON，不要输出 Markdown。",
		rules.InputRules,
		rules.StageModel,
	}, "\n\n")

	payload := map[string]any{
		"model": c.model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": buildSemanticPrompt(messages, features)},
		},
		"temperature":     0.1,
		"response_format": map[string]string{"type": "json_object"},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return domain.SemanticLabels{}, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.currentBaseURL()+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return domain.SemanticLabels{}, err
	}
	request.Header.Set("Authorization", "Bearer "+c.apiKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := c.client.Do(request)
	if err != nil {
		return domain.SemanticLabels{}, err
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return domain.SemanticLabels{}, err
	}
	if response.StatusCode >= 300 {
		c.logger.Warn("kimi request failed", "status", response.StatusCode, "body", string(responseBody))
		return domain.SemanticLabels{}, fmt.Errorf("kimi request failed with status %d", response.StatusCode)
	}

	var envelope struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(responseBody, &envelope); err != nil {
		return domain.SemanticLabels{}, err
	}
	if len(envelope.Choices) == 0 {
		return domain.SemanticLabels{}, fmt.Errorf("kimi returned no choices")
	}

	content := strings.TrimSpace(envelope.Choices[0].Message.Content)
	var raw semanticLabelResponse
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		start := strings.Index(content, "{")
		end := strings.LastIndex(content, "}")
		if start >= 0 && end > start {
			if nestedErr := json.Unmarshal([]byte(content[start:end+1]), &raw); nestedErr == nil {
				return normalizeSemanticLabels(raw), nil
			}
		}
		return domain.SemanticLabels{}, err
	}
	return normalizeSemanticLabels(raw), nil
}

func (c *KimiClient) GenerateNarrative(ctx context.Context, rules Rules, record domain.AnalysisRecord) (generatedNarrative, error) {
	if _, err := c.CheckAvailability(ctx); err != nil {
		return generatedNarrative{}, err
	}

	systemPrompt := strings.Join([]string{
		"你是一个严谨的聊天分析助手，需要结合 chat-skills 规则输出克制、非诊断式的分析结果。",
		"不要夸大结论，不要把对方描述成被完全看透。只给倾向性判断。",
		"必须输出 JSON，不要输出 Markdown。",
		"必须遵守已给定的阶段、策略与风险边界，不要自作主张改变方向。",
		rules.InputRules,
		rules.Quantizer,
		rules.Algorithm,
		rules.OutputRules,
		rules.StageModel,
	}, "\n\n")

	payload := map[string]any{
		"model": c.model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": buildNarrativePrompt(record)},
		},
		"temperature":     0.25,
		"response_format": map[string]string{"type": "json_object"},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return generatedNarrative{}, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.currentBaseURL()+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return generatedNarrative{}, err
	}
	request.Header.Set("Authorization", "Bearer "+c.apiKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := c.client.Do(request)
	if err != nil {
		return generatedNarrative{}, err
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return generatedNarrative{}, err
	}
	if response.StatusCode >= 300 {
		c.logger.Warn("kimi request failed", "status", response.StatusCode, "body", string(responseBody))
		return generatedNarrative{}, fmt.Errorf("kimi request failed with status %d", response.StatusCode)
	}

	var envelope struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(responseBody, &envelope); err != nil {
		return generatedNarrative{}, err
	}
	if len(envelope.Choices) == 0 {
		return generatedNarrative{}, fmt.Errorf("kimi returned no choices")
	}

	content := strings.TrimSpace(envelope.Choices[0].Message.Content)
	var narrative generatedNarrative
	if err := json.Unmarshal([]byte(content), &narrative); err != nil {
		start := strings.Index(content, "{")
		end := strings.LastIndex(content, "}")
		if start >= 0 && end > start {
			if nestedErr := json.Unmarshal([]byte(content[start:end+1]), &narrative); nestedErr == nil {
				return narrative, nil
			}
		}
		return generatedNarrative{}, err
	}
	return narrative, nil
}

func buildSemanticPrompt(messages []domain.Message, features domain.FactFeatures) string {
	return fmt.Sprintf(`请基于以下聊天记录和事实特征，只输出语义标签 JSON：
{
  "stage_candidates": ["stranger_contact", "warm_up", "comfort_building", "invite_window", "conflict_or_fadeout"],
  "topic_type": "话题类型，短词即可",
  "signals": {
    "window_signal": 0.0,
    "defensiveness": 0.0,
    "backstage_exposure": 0.0,
    "compliance_signal": 0.0,
    "emotional_valence": 0.0,
    "conflict_risk": 0.0,
    "receptiveness": 0.0
  },
  "evidence": [
    {"type": "window_signal", "quote": "原句", "speaker": "target", "score": 0.0}
  ]
}

要求：
1. 所有 signals 范围必须为 0 到 1。
2. stage_candidates 只允许使用给定枚举，按可能性从高到低排序。
3. evidence 最多 5 条，只保留最关键证据。
4. 不要输出建议、回复话术或最终结论。

事实特征：
- user_turns: %d
- target_turns: %d
- user_chars: %d
- target_chars: %d
- user_questions: %d
- target_questions: %d
- positive_signals: %d
- negative_signals: %d
- invite_signals: %d
- conflict_signals: %d
- disclosure_signals: %d
- compliance_signals: %d
- boundary_signals: %d
- deflection_signals: %d
- humor_signals: %d
- warmth_signals: %d

结构化聊天记录：
%s`,
		features.UserTurns,
		features.TargetTurns,
		features.UserChars,
		features.TargetChars,
		features.UserQuestions,
		features.TargetQuestions,
		features.PositiveSignals,
		features.NegativeSignals,
		features.InviteSignals,
		features.ConflictSignals,
		features.DisclosureSignals,
		features.ComplianceSignals,
		features.BoundarySignals,
		features.DeflectionSignals,
		features.HumorSignals,
		features.WarmthSignals,
		messagesAsText(messages),
	)
}

func buildNarrativePrompt(record domain.AnalysisRecord) string {
	return fmt.Sprintf(`请基于以下结构化数据，输出 JSON：
{
  "summary": "一句话总结当前互动局面",
  "attitude": "对方态度倾向",
  "psychology": "对方当前更可能的心理状态",
  "suggestions": ["建议1", "建议2", "建议3"],
  "reply_options": ["回复1", "回复2", "回复3"],
  "rationale": "说明建议背后的原因",
  "risk_note": "如有必要，提醒边界与风险"
}

要求：
1. 不使用诊断式措辞。
2. 不输出任何额外字段。
3. 建议要短、自然、可执行。

当前阶段：%s
策略决策：
- type: %s
- label: %s
- reason: %s
语义标签：
- topic_type: %s
- signals: %s
关键证据句：
%s

核心指标：
- IVI: %.2f (%s)
- SPE: %.2f (%s)
- EWS: %.2f (%s)

量化参数：
%s

结构化聊天记录：
%s`,
		record.Result.Stage,
		record.Result.Strategy.Type,
		record.Result.Strategy.Label,
		record.Result.Strategy.Reason,
		record.Result.Semantic.TopicType,
		formatSignalMap(record.Result.Semantic.Signals),
		formatEvidence(record.Result.Semantic.Evidence),
		record.Result.Metrics.IVI.Score, record.Result.Metrics.IVI.Label,
		record.Result.Metrics.SPE.Score, record.Result.Metrics.SPE.Label,
		record.Result.Metrics.EWS.Score, record.Result.Metrics.EWS.Label,
		formatSignalMap(record.Result.Metrics.Params),
		messagesAsText(record.StructuredMessages),
	)
}

func messagesAsText(messages []domain.Message) string {
	lines := make([]string, 0, len(messages))
	for _, message := range messages {
		lines = append(lines, fmt.Sprintf("%s: %s", message.Speaker, message.Content))
	}
	return strings.Join(lines, "\n")
}

func formatSignalMap(values map[string]float64) string {
	if len(values) == 0 {
		return "{}"
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, fmt.Sprintf("%s=%.2f", key, values[key]))
	}
	return strings.Join(lines, ", ")
}

func formatEvidence(evidence []domain.SemanticEvidence) string {
	if len(evidence) == 0 {
		return "- 无明显证据句"
	}
	lines := make([]string, 0, len(evidence))
	for _, item := range evidence {
		lines = append(lines, fmt.Sprintf("- [%s/%s/%.2f] %s", item.Type, item.Speaker, item.Score, item.Quote))
	}
	return strings.Join(lines, "\n")
}

func normalizeSemanticLabels(raw semanticLabelResponse) domain.SemanticLabels {
	labels := domain.SemanticLabels{
		StageCandidates: normalizeStageCandidates(raw.StageCandidates),
		TopicType:       strings.TrimSpace(raw.TopicType),
		Signals: map[string]float64{
			"window_signal":      clamp(raw.Signals.WindowSignal, 0, 1),
			"defensiveness":      clamp(raw.Signals.Defensiveness, 0, 1),
			"backstage_exposure": clamp(raw.Signals.BackstageExposure, 0, 1),
			"compliance_signal":  clamp(raw.Signals.ComplianceSignal, 0, 1),
			"emotional_valence":  clamp(raw.Signals.EmotionalValence, 0, 1),
			"conflict_risk":      clamp(raw.Signals.ConflictRisk, 0, 1),
			"receptiveness":      clamp(raw.Signals.Receptiveness, 0, 1),
		},
		Evidence: make([]domain.SemanticEvidence, 0, len(raw.Evidence)),
	}
	if labels.TopicType == "" {
		labels.TopicType = "日常互动"
	}

	for _, item := range raw.Evidence {
		quote := strings.TrimSpace(item.Quote)
		if quote == "" {
			continue
		}
		labels.Evidence = append(labels.Evidence, domain.SemanticEvidence{
			Type:    strings.TrimSpace(item.Type),
			Quote:   quote,
			Speaker: strings.TrimSpace(item.Speaker),
			Score:   round(clamp(item.Score, 0, 1)),
		})
		if len(labels.Evidence) == 5 {
			break
		}
	}

	return labels
}

func (c *KimiClient) checkModels(ctx context.Context, baseURL string) (AvailabilityResult, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		return AvailabilityResult{}, err
	}
	request.Header.Set("Authorization", "Bearer "+c.apiKey)

	response, err := c.client.Do(request)
	if err != nil {
		return AvailabilityResult{}, err
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return AvailabilityResult{}, err
	}
	if response.StatusCode >= 300 {
		c.logger.Warn("kimi availability check failed", "status", response.StatusCode, "base_url", baseURL, "body", string(responseBody))
		return AvailabilityResult{}, fmt.Errorf("kimi availability check failed with status %d", response.StatusCode)
	}

	var envelope struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(responseBody, &envelope); err != nil {
		return AvailabilityResult{}, err
	}

	models := make([]string, 0, len(envelope.Data))
	for _, item := range envelope.Data {
		id := strings.TrimSpace(item.ID)
		if id != "" {
			models = append(models, id)
		}
	}
	sort.Strings(models)

	return AvailabilityResult{
		OK:              true,
		ResolvedBaseURL: baseURL,
		AvailableModels: models,
	}, nil
}

func (c *KimiClient) currentBaseURL() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.baseURL
}

func (c *KimiClient) setBaseURL(baseURL string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.baseURL = strings.TrimRight(baseURL, "/")
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
