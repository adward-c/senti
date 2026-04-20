<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'

import { analyzeImage, analyzeText, fetchAnalysis, fetchHistory } from './api'
import type { AnalysisRecord, AnalysisSummary } from './types'

const mode = ref<'text' | 'image'>('text')
const textInput = ref(`我：今天忙完了吗？
对方：差不多，刚缓过来一点。
我：那你这两天像是在连轴转。
对方：是啊，感觉脑子都快烧了，周末只想找个安静点的地方坐坐。
我：那先别安排太满，找个轻松的地方喝点东西？
对方：这个倒是可以。`)
const selectedFile = ref<File | null>(null)
const loading = ref(false)
const errorMessage = ref('')
const history = ref<AnalysisSummary[]>([])
const activeRecord = ref<AnalysisRecord | null>(null)

const hasResult = computed(() => activeRecord.value !== null)

async function loadHistory() {
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
    await loadHistory()
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '分析失败，请稍后再试'
  } finally {
    loading.value = false
  }
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
  void loadHistory()
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
        <span>Vue 3</span>
        <span>Go API</span>
        <span>Kimi</span>
        <span>Postgres</span>
      </div>
    </header>

    <main class="workspace">
      <section class="panel input-panel">
        <div class="section-head">
          <h2>输入内容</h2>
          <p>支持文本粘贴或长截图上传，长图会先走后端 OCR 再进入分析引擎。</p>
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
        </template>
      </section>

      <aside class="panel history-panel">
        <div class="section-head">
          <h2>分析历史</h2>
          <p>最近的分析会保存在数据库里，方便回看。</p>
        </div>

        <div v-if="history.length === 0" class="empty-history">还没有历史记录。</div>

        <button
          v-for="item in history"
          :key="item.id"
          class="history-item"
          @click="openHistory(item.id)"
        >
          <span class="history-stage">{{ item.stage }}</span>
          <strong>{{ item.summary }}</strong>
          <small>{{ formatDate(item.createdAt) }}</small>
        </button>
      </aside>
    </main>
  </div>
</template>
