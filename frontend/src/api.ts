import type { AnalysisRecord, AnalysisSummary } from './types'

const API_BASE = import.meta.env.VITE_API_BASE_URL ?? '/api'

async function request<T>(input: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE}${input}`, init)
  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as { error?: string }
    throw new Error(body.error ?? '请求失败')
  }
  return response.json() as Promise<T>
}

export function fetchHistory() {
  return request<{ items: AnalysisSummary[] }>('/history')
}

export function fetchAnalysis(id: string) {
  return request<AnalysisRecord>(`/history/${id}`)
}

export function analyzeText(text: string) {
  return request<AnalysisRecord>('/analyze/text', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ text }),
  })
}

export function analyzeImage(file: File) {
  const formData = new FormData()
  formData.append('image', file)
  return request<AnalysisRecord>('/analyze/image', {
    method: 'POST',
    body: formData,
  })
}

