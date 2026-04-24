<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'

import {
  analyzeImage,
  analyzeText,
  deleteAnalysis,
  fetchAnalysis,
  fetchHistory,
  login,
  register,
  saveAnalysis,
  setAuthToken,
} from './api'
import type { AnalysisRecord, AnalysisSummary, AuthUser } from './types'

const authMode = ref<'login' | 'register'>('login')
const currentUser = ref<AuthUser | null>(null)
const username = ref('')
const password = ref('')
const inviteCode = ref('')
const mode = ref<'text' | 'image'>('text')
const textInput = ref(`我：今天忙完了吗？
对方：差不多，刚缓过来一点。
我：那你这两天像是在连轴转。
对方：是啊，感觉脑子都快烧了，周末只想找个安静点的地方坐坐。
我：那先别安排太满，找个轻松的地方喝点东西？
对方：这个倒是可以。`)
const selectedFile = ref<File | null>(null)
const loading = ref(false)
const authLoading = ref(false)
const errorMessage = ref('')
const authError = ref('')
const history = ref<AnalysisSummary[]>([])
const activeRecord = ref<AnalysisRecord | null>(null)

const hasResult = computed(() => activeRecord.value !== null)
const isSaved = computed(() => Boolean(activeRecord.value?.saved))
const canUseApp = computed(() => currentUser.value !== null)
const evidenceItems = computed(() => activeRecord.value?.result.semantic.evidence ?? [])

async function loadHistory() {
  if (!currentUser.value) {
    history.value = []
    return
  }
  try {
    const response = await fetchHistory()
    history.value = response.items
  } catch (error) {
    console.error(error)
  }
}

async function submit() {
  loading.value = true
  errorMessage.value = ''
  try {
    if (mode.value === 'text') {
      activeRecord.value = await analyzeText(textInput.value)
    } else if (selectedFile.value) {
      activeRecord.value = await analyzeImage(selectedFile.value)
    } else {
      throw new Error('请先选择聊天截图')
    }
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '分析失败，请稍后再试'
  } finally {
    loading.value = false
  }
}

async function saveCurrentAnalysis() {
  if (!activeRecord.value || activeRecord.value.saved) return
  loading.value = true
  errorMessage.value = ''
  try {
    activeRecord.value = await saveAnalysis(activeRecord.value)
    await loadHistory()
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '保存失败，请稍后再试'
  } finally {
    loading.value = false
  }
}

async function removeHistory(id: string) {
  loading.value = true
  errorMessage.value = ''
  try {
    await deleteAnalysis(id)
    if (activeRecord.value?.id === id) {
      activeRecord.value = null
    }
    await loadHistory()
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '删除失败，请稍后再试'
  } finally {
    loading.value = false
  }
}

async function submitAuth() {
  authLoading.value = true
  authError.value = ''
  try {
    const response =
      authMode.value === 'login'
        ? await login(username.value, password.value)
        : await register(username.value, password.value, inviteCode.value)
    currentUser.value = response.user
    setAuthToken(response.token)
    localStorage.setItem('senti_user', JSON.stringify(response.user))
    password.value = ''
    inviteCode.value = ''
    await loadHistory()
  } catch (error) {
    authError.value = error instanceof Error ? error.message : '登录失败'
  } finally {
    authLoading.value = false
  }
}

function logout() {
  currentUser.value = null
  activeRecord.value = null
  history.value = []
  setAuthToken('')
  localStorage.removeItem('senti_user')
}

async function openHistory(id: string) {
  loading.value = true
  errorMessage.value = ''
  try {
    activeRecord.value = await fetchAnalysis(id)
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '读取历史记录失败'
  } finally {
    loading.value = false
  }
}

function onFileChange(event: Event) {
  const target = event.target as HTMLInputElement
  selectedFile.value = target.files?.[0] ?? null
}

function formatDate(value: string) {
  return new Date(value).toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

onMounted(() => {
  setAuthToken(localStorage.getItem('senti_token') ?? '')
  const storedUser = localStorage.getItem('senti_user')
  if (storedUser) {
    currentUser.value = JSON.parse(storedUser) as AuthUser
    void loadHistory()
  }
})
</script>

<template>
  <div class="app-shell">
    <header class="hero">
      <div class="hero-copy">
        <p class="eyebrow">Senti</p>
        <h1>把聊天记录变成一份可操作的互动分析报告</h1>
        <p class="hero-text">
          基于心理学视角、聊天特征提取和 Kimi 生成能力，输出阶段判断、态度倾向、节奏建议和自然回复参考。
        </p>
      </div>
      <div class="hero-badge">
        <span v-if="currentUser">已登录：{{ currentUser.username }}</span>
        <button v-if="currentUser" class="ghost-button" @click="logout">退出</button>
      </div>
    </header>

    <section v-if="!canUseApp" class="panel auth-panel">
      <div class="section-head">
        <h2>{{ authMode === 'login' ? '登录' : '注册内测账号' }}</h2>
        <p>内测阶段需要账号和邀请码，避免公开访问造成模型调用成本失控。</p>
      </div>
      <div class="mode-switch">
        <button :class="{ active: authMode === 'login' }" @click="authMode = 'login'">登录</button>
        <button :class="{ active: authMode === 'register' }" @click="authMode = 'register'">注册</button>
      </div>
      <div class="field">
        <label for="username">邮箱或用户名</label>
        <input id="username" v-model="username" autocomplete="username" placeholder="name@example.com" />
      </div>
      <div class="field">
        <label for="password">密码</label>
        <input id="password" v-model="password" type="password" autocomplete="current-password" placeholder="至少 8 位" />
      </div>
      <div v-if="authMode === 'register'" class="field">
        <label for="invite-code">邀请码</label>
        <input id="invite-code" v-model="inviteCode" type="password" placeholder="请输入内测邀请码" />
      </div>
      <button class="primary-action" :disabled="authLoading" @click="submitAuth">
        {{ authLoading ? '处理中...' : authMode === 'login' ? '登录' : '注册并登录' }}
      </button>
      <p v-if="authError" class="error-banner">{{ authError }}</p>
    </section>

    <main v-else class="workspace">
      <section class="panel input-panel">
        <div class="section-head">
          <h2>输入内容</h2>
          <p>本次分析默认不保存。需要回看时，请在结果页点击保存。</p>
        </div>

        <div class="mode-switch">
          <button :class="{ active: mode === 'text' }" @click="mode = 'text'">文本分析</button>
          <button :class="{ active: mode === 'image' }" @click="mode = 'image'">截图分析</button>
        </div>

        <div v-if="mode === 'text'" class="field">
          <label for="chat-text">聊天记录</label>
          <textarea
            id="chat-text"
            v-model="textInput"
            rows="14"
            placeholder="建议用 我： / 对方： 标注，或按真实顺序逐行粘贴。"
          />
        </div>

        <div v-else class="field upload-field">
          <label for="chat-image">聊天长截图</label>
          <input id="chat-image" type="file" accept="image/*" @change="onFileChange" />
          <p class="helper">
            {{ selectedFile ? `已选择：${selectedFile.name}` : '建议上传完整聊天长图，避免截断关键时间和上下文。' }}
          </p>
        </div>

        <button class="primary-action" :disabled="loading" @click="submit">
          {{ loading ? '分析中...' : '开始分析' }}
        </button>

        <p v-if="errorMessage" class="error-banner">{{ errorMessage }}</p>
        <p class="disclaimer">
          仅供沟通参考，不构成心理诊断或关系结论。遇到威胁、自伤、骚扰等高风险情境，请优先寻求现实支持和专业帮助。
        </p>
      </section>

      <section class="panel result-panel">
        <div class="section-head">
          <h2>分析结果</h2>
          <p>结果由规则量化、指标计算和 AI 生成共同完成。</p>
        </div>

        <div v-if="!hasResult" class="empty-state">
          <p>提交一次聊天分析后，结果会显示在这里。</p>
        </div>

        <template v-else-if="activeRecord">
          <div class="summary-card">
            <div>
              <p class="card-label">当前阶段</p>
              <h3>{{ activeRecord.result.stage }}</h3>
            </div>
            <p>{{ activeRecord.result.summary }}</p>
          </div>

          <div class="metrics-grid">
            <article class="metric-card">
              <span>IVI</span>
              <strong>{{ activeRecord.result.metrics.ivi.score }}</strong>
              <p>{{ activeRecord.result.metrics.ivi.label }}</p>
            </article>
            <article class="metric-card">
              <span>SPE</span>
              <strong>{{ activeRecord.result.metrics.spe.score }}</strong>
              <p>{{ activeRecord.result.metrics.spe.label }}</p>
            </article>
            <article class="metric-card">
              <span>EWS</span>
              <strong>{{ activeRecord.result.metrics.ews.score }}</strong>
              <p>{{ activeRecord.result.metrics.ews.label }}</p>
            </article>
          </div>

          <div class="analysis-grid">
            <article class="info-card">
              <p class="card-label">态度倾向</p>
              <p>{{ activeRecord.result.attitude }}</p>
            </article>
            <article class="info-card">
              <p class="card-label">心理状态</p>
              <p>{{ activeRecord.result.psychology }}</p>
            </article>
          </div>

          <article class="info-card">
            <p class="card-label">关键证据</p>
            <div v-if="evidenceItems.length" class="evidence-list">
              <div
                v-for="item in evidenceItems"
                :key="`${item.type}-${item.quote}`"
                class="evidence-item"
              >
                <div class="evidence-meta">
                  <span>{{ item.speaker === 'user' ? '我' : '对方' }}</span>
                  <span>{{ item.score.toFixed(2) }}</span>
                </div>
                <p>{{ item.quote }}</p>
              </div>
            </div>
            <p v-else class="legacy-note">这次没有提取到足够明确的证据句。</p>
          </article>

          <article class="info-card">
            <p class="card-label">下一步建议</p>
            <ul>
              <li v-for="item in activeRecord.result.suggestions" :key="item">{{ item }}</li>
            </ul>
          </article>

          <article class="info-card">
            <p class="card-label">回复参考</p>
            <ul>
              <li v-for="item in activeRecord.result.replyOptions" :key="item">{{ item }}</li>
            </ul>
          </article>

          <article class="info-card">
            <p class="card-label">为什么是这个方向</p>
            <p>{{ activeRecord.result.rationale }}</p>
          </article>

          <article class="info-card risk-card">
            <p class="card-label">风险提醒</p>
            <p>{{ activeRecord.result.riskNote }}</p>
          </article>

          <button class="primary-action save-action" :disabled="loading || isSaved" @click="saveCurrentAnalysis">
            {{ isSaved ? '已保存' : '保存分析' }}
          </button>

          <article class="info-card conversation-card">
            <p class="card-label">聊天记录</p>
            <div class="conversation-list">
              <div
                v-for="(message, index) in activeRecord.structuredMessages"
                :key="`${message.speaker}-${index}`"
                class="conversation-item"
                :class="message.speaker"
              >
                <span class="speaker-tag">
                  {{ message.speaker === 'user' ? '我' : '对方' }}
                </span>
                <p>{{ message.content }}</p>
              </div>
            </div>
          </article>
        </template>
      </section>

      <aside class="panel history-panel">
        <div class="section-head">
          <h2>分析历史</h2>
          <p>这里只显示你主动保存的分析。</p>
        </div>

        <div v-if="history.length === 0" class="empty-history">还没有历史记录。</div>

        <div v-for="item in history" :key="item.id" class="history-item">
          <button class="history-open" @click="openHistory(item.id)">
            <span class="history-stage">{{ item.stage }}</span>
            <strong>{{ item.summary }}</strong>
            <small>{{ formatDate(item.createdAt) }}</small>
          </button>
          <button class="delete-button" @click="removeHistory(item.id)">删除</button>
        </div>
      </aside>
    </main>
  </div>
</template>
