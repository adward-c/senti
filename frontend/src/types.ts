export interface MetricValue {
  name: string
  raw: number
  score: number
  label: string
  explanation: string
}

export interface AnalysisMetrics {
  ivi: MetricValue
  spe: MetricValue
  ews: MetricValue
  params?: Record<string, number>
  signals?: Record<string, number>
}

export interface FactFeatures {
  userTurns: number
  targetTurns: number
  userChars: number
  targetChars: number
  userQuestions: number
  targetQuestions: number
  emojiCount: number
  positiveSignals: number
  negativeSignals: number
  inviteSignals: number
  conflictSignals: number
  disclosureSignals: number
  complianceSignals: number
  boundarySignals: number
  deflectionSignals: number
  humorSignals: number
  warmthSignals: number
  userLatencyMin: number
  targetLatencyMin: number
}

export interface SemanticEvidence {
  type: string
  quote: string
  speaker: string
  score: number
}

export interface SemanticLabels {
  stageCandidates: string[]
  topicType: string
  signals: Record<string, number>
  evidence: SemanticEvidence[]
}

export interface StrategyDecision {
  type: string
  label: string
  reason: string
  riskBlock: boolean
}

export interface ParamTrace {
  value: number
  basis: number
  adjustments: string[]
}

export interface AnalysisDebug {
  factFeatures: FactFeatures
  semanticLabels: SemanticLabels
  stageCandidates: string[]
  stageReason: string
  paramTraces: Record<string, ParamTrace>
  metricInputs: Record<string, number>
  strategy: StrategyDecision
}

export interface Message {
  speaker: string
  content: string
}

export interface AnalysisResultPayload {
  stage: string
  summary: string
  attitude: string
  psychology: string
  suggestions: string[]
  replyOptions: string[]
  rationale: string
  riskNote: string
  disclaimer: string
  conversation: Message[]
  metrics: AnalysisMetrics
  semantic: SemanticLabels
  strategy: StrategyDecision
  debug: AnalysisDebug
  rawOcrText?: string
}

export interface AnalysisRecord {
  id: string
  inputType: string
  sourceText: string
  imagePath?: string
  structuredMessages: Message[]
  result: AnalysisResultPayload
  createdAt: string
}

export interface AnalysisSummary {
  id: string
  inputType: string
  stage: string
  summary: string
  createdAt: string
}
