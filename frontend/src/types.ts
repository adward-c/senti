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

