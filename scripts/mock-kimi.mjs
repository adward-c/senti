import http from 'node:http'

const port = Number(process.env.PORT ?? 18081)
const model = process.env.KIMI_MODEL ?? 'moonshot-v1-8k'

function json(response, status, payload) {
  response.writeHead(status, {
    'content-type': 'application/json',
  })
  response.end(JSON.stringify(payload))
}

function readBody(request) {
  return new Promise((resolve, reject) => {
    let body = ''
    request.setEncoding('utf8')
    request.on('data', (chunk) => {
      body += chunk
    })
    request.on('end', () => resolve(body))
    request.on('error', reject)
  })
}

function completion(content) {
  return {
    id: `mock-${Date.now()}`,
    object: 'chat.completion',
    choices: [
      {
        index: 0,
        message: {
          role: 'assistant',
          content: JSON.stringify(content),
        },
        finish_reason: 'stop',
      },
    ],
  }
}

function semanticLabels(prompt) {
  const conflict = /不想解释|先别聊|难受|别联系|自杀|伤害自己/.test(prompt)
  if (conflict) {
    return {
      stage_candidates: ['conflict_or_fadeout', 'comfort_building'],
      topic_type: '冲突降温',
      signals: {
        window_signal: 0.1,
        defensiveness: 0.86,
        backstage_exposure: 0.25,
        compliance_signal: 0.05,
        emotional_valence: 0.2,
        conflict_risk: 0.85,
        receptiveness: 0.18,
      },
      evidence: [
        { type: 'defensiveness', quote: '我现在真的很累，不想解释。', speaker: 'target', score: 0.86 },
        { type: 'conflict_risk', quote: '那就先别聊了吧。', speaker: 'target', score: 0.8 },
      ],
    }
  }

  return {
    stage_candidates: ['invite_window', 'comfort_building', 'warm_up'],
    topic_type: '轻量邀约',
    signals: {
      window_signal: 0.78,
      defensiveness: 0.12,
      backstage_exposure: 0.7,
      compliance_signal: 0.72,
      emotional_valence: 0.74,
      conflict_risk: 0.08,
      receptiveness: 0.8,
    },
    evidence: [
      { type: 'window_signal', quote: '这个倒是可以。', speaker: 'target', score: 0.82 },
      { type: 'backstage_exposure', quote: '周末只想找个安静点的地方坐坐。', speaker: 'target', score: 0.68 },
    ],
  }
}

function narrative(prompt) {
  const conflict = /冲突\/冷淡|risk_block|不想解释|先别聊/.test(prompt)
  if (conflict) {
    return {
      summary: '当前对话处在明显降温阶段，对方更需要空间而不是继续解释。',
      attitude: '对方偏防御和疲惫，短时间内不适合继续追问。',
      psychology: '对方可能在回避进一步冲突，需要先恢复安全感。',
      suggestions: ['先停止追问', '用短句承接对方状态', '等情绪回稳后再讨论'],
      reply_options: ['好，我先不继续追问，你先休息。', '我收到你的状态了，今天先到这里。', '不急着解释，等你舒服一点再说。'],
      rationale: '边界和疲惫信号较强，继续推进容易加重对方防御。',
      risk_note: '避免连发解释或追问；若出现自伤、威胁等高风险内容，应优先寻求现实支持。',
    }
  }

  return {
    summary: '当前处于邀约窗口，对方对轻松安排有开放态度。',
    attitude: '对方愿意接话，也接受低压力的线下提议。',
    psychology: '对方更像是疲惫后想找轻松空间放松。',
    suggestions: ['把邀约收敛成一个轻量选项', '给对方保留选择空间', '继续保持自然节奏'],
    reply_options: ['那我们找个安静点的地方坐坐，不赶时间。', '可以，周末挑个轻松的地方喝点东西。', '那就不安排太满，找个舒服的位置放松一下。'],
    rationale: '对方有真实状态暴露和配合信号，适合轻推进而不是加压。',
    risk_note: '窗口存在但仍需低压力表达，避免一次性抛出过多安排。',
  }
}

const server = http.createServer(async (request, response) => {
  if (request.method === 'GET' && request.url === '/v1/models') {
    json(response, 200, { data: [{ id: model }, { id: 'kimi-k2.6' }, { id: 'moonshot-v1-8k' }] })
    return
  }

  if (request.method === 'POST' && request.url === '/v1/chat/completions') {
    const raw = await readBody(request)
    const payload = JSON.parse(raw)
    const userPrompt = payload.messages?.find((message) => message.role === 'user')?.content ?? ''
    const content = userPrompt.includes('"stage_candidates"') ? semanticLabels(userPrompt) : narrative(userPrompt)
    json(response, 200, completion(content))
    return
  }

  json(response, 404, { error: 'not found' })
})

server.listen(port, '0.0.0.0', () => {
  console.log(`mock kimi listening on ${port}`)
})
