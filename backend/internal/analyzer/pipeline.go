package analyzer

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"senti/backend/internal/domain"
)

var canonicalStages = []string{
	"stranger_contact",
	"warm_up",
	"comfort_building",
	"invite_window",
	"conflict_or_fadeout",
}

func DetectStage(features domain.FactFeatures, semantic domain.SemanticLabels) (string, []string, string) {
	ruleScores := stageRuleScores(features, semantic)
	fused := make(map[string]float64, len(ruleScores))
	for stage, score := range ruleScores {
		fused[stage] = score
	}

	for index, stage := range semantic.StageCandidates {
		weight := 0.35 - float64(index)*0.08
		if weight < 0.1 {
			weight = 0.1
		}
		fused[stage] += weight
	}

	ranked := rankStages(fused)
	stage := ranked[0]
	reason := fmt.Sprintf(
		"规则候选优先级为 %s，语义候选为 %s，最终融合为 %s。",
		strings.Join(rankStages(ruleScores), " > "),
		strings.Join(semantic.StageCandidates, " > "),
		stageLabel(stage),
	)

	return stageLabel(stage), stageLabels(ranked), reason
}

func Quantize(features domain.FactFeatures, semantic domain.SemanticLabels, stage string) (map[string]float64, map[string]domain.ParamTrace) {
	totalTurns := maxFloat(float64(features.UserTurns+features.TargetTurns), 1)
	totalChars := maxFloat(float64(features.UserChars+features.TargetChars), 1)
	targetTurnRatio := float64(features.TargetTurns) / totalTurns
	targetCharRatio := float64(features.TargetChars) / totalChars
	userTurnRatio := float64(features.UserTurns) / totalTurns
	userCharRatio := float64(features.UserChars) / totalChars

	windowSignal := signalValue(semantic.Signals, "window_signal")
	defensiveSignal := signalValue(semantic.Signals, "defensiveness")
	backstageSignal := signalValue(semantic.Signals, "backstage_exposure")
	complianceSignal := signalValue(semantic.Signals, "compliance_signal")
	emotionalValence := signalValue(semantic.Signals, "emotional_valence")
	conflictRisk := signalValue(semantic.Signals, "conflict_risk")
	receptiveness := signalValue(semantic.Signals, "receptiveness")

	params := make(map[string]float64)
	traces := make(map[string]domain.ParamTrace)

	addParam(params, traces, "Sp", 0.5, []paramAdjustment{
		adj(targetTurnRatio >= 0.45, 0.1, "对方回合占比高，显示性投入上调"),
		adj(targetCharRatio >= 0.45, 0.1, "对方字数占比高，信息投入上调"),
		adj(features.DisclosureSignals >= 2, 0.1, "出现真实状态暴露，投入上调"),
		adj(features.InviteSignals >= 1, 0.1, "出现邀约/线下信号，投入上调"),
		adj(windowSignal >= 0.6, 0.05, "语义层识别到窗口信号，投入微调上调"),
		adj(conflictRisk >= 0.65, -0.1, "冲突风险偏高，投入回调"),
		adj(features.BoundarySignals >= 1, -0.15, "出现边界表达，投入下调"),
	})

	addParam(params, traces, "Fback", 0.5, []paramAdjustment{
		adj(targetTurnRatio >= 0.45, 0.1, "对方持续接话，反馈增益上调"),
		adj(features.TargetQuestions >= 1, 0.05, "对方有问回，反馈增益微调上调"),
		adj(features.PositiveSignals >= 2, 0.1, "积极反馈较多，反馈增益上调"),
		adj(receptiveness >= 0.6, 0.05, "语义层判断接纳度较高，反馈增益上调"),
		adj(features.DeflectionSignals >= 2, -0.1, "存在多次模糊搪塞，反馈增益下调"),
	})

	addParam(params, traces, "Pface", 0.5, []paramAdjustment{
		adj(features.NegativeSignals >= 2, 0.1, "负向表达较多，面子阻力上调"),
		adj(features.DeflectionSignals >= 1, 0.1, "存在推脱/拖延表达，面子阻力上调"),
		adj(defensiveSignal >= 0.6, 0.1, "语义层识别到防御姿态，面子阻力上调"),
		adj(features.DisclosureSignals >= 2, -0.1, "有真实暴露，面子阻力下调"),
		adj(backstageSignal >= 0.6, -0.1, "后台暴露明显，面子阻力下调"),
	})

	addParam(params, traces, "Backstage", 0.35, []paramAdjustment{
		adj(features.DisclosureSignals >= 1, 0.1, "出现状态和私域信息暴露，后台暴露上调"),
		adj(features.DisclosureSignals >= 3, 0.1, "连续暴露真实状态，后台暴露继续上调"),
		adj(backstageSignal >= 0.6, 0.1, "语义层判断后台暴露明显，后台暴露上调"),
		adj(features.BoundarySignals >= 1, -0.1, "边界表达出现，后台暴露下调"),
	})

	addParam(params, traces, "Cp", 0.3, []paramAdjustment{
		adj(features.ComplianceSignals >= 1, 0.1, "对方接住轻量提议，服从阶梯上调"),
		adj(features.InviteSignals >= 1 && complianceSignal >= 0.55, 0.1, "邀约与配合信号同时出现，服从阶梯上调"),
		adj(complianceSignal >= 0.7, 0.05, "语义层识别到配合倾向，服从阶梯微调上调"),
		adj(features.BoundarySignals >= 1, -0.15, "出现边界信号，服从阶梯下调"),
	})

	addParam(params, traces, "Noise", 0.3, []paramAdjustment{
		adj(features.DeflectionSignals >= 1, 0.1, "存在改天/再说等搪塞词，言语掩饰上调"),
		adj(features.DeflectionSignals >= 3, 0.1, "搪塞密度较高，言语掩饰继续上调"),
		adj(defensiveSignal >= 0.55, 0.05, "防御感偏高，言语掩饰微调上调"),
		adj(backstageSignal >= 0.6, -0.05, "真实暴露明显，言语掩饰下调"),
	})

	addParam(params, traces, "Ve", 0.5, []paramAdjustment{
		adj(features.PositiveSignals >= 2, 0.1, "积极信号较多，情绪效价上调"),
		adj(features.HumorSignals >= 1, 0.05, "存在轻松玩笑，情绪效价微调上调"),
		adj(emotionalValence >= 0.6, 0.1, "语义层判断情绪偏正向，情绪效价上调"),
		adj(features.ConflictSignals >= 1, -0.15, "冲突信号出现，情绪效价下调"),
		adj(features.BoundarySignals >= 1, -0.1, "边界表达出现，情绪效价下调"),
	})

	addParam(params, traces, "Rlatency", 0.5, []paramAdjustment{
		adj(features.TargetLatencyMin <= features.UserLatencyMin*1.1, 0.1, "对方回复速度接近或更快，响应延迟上调"),
		adj(features.TargetLatencyMin > features.UserLatencyMin*1.8, -0.15, "对方明显更慢，响应延迟下调"),
		adj(features.TargetLatencyMin > 20, -0.1, "对方整体回复偏慢，响应延迟下调"),
	})

	addParam(params, traces, "UserInvestment", 0.4, []paramAdjustment{
		adj(userTurnRatio >= 0.5, 0.1, "你方回合占比较高，投入上调"),
		adj(userCharRatio >= 0.55, 0.1, "你方字数显著更多，投入继续上调"),
		adj(userTurnRatio < 0.4, -0.05, "你方整体投入不高，投入微调下调"),
	})

	addParam(params, traces, "UserDdepth", 0.5, []paramAdjustment{
		adj(features.UserLatencyMin >= features.TargetLatencyMin*1.2, 0.1, "你方节奏更慢，战略纵深上调"),
		adj(features.UserLatencyMin < features.TargetLatencyMin*0.8, -0.1, "你方回复更快，战略纵深下调"),
		adj(stage == "冲突/冷淡", -0.05, "当前阶段偏冷，主动方纵深下调"),
	})

	addParam(params, traces, "TargetDdepth", 0.5, []paramAdjustment{
		adj(features.TargetLatencyMin >= features.UserLatencyMin*1.2, 0.1, "对方节奏更慢，对方纵深上调"),
		adj(features.TargetLatencyMin < features.UserLatencyMin*0.8, -0.1, "对方回复更快，对方纵深下调"),
		adj(defensiveSignal >= 0.6, 0.05, "防御姿态明显，对方纵深微调上调"),
	})

	addParam(params, traces, "CpIndex", 0.25, []paramAdjustment{
		adj(params["Cp"] >= 0.5, 0.15, "服从阶梯达到中高位，服从指数上调"),
		adj(windowSignal >= 0.65, 0.1, "窗口信号明显，服从指数上调"),
		adj(features.BoundarySignals >= 1, -0.15, "边界表达出现，服从指数下调"),
	})

	addParam(params, traces, "GapEffect", 0, []paramAdjustment{
		adj(features.PositiveSignals > features.NegativeSignals, 0.15, "积极信号多于负向信号，情绪落差上调"),
		adj(features.HumorSignals >= 1, 0.05, "轻松玩笑出现，情绪落差微调上调"),
		adj(features.ConflictSignals >= 1, -0.2, "存在冲突信号，情绪落差下调"),
		adj(conflictRisk >= 0.6, -0.1, "语义层判断冲突风险偏高，情绪落差下调"),
	}, true)

	addParam(params, traces, "EEV", 0.1, []paramAdjustment{
		adj(windowSignal >= 0.6, 0.15, "窗口信号较强，预期情绪价值上调"),
		adj(receptiveness >= 0.6, 0.1, "接纳度较高，预期情绪价值上调"),
		adj(features.ConflictSignals >= 1, -0.15, "冲突信号出现，预期情绪价值下调"),
		adj(features.DeflectionSignals >= 2, -0.05, "推脱偏多，预期情绪价值下调"),
	}, true)

	addParam(params, traces, "Risk", 0.2, []paramAdjustment{
		adj(features.ConflictSignals >= 1, 0.2, "冲突信号出现，风险显著上调"),
		adj(features.BoundarySignals >= 1, 0.2, "边界信号出现，风险显著上调"),
		adj(conflictRisk >= 0.65, 0.15, "语义层判断冲突风险高，风险上调"),
		adj(windowSignal >= 0.6 && features.ConflictSignals == 0, -0.05, "窗口信号较好且无冲突，风险微调下调"),
	})

	return params, traces
}

func BuildMetrics(features domain.FactFeatures, params map[string]float64) (domain.AnalysisMetrics, map[string]float64) {
	iviRaw := (params["Sp"] * math.Log(params["Fback"]+1)) / maxFloat(params["UserInvestment"]*maxFloat(params["Pface"], 0.1), 0.1)
	speRaw := (params["UserDdepth"] / maxFloat(params["TargetDdepth"], 0.1)) * (maxFloat(features.TargetLatencyMin, 1) / maxFloat(features.UserLatencyMin, 1))
	ewsRaw := (params["GapEffect"] * params["CpIndex"]) + params["EEV"] - (params["Risk"] * 0.35)

	metricInputs := map[string]float64{
		"sp":             params["Sp"],
		"fback":          params["Fback"],
		"userInvestment": params["UserInvestment"],
		"pface":          params["Pface"],
		"userDdepth":     params["UserDdepth"],
		"targetDdepth":   params["TargetDdepth"],
		"targetLatency":  features.TargetLatencyMin,
		"userLatency":    features.UserLatencyMin,
		"gapEffect":      params["GapEffect"],
		"cpIndex":        params["CpIndex"],
		"eev":            params["EEV"],
		"risk":           params["Risk"],
	}

	return domain.AnalysisMetrics{
		IVI:    metric("IVI", iviRaw, scoreIndex(iviRaw, 0, 1.4), "对方真实投入度", iviLabel(iviRaw)),
		SPE:    metric("SPE", speRaw, scoreIndex(speRaw, 0.2, 1.8), "当前互动势能", speLabel(speRaw)),
		EWS:    metric("EWS", ewsRaw, scoreIndex(ewsRaw, -0.2, 1.2), "升温窗口强度", ewsLabel(ewsRaw)),
		Params: params,
		Signals: map[string]float64{
			"userTurns":         float64(features.UserTurns),
			"targetTurns":       float64(features.TargetTurns),
			"userChars":         float64(features.UserChars),
			"targetChars":       float64(features.TargetChars),
			"userQuestions":     float64(features.UserQuestions),
			"targetQuestions":   float64(features.TargetQuestions),
			"positiveSignals":   float64(features.PositiveSignals),
			"negativeSignals":   float64(features.NegativeSignals),
			"inviteSignals":     float64(features.InviteSignals),
			"conflictSignals":   float64(features.ConflictSignals),
			"disclosureSignals": float64(features.DisclosureSignals),
			"complianceSignals": float64(features.ComplianceSignals),
			"boundarySignals":   float64(features.BoundarySignals),
			"deflectionSignals": float64(features.DeflectionSignals),
			"humorSignals":      float64(features.HumorSignals),
			"warmthSignals":     float64(features.WarmthSignals),
		},
	}, metricInputs
}

func DecideStrategy(stage string, metrics domain.AnalysisMetrics, params map[string]float64, semantic domain.SemanticLabels, sourceText string) domain.StrategyDecision {
	conflictRisk := signalValue(semantic.Signals, "conflict_risk")
	hasHighRiskWords := strings.Contains(sourceText, "自杀") || strings.Contains(sourceText, "伤害自己")

	switch {
	case hasHighRiskWords:
		return domain.StrategyDecision{
			Type:      "risk_block",
			Label:     "风险阻断",
			Reason:    "出现高风险情绪或自伤表达，普通聊天建议不再适用，应优先转向现实支持。",
			RiskBlock: true,
		}
	case params["Risk"] >= 0.7 || conflictRisk >= 0.7:
		return domain.StrategyDecision{
			Type:      "risk_block",
			Label:     "风险阻断",
			Reason:    "冲突或边界风险偏高，当前不适合继续推进，应先停止施压。",
			RiskBlock: true,
		}
	case metrics.SPE.Score < 4 || metrics.IVI.Raw < 0.5:
		return domain.StrategyDecision{
			Type:      "reset",
			Label:     "关系重置",
			Reason:    "当前势能偏低或真实投入不足，更适合体面收缩与重建节奏，而不是继续加码。",
			RiskBlock: false,
		}
	case stage != "冲突/冷淡" && metrics.IVI.Raw > 0.8 && (metrics.EWS.Raw > 0.8 || (stage == "邀约窗口" && signalValue(semantic.Signals, "window_signal") >= 0.7 && metrics.EWS.Score >= 4)):
		return domain.StrategyDecision{
			Type:      "attack",
			Label:     "轻推进",
			Reason:    "窗口已经形成，且真实投入不低，可以用低压力方式往具体安排推进一步。",
			RiskBlock: false,
		}
	default:
		return domain.StrategyDecision{
			Type:      "pull",
			Label:     "维持拉扯",
			Reason:    "当前更适合继续积累舒适感和反馈密度，通过轻松互动观察窗口是否继续扩大。",
			RiskBlock: false,
		}
	}
}

type paramAdjustment struct {
	enabled bool
	delta   float64
	reason  string
}

func adj(enabled bool, delta float64, reason string) paramAdjustment {
	return paramAdjustment{enabled: enabled, delta: delta, reason: reason}
}

func addParam(params map[string]float64, traces map[string]domain.ParamTrace, key string, basis float64, adjustments []paramAdjustment, centered ...bool) {
	value := basis
	trace := domain.ParamTrace{
		Basis:       round(basis),
		Adjustments: make([]string, 0, len(adjustments)),
	}
	for _, adjustment := range adjustments {
		if !adjustment.enabled {
			continue
		}
		value += adjustment.delta
		trace.Adjustments = append(trace.Adjustments, fmt.Sprintf("%+.2f %s", adjustment.delta, adjustment.reason))
	}

	if len(centered) > 0 && centered[0] {
		value = quantizeCentered(value)
	} else {
		value = quantizeScore(value)
	}
	trace.Value = value
	params[key] = value
	traces[key] = trace
}

func stageRuleScores(features domain.FactFeatures, semantic domain.SemanticLabels) map[string]float64 {
	scores := map[string]float64{
		"stranger_contact":    0.2,
		"warm_up":             0.2,
		"comfort_building":    0.2,
		"invite_window":       0.2,
		"conflict_or_fadeout": 0.2,
	}

	if features.ConflictSignals >= 1 || features.BoundarySignals >= 1 || signalValue(semantic.Signals, "conflict_risk") >= 0.6 {
		scores["conflict_or_fadeout"] += 0.6
	}
	if features.InviteSignals >= 1 || signalValue(semantic.Signals, "window_signal") >= 0.6 {
		scores["invite_window"] += 0.45
	}
	if features.DisclosureSignals >= 2 || signalValue(semantic.Signals, "backstage_exposure") >= 0.6 {
		scores["comfort_building"] += 0.4
	}
	if features.PositiveSignals >= 2 || features.HumorSignals >= 1 || signalValue(semantic.Signals, "emotional_valence") >= 0.55 {
		scores["warm_up"] += 0.35
	}
	if features.TargetTurns <= 2 && features.DisclosureSignals == 0 && features.InviteSignals == 0 {
		scores["stranger_contact"] += 0.35
	}
	if features.TargetQuestions >= 1 || signalValue(semantic.Signals, "receptiveness") >= 0.55 {
		scores["warm_up"] += 0.2
	}
	if features.ComplianceSignals >= 1 {
		scores["invite_window"] += 0.15
	}

	return scores
}

func rankStages(scores map[string]float64) []string {
	type item struct {
		stage string
		score float64
	}
	items := make([]item, 0, len(scores))
	for stage, score := range scores {
		items = append(items, item{stage: stage, score: score})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].score == items[j].score {
			return items[i].stage < items[j].stage
		}
		return items[i].score > items[j].score
	})

	ranked := make([]string, 0, len(items))
	for _, item := range items {
		ranked = append(ranked, item.stage)
	}
	return ranked
}

func stageLabels(stages []string) []string {
	labels := make([]string, 0, len(stages))
	for _, stage := range stages {
		labels = append(labels, stageLabel(stage))
	}
	return labels
}

func stageLabel(stage string) string {
	switch stage {
	case "stranger_contact":
		return "初识试探"
	case "warm_up":
		return "轻松升温"
	case "comfort_building":
		return "舒适建立"
	case "invite_window":
		return "邀约窗口"
	case "conflict_or_fadeout":
		return "冲突/冷淡"
	default:
		return "初识试探"
	}
}

func normalizeStageCandidates(candidates []string) []string {
	seen := map[string]bool{}
	normalized := make([]string, 0, len(canonicalStages))
	for _, candidate := range candidates {
		stage := strings.TrimSpace(strings.ToLower(candidate))
		switch stage {
		case "stranger_contact", "初识试探":
			stage = "stranger_contact"
		case "warm_up", "轻松升温":
			stage = "warm_up"
		case "comfort_building", "舒适建立":
			stage = "comfort_building"
		case "invite_window", "邀约窗口":
			stage = "invite_window"
		case "conflict_or_fadeout", "冲突/冷淡", "fadeout", "conflict":
			stage = "conflict_or_fadeout"
		default:
			continue
		}
		if !seen[stage] {
			seen[stage] = true
			normalized = append(normalized, stage)
		}
	}
	if len(normalized) == 0 {
		return []string{"stranger_contact", "warm_up"}
	}
	return normalized
}

func signalValue(signals map[string]float64, key string) float64 {
	if signals == nil {
		return 0
	}
	return clamp(signals[key], 0, 1)
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
