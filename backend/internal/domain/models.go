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

type AnalysisResult struct {
	Stage        string          `json:"stage"`
	Summary      string          `json:"summary"`
	Attitude     string          `json:"attitude"`
	Psychology   string          `json:"psychology"`
	Suggestions  []string        `json:"suggestions"`
	ReplyOptions []string        `json:"replyOptions"`
	Rationale    string          `json:"rationale"`
	RiskNote     string          `json:"riskNote"`
	Disclaimer   string          `json:"disclaimer"`
	Conversation []Message       `json:"conversation"`
	Metrics      AnalysisMetrics `json:"metrics"`
	RawOCRText   string          `json:"rawOcrText,omitempty"`
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
