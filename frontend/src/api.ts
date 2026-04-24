import type { AnalysisRecord, AnalysisSummary, AuthResponse } from './types'

const API_BASE = import.meta.env.VITE_API_BASE_URL ?? '/api'

let authToken = localStorage.getItem('senti_token') ?? ''

export function setAuthToken(token: string) {
  authToken = token
  if (token) {
    localStorage.setItem('senti_token', token)
  } else {
    localStorage.removeItem('senti_token')
  }
}

async function request<T>(input: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers)
  if (authToken) {
    headers.set('Authorization', `Bearer ${authToken}`)
  }
  const response = await fetch(`${API_BASE}${input}`, { ...init, headers })
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

export function deleteAnalysis(id: string) {
  return request<{ ok: boolean }>(`/history/${id}`, { method: 'DELETE' })
}

export function saveAnalysis(record: AnalysisRecord) {
  return request<AnalysisRecord>('/analyses/save', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(record),
  })
}

export function register(username: string, password: string, inviteCode: string) {
  return request<AuthResponse>('/auth/register', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ username, password, inviteCode }),
  })
}

export function login(username: string, password: string) {
  return request<AuthResponse>('/auth/login', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ username, password }),
  })
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
