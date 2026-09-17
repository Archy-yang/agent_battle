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
  // ---- M3.4 追加字段（backend/match/api.go StateView 已实现，旧对局下为 undefined） ----
  // 本回合是否处于遭遇战（外壳据此自动切到对战页）
  in_combat?: boolean
  // 感知半径内的敌对实体（EnemyView.essence 是五行中文名字符串，与 SeenEntity 的 number 不同）
  enemies?: EnemyView[]
  // 当前移动订单目标格（move_to），无订单为 null
  order?: Vec2 | null
}

// M3.4：感知范围内的一名敌对实体（backend EnemyView）。
export interface EnemyView {
  id: number
  pos: Vec2
  distance: number
  essence: string // 五行中文名：木/火/土/金/水
  integrity: number
  heat: number
}

// v0.4 计划：explored 元素从纯坐标扩展为带地形记忆的格子
// （essence/danger/passable 由服务端下发）；在服务端实现前，
// 前端把"已探索未感知"渲染为中性暗土色，不自造地形。
export type ExploredCell = Vec2 & {
  essence?: number
  danger?: boolean
  passable?: boolean
}

// EnemyView.essence（中文名）→ 五行语义色，与 essenceColor 对齐
const ESSENCE_COLORS_BY_NAME: Record<string, string> = {
  木: '#57b26a',
  火: '#e05545',
  土: '#c9a44a',
  金: '#b8c0cc',
  水: '#4a8fd4',
}

export const essenceColorByName = (name: string): string =>
  ESSENCE_COLORS_BY_NAME[name] ?? '#b8c0cc'

// ---- M3.6 追加类型：组件 / 物料 / 移动订单 / 养成档案 ----

// 引擎 ComponentView：本体的一个组件（type/essence 为中文名字符串）
export interface ComponentView {
  id: number
  type: string // 躯壳/储气/灵机/感识/识海/离火/仓廪/清源
  essence: string // 木/火/土/金/水
  active: boolean
}

// SelfView 的 M3.6 扩展（引擎已带 components 字段）
export interface SelfExt extends SelfView {
  components?: ComponentView[]
}

// StateView 的 M3.6 扩展（materials: 本源中文名 → 数量；order: 当前移动订单）
export interface StateExt extends StateView {
  materials?: Record<string, number>
  order?: Vec2 | null
}

// GET/POST /api/profile —— Agent 养成档案
export interface ProfileSliders {
  adventure: number // 0..1 冒险 ↔ 谨慎
  cooperation: number // 0..1 合作 ↔ 竞争
  exploration: number // 0..1 探索 ↔ 利用
}

export interface ProfileLesson {
  text: string
  enabled: boolean
}

export interface Profile {
  persona: string
  sliders: ProfileSliders
  lessons: ProfileLesson[]
  updated: string
}

// POST /api/match/{id}/tick 的组装意图
export interface BlueprintSpec {
  type: number
  essence?: number
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
