package analyzer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
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

func (c *KimiClient) Generate(ctx context.Context, rules Rules, record domain.AnalysisRecord) (generatedNarrative, error) {
	if c.apiKey == "" {
		return generatedNarrative{}, fmt.Errorf("kimi api key not configured")
	}

	systemPrompt := strings.Join([]string{
		"你是一个严谨的聊天分析助手，需要结合 chat-skills 规则输出克制、非诊断式的分析结果。",
		"不要夸大结论，不要把对方描述成被完全看透。只给倾向性判断。",
		"必须输出 JSON，不要输出 Markdown。",
		"可参考的规则摘要如下：",
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
			{"role": "user", "content": buildUserPrompt(record)},
		},
		"temperature":     0.3,
		"response_format": map[string]string{"type": "json_object"},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return generatedNarrative{}, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
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

func buildUserPrompt(record domain.AnalysisRecord) string {
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
核心指标：
- IVI: %.2f (%s)
- SPE: %.2f (%s)
- EWS: %.2f (%s)

结构化聊天记录：
%s`,
		record.Result.Stage,
		record.Result.Metrics.IVI.Score, record.Result.Metrics.IVI.Label,
		record.Result.Metrics.SPE.Score, record.Result.Metrics.SPE.Label,
		record.Result.Metrics.EWS.Score, record.Result.Metrics.EWS.Label,
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
