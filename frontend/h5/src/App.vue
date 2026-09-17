<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  api, essenceColor, essenceName,
  type GuidanceKind, type PredictChoice, type StateView, type Vec2,
} from './api'

const view = ref<StateView | null>(null)
const busy = ref(false)
const error = ref('')
const guideKind = ref<GuidanceKind>('whisper')
const guideText = ref('')
const guideError = ref('')
const tab = ref<'cognition' | 'events' | 'qa' | 'predict'>('cognition')
const question = ref('')
const askBusy = ref(false)
const askError = ref('')
const predictBusy = ref(false)
const predictError = ref('')

const W = 12
const H = 12
const SPAWN: Vec2 = { x: 1, y: 10 }

const exploredSet = computed(() => {
  const s = new Set<string>()
  for (const p of view.value?.explored ?? []) s.add(`${p.x},${p.y}`)
  return s
})

const visibleSet = computed(() => {
  const s = new Set<string>()
  for (const p of view.value?.observation?.visible ?? []) s.add(`${p.x},${p.y}`)
  return s
})

const entityAt = (x: number, y: number) =>
  view.value?.observation?.entities?.find(e => e.pos.x === x && e.pos.y === y)

const selfAt = (x: number, y: number) =>
  view.value?.observation?.self.pos.x === x && view.value!.observation!.self.pos.y === y

// v0.1 地形渲染是客户端先验（旧地图）；正式版应由 Observation 供给。
const terrainDanger = (x: number, y: number) => x >= 3 && x <= 7 && y >= 3 && y <= 7
const terrainEssence = (x: number, y: number): number => {
  if (terrainDanger(x, y)) return 1 // 火
  if (y <= 1) return 3 // 金
  if (x === 0) return 4 // 水
  return 2 // 土
}

const cellStyle = (x: number, y: number) => {
  const key = `${x},${y}`
  const explored = exploredSet.value.has(key)
  const visible = visibleSet.value.has(key)
  const base = explored ? essenceColor(terrainEssence(x, y)) : '#222'
  return {
    background: base,
    opacity: explored ? (visible ? 1 : 0.45) : 1,
    outline: terrainDanger(x, y) ? '1px solid rgba(255,80,60,.4)' : 'none',
    outlineOffset: '-2px',
  }
}

// 无 LLM 认知摘要时的客户端派生回退
const derivedCognition = computed(() => {
  const v = view.value
  if (!v?.observation) return []
  const s = v.observation.self
  const seen = v.observation.entities ?? []
  const rows: [string, string][] = [
    ['Goal', (s.carrying ?? []).length ? '携带 Fragment 返回出生点' : '前往危险区取得 Fragment'],
    ['Plan', '直线推进，功率 0.4（阴偏稳）'],
    ['Known', seen.length
      ? seen.map(e => `${e.kind === 'fragment' ? '结晶' : e.owner === 'unknown' ? '未知体' : e.kind}·${essenceName(e.essence)} @${e.pos.x},${e.pos.y}`).join('；')
      : '感知范围内无其他实体'],
    ['Risk', `热 ${s.heat}/25 · 气 ${s.battery}/${s.max_battery}`],
    ['Suspicions', seen.some(e => e.owner === 'unknown') ? 'Unknown Agent 在附近（火迹明显，虚张声势？）' : '无'],
  ]
  return rows
})

const influenceText = computed(() =>
  (view.value?.influence ?? 0).toFixed(1))

const costOf = (kind: string) => view.value?.costs?.[kind] ?? 0

const GUIDANCE_OPTIONS: { value: GuidanceKind; label: string }[] = [
  { value: 'whisper', label: '轻语' },
  { value: 'guide', label: '引导' },
  { value: 'deeplink', label: '深谈' },
  { value: 'information', label: '情报' },
]

const PREDICT_OPTIONS: { value: PredictChoice; label: string }[] = [
  { value: 'move', label: '移动' },
  { value: 'sense', label: '侦察' },
  { value: 'attack', label: '攻击' },
  { value: 'pickup', label: '拾取' },
  { value: 'assemble', label: '组装' },
  { value: 'wait', label: '待命' },
]

const TABS: { key: 'cognition' | 'events' | 'qa' | 'predict'; label: string }[] = [
  { key: 'cognition', label: '认知' },
  { key: 'events', label: '事件' },
  { key: 'qa', label: '对话' },
  { key: 'predict', label: '预测' },
]

const choiceLabel = (c?: string) =>
  PREDICT_OPTIONS.find(o => o.value === c)?.label ?? c ?? '—'

const errText = (e: unknown) => (e instanceof Error ? e.message : String(e))

async function refresh() {
  if (!view.value) return
  view.value = await api<StateView>(`/api/match/${view.value.id}/state`)
}

async function newMatch() {
  busy.value = true
  error.value = ''
  try {
    view.value = await api<StateView>('/api/match', {})
  } catch (e) {
    error.value = errText(e)
  } finally {
    busy.value = false
  }
}

async function step(auto: boolean) {
  if (!view.value) return
  busy.value = true
  error.value = ''
  try {
    view.value = await api<StateView>(
      `/api/match/${view.value.id}/${auto ? 'auto' : 'tick'}`, auto ? {} : { intents: [] })
  } catch (e) {
    error.value = errText(e)
  } finally {
    busy.value = false
  }
}

async function runToEnd() {
  if (!view.value) return
  busy.value = true
  error.value = ''
  try {
    for (let i = 0; i < 60; i++) {
      view.value = await api<StateView>(`/api/match/${view.value.id}/auto`, {})
      if (view.value.status !== 'running') break
    }
  } catch (e) {
    error.value = errText(e)
  } finally {
    busy.value = false
  }
}

async function sendGuidance() {
  if (!view.value || !guideText.value.trim() || busy.value) return
  busy.value = true
  error.value = ''
  guideError.value = ''
  try {
    view.value = await api<StateView>(`/api/match/${view.value.id}/guidance`, {
      kind: guideKind.value,
      text: guideText.value.trim(),
    })
    guideText.value = ''
  } catch (e) {
    guideError.value = errText(e)
  } finally {
    busy.value = false
  }
}

async function ask() {
  if (!view.value || !question.value.trim() || askBusy.value) return
  askBusy.value = true
  askError.value = ''
  try {
    await api<{ answer: string }>(`/api/match/${view.value.id}/ask`, {
      question: question.value.trim(),
    })
    question.value = ''
    view.value = await api<StateView>(`/api/match/${view.value.id}/state`)
  } catch (e) {
    askError.value = errText(e)
  } finally {
    askBusy.value = false
  }
}

async function predict(choice: PredictChoice) {
  if (!view.value || predictBusy.value || view.value.status !== 'running') return
  predictBusy.value = true
  predictError.value = ''
  try {
    await api<{ ok: boolean }>(`/api/match/${view.value.id}/predict`, { choice })
    view.value = await api<StateView>(`/api/match/${view.value.id}/state`)
  } catch (e) {
    predictError.value = errText(e)
  } finally {
    predictBusy.value = false
  }
}

async function pause() {
  if (!view.value || predictBusy.value || view.value.status !== 'running') return
  predictBusy.value = true
  predictError.value = ''
  try {
    await api<{ ok: boolean }>(`/api/match/${view.value.id}/pause`, null)
    view.value = await api<StateView>(`/api/match/${view.value.id}/state`)
  } catch (e) {
    predictError.value = errText(e)
  } finally {
    predictBusy.value = false
  }
}

onMounted(newMatch)
</script>

<template>
  <div class="wrap">
    <header>
      <h1>agent_battle <small>scenario/mvp-0.1</small></h1>
      <div class="bar" v-if="view">
        <span>Turn {{ view.turn }}/{{ view.turn_limit }}</span>
        <span class="influence">⚡ {{ influenceText }}</span>
        <span v-if="view.pilot" class="badge" :class="'pilot-' + view.pilot">{{ view.pilot }}</span>
        <span :class="{ warn: (view.observation?.self.heat ?? 0) > 18 }">
          热 {{ view.observation?.self.heat ?? '—' }}
        </span>
        <span>形 {{ view.observation?.self.integrity ?? '—' }}</span>
        <span>气 {{ view.observation?.self.battery ?? '—' }}</span>
        <span>状态 {{ view.status }}</span>
        <span class="seed">seed {{ view.seed }}</span>
      </div>
    </header>

    <div class="main">
      <div class="map-wrap">
        <div class="grid">
          <div v-for="y in H" :key="y" class="row">
            <div
              v-for="x in W" :key="x"
              class="cell"
              :style="cellStyle(x - 1, y - 1)"
            >
              <span v-if="selfAt(x - 1, y - 1)">🤖</span>
              <span v-else-if="entityAt(x - 1, y - 1)?.kind === 'fragment'">💎</span>
              <span v-else-if="entityAt(x - 1, y - 1)">❓</span>
              <span v-else-if="x - 1 === SPAWN.x && y - 1 === SPAWN.y" class="spawn">⌂</span>
            </div>
          </div>
        </div>
        <div class="btns">
          <button @click="newMatch" :disabled="busy">新对局</button>
          <button @click="step(true)" :disabled="busy || view?.status !== 'running'">自动一步</button>
          <button @click="runToEnd" :disabled="busy || view?.status !== 'running'">自动到结束</button>
        </div>
        <p v-if="error" class="err">{{ error }}</p>
      </div>

      <aside class="panel">
        <nav class="tabs">
          <button
            v-for="t in TABS" :key="t.key"
            :class="{ active: tab === t.key }"
            @click="tab = t.key"
          >{{ t.label }}</button>
        </nav>

        <!-- 认知 -->
        <section v-show="tab === 'cognition'">
          <div v-if="view && view.status !== 'running'" class="review-card">
            <h2>赛后复盘</h2>
            <template v-if="view.review">
              <p class="review-summary">{{ view.review.summary }}</p>
              <ul class="lessons">
                <li v-for="(l, i) in view.review.lessons" :key="i">教训：{{ l }}</li>
              </ul>
            </template>
            <p v-else class="final">对局已结束：{{ view.status }}</p>
          </div>

          <h2>Agent 认知摘要 <small>{{ view?.cognition ? 'LLM 自报' : 'v0.1 派生' }}</small></h2>
          <dl v-if="view?.cognition">
            <dt>目标</dt><dd>{{ view.cognition.goal }}</dd>
            <dt>计划</dt><dd>{{ view.cognition.plan }}</dd>
            <dt>已知</dt><dd>{{ view.cognition.known }}</dd>
            <dt>疑点</dt><dd>{{ view.cognition.suspicions }}</dd>
            <dt>风险</dt><dd>{{ view.cognition.risk }}</dd>
            <dt>置信</dt><dd>{{ view.cognition.confidence.toFixed(2) }}</dd>
          </dl>
          <dl v-else-if="derivedCognition.length">
            <template v-for="[k, v] in derivedCognition" :key="k">
              <dt>{{ k }}</dt>
              <dd>{{ v }}</dd>
            </template>
          </dl>
          <p v-else>—</p>
        </section>

        <!-- 事件 -->
        <section v-show="tab === 'events'">
          <h2>最近事件</h2>
          <ul class="events" v-if="(view?.events ?? []).length">
            <li v-for="(e, i) in (view?.events ?? []).slice(-8).reverse()" :key="i">
              <em>T{{ e.tick }}</em> {{ e.text }}
            </li>
          </ul>
          <p v-else>—</p>
        </section>

        <!-- 对话 -->
        <section v-show="tab === 'qa'">
          <h2>Ask <small>免费提问，Agent 第一人称回答</small></h2>
          <div class="guide">
            <input v-model="question" placeholder="例如：你现在最怕什么？" @keyup.enter="ask" />
            <button @click="ask" :disabled="askBusy || !view">提问</button>
          </div>
          <p v-if="askError" class="sub-err">{{ askError }}</p>
          <ul class="qa-list" v-if="(view?.qa ?? []).length">
            <li v-for="(q, i) in (view?.qa ?? []).slice().reverse()" :key="'q' + i">
              <p class="q">Q：{{ q.question }}</p>
              <p class="a">A：{{ q.answer }}</p>
            </li>
          </ul>
          <p v-else>—</p>
        </section>

        <!-- 预测 -->
        <section v-show="tab === 'predict'">
          <h2>Predict <small>猜中 Agent 下一步 +0.5</small></h2>
          <div class="predict-btns">
            <button
              v-for="o in PREDICT_OPTIONS" :key="o.value"
              @click="predict(o.value)"
              :disabled="predictBusy || !view || view.status !== 'running'"
            >{{ o.label }}</button>
          </div>
          <p v-if="predictError" class="sub-err">{{ predictError }}</p>
          <p v-if="view?.pending" class="pending">
            已预测：{{ choiceLabel(view.pending.choice) }}，等下一回合结算
          </p>
          <div class="pause-row">
            <button
              class="danger"
              @click="pause"
              :disabled="predictBusy || !view || view.status !== 'running'"
            >紧急暂停 (⚡{{ (costOf('pause') || 3).toFixed(1) }})</button>
            <small>每局一次，消耗影响点</small>
          </div>
          <h2>预测历史</h2>
          <ul class="events" v-if="(view?.predictions ?? []).length">
            <li v-for="(p, i) in (view?.predictions ?? []).slice().reverse()" :key="'p' + i">
              <em>T{{ p.turn }}</em>
              {{ choiceLabel(p.choice) }}
              <template v-if="p.resolved">
                → 实际 {{ choiceLabel(p.actual) }} ·
                <b :class="p.hit ? 'hit' : 'miss'">{{ p.hit ? '命中 ✓' : '未中 ✗' }}</b>
              </template>
              <template v-else> · 待结算</template>
            </li>
          </ul>
          <p v-else>—</p>
        </section>

        <h2>Guidance <small>给偏好，不给动作</small></h2>
        <div class="guide">
          <select v-model="guideKind">
            <option v-for="o in GUIDANCE_OPTIONS" :key="o.value" :value="o.value">
              {{ o.label }} (⚡{{ costOf(o.value).toFixed(1) }})
            </option>
          </select>
          <input v-model="guideText" placeholder="例如：先确认它的储气再决定是否接战" @keyup.enter="sendGuidance" />
          <button @click="sendGuidance" :disabled="busy || !view">发送</button>
        </div>
        <p v-if="guideError" class="sub-err">{{ guideError }}</p>
        <ul class="events">
          <li v-for="(g, i) in (view?.guidance ?? []).slice().reverse()" :key="'g' + i">
            <em>T{{ g.turn }} {{ g.kind }}</em> {{ g.text }}
          </li>
        </ul>
      </aside>
    </div>
  </div>
</template>

<style>
* { box-sizing: border-box; }
body {
  margin: 0;
  background: #14161a;
  color: #d8dce2;
  font-family: system-ui, -apple-system, 'PingFang SC', sans-serif;
}
.wrap { max-width: 1020px; margin: 0 auto; padding: 16px; }
header h1 { font-size: 18px; margin: 0 0 8px; }
header small { color: #7a8290; font-weight: normal; }
.bar { display: flex; gap: 14px; font-size: 13px; color: #aab2bd; flex-wrap: wrap; align-items: center; }
.bar .warn { color: #ff6b57; }
.bar .seed { margin-left: auto; color: #59616d; }
.bar .influence { color: #ffd86b; font-weight: 600; }
.badge {
  font-size: 11px; padding: 1px 7px; border-radius: 8px;
  border: 1px solid #3a4150; color: #9fd0ff; background: #1d2129;
}
.badge.pilot-scripted { color: #aab2bd; }
.main { display: flex; gap: 20px; margin-top: 14px; flex-wrap: wrap; }
.map-wrap { flex: 0 0 auto; }
.grid { display: inline-block; border: 1px solid #2a2f37; }
.row { display: flex; }
.cell {
  width: 38px; height: 38px;
  display: flex; align-items: center; justify-content: center;
  font-size: 18px; position: relative;
}
.cell .spawn { color: #8fd; font-size: 12px; }
.btns { display: flex; gap: 8px; margin-top: 10px; flex-wrap: wrap; }
button {
  background: #2a3040; color: #d8dce2; border: 1px solid #3a4150;
  border-radius: 6px; padding: 6px 12px; cursor: pointer; font-size: 13px;
}
button:hover:not(:disabled) { background: #35405a; }
button:disabled { opacity: 0.4; cursor: default; }
button.danger { border-color: #5a3a3a; color: #ff9d8f; }
button.danger:hover:not(:disabled) { background: #45303a; }
.panel { flex: 1 1 320px; min-width: 300px; }
.tabs { display: flex; gap: 4px; border-bottom: 1px solid #2a2f37; margin-bottom: 4px; }
.tabs button {
  background: transparent; border: none; border-bottom: 2px solid transparent;
  border-radius: 0; color: #7a8290; padding: 6px 10px;
}
.tabs button:hover:not(:disabled) { background: transparent; color: #d8dce2; }
.tabs button.active { color: #fff; border-bottom-color: #9fd0ff; }
.panel h2 { font-size: 14px; margin: 18px 0 6px; color: #9fd0ff; }
.panel h2 small { color: #59616d; font-weight: normal; }
dl { margin: 0; font-size: 13px; }
dt { color: #7a8290; margin-top: 8px; }
dd { margin: 2px 0 0; line-height: 1.5; }
.events { list-style: none; margin: 0; padding: 0; font-size: 12.5px; color: #aab2bd; }
.events li { padding: 3px 0; border-bottom: 1px dashed #242933; }
.events em { color: #59616d; font-style: normal; margin-right: 6px; }
.events b.hit { color: #6fd48a; font-weight: 600; }
.events b.miss { color: #ff6b57; font-weight: 600; }
.guide { display: flex; gap: 6px; }
.guide select, .guide input {
  background: #1d2129; color: #d8dce2; border: 1px solid #3a4150;
  border-radius: 6px; padding: 6px 8px; font-size: 13px;
}
.guide input { flex: 1; }
.err { color: #ff6b57; font-size: 13px; }
.sub-err { color: #ff6b57; font-size: 12px; margin: 4px 0 0; }
.review-card {
  margin-top: 14px; padding: 10px 12px;
  border: 1px solid #3a4150; border-radius: 8px; background: #1d2129;
}
.review-card h2 { margin-top: 0; }
.review-summary { font-size: 13px; line-height: 1.6; margin: 4px 0; }
.lessons { list-style: none; margin: 6px 0 0; padding: 0; font-size: 12.5px; color: #aab2bd; }
.lessons li { padding: 3px 0; border-bottom: 1px dashed #242933; }
.lessons li:last-child { border-bottom: none; }
.final { color: #7a8290; font-size: 13px; margin: 4px 0; }
.predict-btns { display: flex; gap: 6px; flex-wrap: wrap; }
.pending { color: #ffd86b; font-size: 12.5px; margin: 8px 0 0; }
.pause-row { display: flex; align-items: center; gap: 8px; margin-top: 12px; }
.pause-row small { color: #59616d; }
.qa-list { list-style: none; margin: 10px 0 0; padding: 0; font-size: 12.5px; }
.qa-list li { padding: 6px 0; border-bottom: 1px dashed #242933; }
.qa-list p { margin: 2px 0; line-height: 1.55; }
.qa-list .q { color: #9fd0ff; }
.qa-list .a { color: #d8dce2; }
</style>
