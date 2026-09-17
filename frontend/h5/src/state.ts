// 全局共享状态 + 对局动作：所有视图组件只通过本模块通信，不在 view 里重复 fetch 逻辑。
import { computed, ref } from 'vue'
import {
  api, essenceColor, essenceName,
  type GuidanceKind, type PredictChoice, type StateView, type Vec2,
} from './api'

// ---------- 状态 ----------
export const view = ref<StateView | null>(null)
export const busy = ref(false)
export const error = ref('')

// Guidance
export const guideKind = ref<GuidanceKind>('whisper')
export const guideText = ref('')
export const guideError = ref('')

// 面板内 Tab（认知/事件/对话/预测）
export const tab = ref<'cognition' | 'events' | 'qa' | 'predict'>('cognition')

// Ask 对话
export const question = ref('')
export const askBusy = ref(false)
export const askError = ref('')

// Predict / 紧急暂停
export const predictBusy = ref(false)
export const predictError = ref('')

// M3.5 接入：敌人 ≤2 格或本回合有交战事件时为 true，届时外壳自动切到对战页。
// 当前阶段无战斗系统，恒为 false 的占位钩子。
export const inCombat = computed(() => false)

// ---------- 地图常量与渲染辅助 ----------
export const W = 12
export const H = 12
export const SPAWN: Vec2 = { x: 1, y: 10 }

export const exploredSet = computed(() => {
  const s = new Set<string>()
  for (const p of view.value?.explored ?? []) s.add(`${p.x},${p.y}`)
  return s
})

export const visibleSet = computed(() => {
  const s = new Set<string>()
  for (const p of view.value?.observation?.visible ?? []) s.add(`${p.x},${p.y}`)
  return s
})

export const entityAt = (x: number, y: number) =>
  view.value?.observation?.entities?.find(e => e.pos.x === x && e.pos.y === y)

export const selfAt = (x: number, y: number) =>
  view.value?.observation?.self.pos.x === x && view.value!.observation!.self.pos.y === y

// v0.1 地形渲染是客户端先验（旧地图）；正式版应由 Observation 供给。
export const terrainDanger = (x: number, y: number) => x >= 3 && x <= 7 && y >= 3 && y <= 7
export const terrainEssence = (x: number, y: number): number => {
  if (terrainDanger(x, y)) return 1 // 火
  if (y <= 1) return 3 // 金
  if (x === 0) return 4 // 水
  return 2 // 土
}

export const cellStyle = (x: number, y: number) => {
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
export const derivedCognition = computed(() => {
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

export const influenceText = computed(() =>
  (view.value?.influence ?? 0).toFixed(1))

export const costOf = (kind: string) => view.value?.costs?.[kind] ?? 0

export const GUIDANCE_OPTIONS: { value: GuidanceKind; label: string }[] = [
  { value: 'whisper', label: '轻语' },
  { value: 'guide', label: '引导' },
  { value: 'deeplink', label: '深谈' },
  { value: 'information', label: '情报' },
]

export const PREDICT_OPTIONS: { value: PredictChoice; label: string }[] = [
  { value: 'move', label: '移动' },
  { value: 'sense', label: '侦察' },
  { value: 'attack', label: '攻击' },
  { value: 'pickup', label: '拾取' },
  { value: 'assemble', label: '组装' },
  { value: 'wait', label: '待命' },
]

export const TABS: { key: 'cognition' | 'events' | 'qa' | 'predict'; label: string }[] = [
  { key: 'cognition', label: '认知' },
  { key: 'events', label: '事件' },
  { key: 'qa', label: '对话' },
  { key: 'predict', label: '预测' },
]

export const choiceLabel = (c?: string) =>
  PREDICT_OPTIONS.find(o => o.value === c)?.label ?? c ?? '—'

const errText = (e: unknown) => (e instanceof Error ? e.message : String(e))

// ---------- 对局动作 ----------
export async function refresh() {
  if (!view.value) return
  view.value = await api<StateView>(`/api/match/${view.value.id}/state`)
}

export async function newMatch() {
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

// 进入地图页时确保有对局；已在对局中则不重复创建（切页返回不丢局）。
export async function ensureMatch() {
  if (!view.value) await newMatch()
}

export async function step(auto: boolean) {
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

export async function runToEnd() {
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

export async function sendGuidance() {
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

export async function ask() {
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

export async function predict(choice: PredictChoice) {
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

export async function pause() {
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
