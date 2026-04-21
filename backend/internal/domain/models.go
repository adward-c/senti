package domain

import "time"

type Message struct {
	Speaker   string     `json:"speaker"`
	Content   string     `json:"content"`
	Timestamp *time.Time `json:"timestamp,omitempty"`
}

type MetricValue struct {
	Name        string  `json:"name"`
	Raw         float64 `json:"raw"`
	Score       float64 `json:"score"`
	Label       string  `json:"label"`
	Explanation string  `json:"explanation"`
}

type AnalysisMetrics struct {
	IVI     MetricValue        `json:"ivi"`
	SPE     MetricValue        `json:"spe"`
	EWS     MetricValue        `json:"ews"`
	Params  map[string]float64 `json:"params,omitempty"`
	Signals map[string]float64 `json:"signals,omitempty"`
}

type FactFeatures struct {
	UserTurns         int     `json:"userTurns"`
	TargetTurns       int     `json:"targetTurns"`
	UserChars         int     `json:"userChars"`
	TargetChars       int     `json:"targetChars"`
	UserQuestions     int     `json:"userQuestions"`
	TargetQuestions   int     `json:"targetQuestions"`
	EmojiCount        int     `json:"emojiCount"`
	PositiveSignals   int     `json:"positiveSignals"`
	NegativeSignals   int     `json:"negativeSignals"`
	InviteSignals     int     `json:"inviteSignals"`
	ConflictSignals   int     `json:"conflictSignals"`
	DisclosureSignals int     `json:"disclosureSignals"`
	ComplianceSignals int     `json:"complianceSignals"`
	BoundarySignals   int     `json:"boundarySignals"`
	DeflectionSignals int     `json:"deflectionSignals"`
	HumorSignals      int     `json:"humorSignals"`
	WarmthSignals     int     `json:"warmthSignals"`
	UserLatencyMin    float64 `json:"userLatencyMin"`
	TargetLatencyMin  float64 `json:"targetLatencyMin"`
}

type SemanticEvidence struct {
	Type    string  `json:"type"`
	Quote   string  `json:"quote"`
	Speaker string  `json:"speaker"`
	Score   float64 `json:"score"`
}

type SemanticLabels struct {
	StageCandidates []string           `json:"stageCandidates"`
	TopicType       string             `json:"topicType"`
	Signals         map[string]float64 `json:"signals"`
	Evidence        []SemanticEvidence `json:"evidence"`
}

type StrategyDecision struct {
	Type      string `json:"type"`
	Label     string `json:"label"`
	Reason    string `json:"reason"`
	RiskBlock bool   `json:"riskBlock"`
}

type ParamTrace struct {
	Value       float64  `json:"value"`
	Basis       float64  `json:"basis"`
	Adjustments []string `json:"adjustments"`
}

type AnalysisDebug struct {
	FactFeatures    FactFeatures          `json:"factFeatures"`
	SemanticLabels  SemanticLabels        `json:"semanticLabels"`
	StageCandidates []string              `json:"stageCandidates"`
	StageReason     string                `json:"stageReason"`
	ParamTraces     map[string]ParamTrace `json:"paramTraces"`
	MetricInputs    map[string]float64    `json:"metricInputs"`
	Strategy        StrategyDecision      `json:"strategy"`
}

type AnalysisResult struct {
	Stage        string           `json:"stage"`
	Summary      string           `json:"summary"`
	Attitude     string           `json:"attitude"`
	Psychology   string           `json:"psychology"`
	Suggestions  []string         `json:"suggestions"`
	ReplyOptions []string         `json:"replyOptions"`
	Rationale    string           `json:"rationale"`
	RiskNote     string           `json:"riskNote"`
	Disclaimer   string           `json:"disclaimer"`
	Conversation []Message        `json:"conversation"`
	Metrics      AnalysisMetrics  `json:"metrics"`
	Semantic     SemanticLabels   `json:"semantic"`
	Strategy     StrategyDecision `json:"strategy"`
	Debug        AnalysisDebug    `json:"debug"`
	RawOCRText   string           `json:"rawOcrText,omitempty"`
}

type AnalysisRecord struct {
	ID                 string         `json:"id"`
	InputType          string         `json:"inputType"`
	SourceText         string         `json:"sourceText"`
	ImagePath          string         `json:"imagePath,omitempty"`
	StructuredMessages []Message      `json:"structuredMessages"`
	Result             AnalysisResult `json:"result"`
	CreatedAt          time.Time      `json:"createdAt"`
}

type AnalysisSummary struct {
	ID        string    `json:"id"`
	InputType string    `json:"inputType"`
	Stage     string    `json:"stage"`
	Summary   string    `json:"summary"`
	CreatedAt time.Time `json:"createdAt"`
}
