package analyzer

import (
	"strings"
	"testing"

	"senti/backend/internal/domain"
)

func TestParseConversationSupportsChinesePrefixes(t *testing.T) {
	input := strings.Join([]string{
		"我：今天忙完了吗？",
		"对方：差不多，刚缓过来一点。",
		"我：那周末出来坐坐？",
		"对方：这个倒是可以。",
	}, "\n")

	messages := ParseConversation(input)
	if len(messages) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(messages))
	}
	if messages[0].Speaker != "user" || messages[0].Content != "今天忙完了吗？" {
		t.Fatalf("unexpected first message: %+v", messages[0])
	}
	if messages[1].Speaker != "target" || messages[1].Content != "差不多，刚缓过来一点。" {
		t.Fatalf("unexpected second message: %+v", messages[1])
	}
}

func TestDetectStageAndStrategyFavorInviteWindow(t *testing.T) {
	features := domain.FactFeatures{
		UserTurns:         3,
		TargetTurns:       3,
		UserChars:         20,
		TargetChars:       28,
		InviteSignals:     2,
		PositiveSignals:   3,
		DisclosureSignals: 2,
		ComplianceSignals: 1,
		UserLatencyMin:    6,
		TargetLatencyMin:  4,
	}
	semantic := domain.SemanticLabels{
		StageCandidates: []string{"invite_window", "comfort_building"},
		Signals: map[string]float64{
			"window_signal":      0.8,
			"backstage_exposure": 0.65,
			"compliance_signal":  0.7,
			"emotional_valence":  0.7,
			"receptiveness":      0.75,
		},
	}

	stage, candidates, _ := DetectStage(features, semantic)
	if stage != "邀约窗口" {
		t.Fatalf("expected 邀约窗口, got %s", stage)
	}
	if len(candidates) == 0 || candidates[0] != "邀约窗口" {
		t.Fatalf("unexpected candidates: %#v", candidates)
	}

	params, _ := Quantize(features, semantic, stage)
	metrics, _ := BuildMetrics(features, params)
	strategy := DecideStrategy(stage, metrics, params, semantic, "")
	if strategy.Type != "attack" {
		t.Fatalf("expected attack strategy, got %s", strategy.Type)
	}
}

func TestDetectStageAndStrategyFavorRiskBlock(t *testing.T) {
	features := domain.FactFeatures{
		UserTurns:         4,
		TargetTurns:       2,
		UserChars:         40,
		TargetChars:       8,
		NegativeSignals:   3,
		ConflictSignals:   1,
		BoundarySignals:   1,
		DeflectionSignals: 2,
		UserLatencyMin:    1,
		TargetLatencyMin:  30,
	}
	semantic := domain.SemanticLabels{
		StageCandidates: []string{"conflict_or_fadeout"},
		Signals: map[string]float64{
			"defensiveness": 0.85,
			"conflict_risk": 0.9,
		},
	}

	stage, _, _ := DetectStage(features, semantic)
	if stage != "冲突/冷淡" {
		t.Fatalf("expected 冲突/冷淡, got %s", stage)
	}

	params, _ := Quantize(features, semantic, stage)
	metrics, _ := BuildMetrics(features, params)
	strategy := DecideStrategy(stage, metrics, params, semantic, "别联系我了")
	if strategy.Type != "risk_block" {
		t.Fatalf("expected risk_block strategy, got %s", strategy.Type)
	}
}
