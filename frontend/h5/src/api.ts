// 与 backend/match/api.go 的 JSON 契约保持一致。

export interface Vec2 {
  x: number
  y: number
}

export interface Event {
  tick: number
  type: string
  text: string
  entity?: number
}

export interface SelfView {
  pos: Vec2
  essence: number
  integrity: number
  max_integrity: number
  heat: number
  battery: number
  max_battery: number
  carrying: number[]
  has_motor: boolean
  sensor_active: boolean
  has_emitter: boolean
}

export interface SeenEntity {
  id: number
  pos: Vec2
  distance: number
  kind: string
  owner: string
  essence: number
  integrity: number
  heat: number
}

export interface Observation {
  tick: number
  self: SelfView
  visible: Vec2[]
  entities: SeenEntity[]
  events: Event[]
}

export type GuidanceKind = 'whisper' | 'guide' | 'deeplink' | 'information'

export interface Guidance {
  turn: number
  kind: string
  text: string
}

// LLM 每回合自我报告的显式认知摘要
export interface Cognition {
  goal: string
  plan: string
  known: string
  suspicions: string
  risk: string
  confidence: number
}

export interface Prediction {
  turn: number
  choice: string
  resolved: boolean
  actual?: string
  hit: boolean
}

export interface QAEntry {
  turn: number
  question: string
  answer: string
}

export interface Review {
  summary: string
  lessons: string[]
}

export type PredictChoice = 'move' | 'sense' | 'attack' | 'pickup' | 'assemble' | 'wait'

export interface StateView {
  id: string
  seed: number
  pack: string
  pilot: string
  status: 'running' | 'won' | 'lost' | 'timeout'
  turn: number
  turn_limit: number
  observation: Observation | null
  explored: Vec2[]
  events: Event[]
  guidance: Guidance[]
  influence: number
  costs: Record<string, number>
  cognition: Cognition | null
  predictions: Prediction[]
  pending: Prediction | null
  qa: QAEntry[]
  review: Review | null
}

const ESSENCE_NAMES = ['木', '火', '土', '金', '水'] as const

export const essenceName = (e: number): string => ESSENCE_NAMES[e] ?? '?'

// 五行语义色，与 design.ts / App.vue 的 --wood/--fire/--earth/--metal/--water 一致
const ESSENCE_COLORS = ['#57b26a', '#e05545', '#c9a44a', '#b8c0cc', '#4a8fd4'] as const

export const essenceColor = (e: number): string => ESSENCE_COLORS[e] ?? '#888'

// body 传 undefined 表示 GET；传 null 表示无 body 的 POST（如 pause）。
export async function api<T>(path: string, body?: unknown): Promise<T> {
  const hasBody = body !== undefined
  const res = await fetch(path, {
    method: hasBody ? 'POST' : 'GET',
    headers: hasBody && body !== null ? { 'Content-Type': 'application/json' } : undefined,
    body: hasBody && body !== null ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error((err as { error?: string }).error ?? res.statusText)
  }
  return res.json() as Promise<T>
}
