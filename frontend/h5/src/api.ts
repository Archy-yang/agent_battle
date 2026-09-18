// 与 backend/match/api.go 的 JSON 契约保持一致。
// v0.4：坐标 {x,y} → {q,r} 一次性硬切（设计 §7 决定 #2），Vec2 删除，
// 不保留别名。

/** 轴向六边形坐标（engine.Hex，pointy-top） */
export interface Hex {
  q: number
  r: number
}

/** @deprecated 旧方形坐标类型，仅剩 types-battle.ts 的类型引用；新代码一律用 Hex */
export type Vec2 = Hex

export interface Event {
  tick: number
  type: string
  text: string
  entity?: number
}

export interface SelfView {
  pos: Hex
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
  pos: Hex
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
  visible: Hex[]
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
  explored: ExploredCell[]
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
  order?: Hex | null
  // ---- v0.5 追加字段（backend/match/api.go StateView 已实现，旧对局下为 undefined） ----
  // 最近一次 LLM 触发原因（combat/arrive/encounter/resource/threshold:*/heartbeat/guidance）
  last_trigger?: string
  // 本局 LLM 调用计数
  llm_calls?: number
  // ---- v0.6 追加字段（backend/match/api.go StateView 已实现，旧对局下为 undefined） ----
  // 克关系战术预览：感知内每个敌人的"我的火 → 它的本源"落区推算
  // （与 enemies 同感知边界，信息边界不变）
  tactics?: TacticsView[]
  // ---- 战斗解说追加字段（backend/match/api.go StateView 已实现，旧对局下为 undefined） ----
  // combat 触发的 LLM 战术解说（Agent 的话，≤50 字第一人称）。它们同时
  // 以 type "commentary" 混在 events 展示流里；此字段供单独消费。
  commentary?: Event[]
  // ---- v0.9 追加字段（敌对 Agent / rival，backend 并行开发中，旧对局下为 undefined） ----
  // rival 的状态视图（感知内或已知信息；不可见/未实现时为 null 或 undefined）
  rival?: RivalView | null
  // rival 最近一次触发原因（对称于 last_trigger；可能 undefined）
  rival_trigger?: string
}

// v0.9：敌对 Agent（rival）状态视图（backend StateView.rival）。
// rival = LLM/脚本驱动的敌对 Agent 实体（kind:"agent"、owner:"rival"），
// 走与玩家对称的 AgentProtocol。essence 服务端可能下发数字索引或五行
// 中文名（两套视图现状不一致），前端按 number | string 容错处理。
// 所有字段均可能缺失（旧对局 / 后端未上线），消费方必须 ?? 兜底。
export interface RivalView {
  id: number
  pos: Hex
  essence: number | string
  integrity: number
  max_integrity: number
  heat: number
}

// M3.4：感知范围内的一名敌对实体（backend EnemyView）。
export interface EnemyView {
  id: number
  pos: Hex
  distance: number
  essence: string // 五行中文名：木/火/土/金/水
  integrity: number
  heat: number
  // v0.9：实体归属；rival = 敌对 Agent。旧后端不下发（undefined），
  // 归属判定须配合 StateView.rival.id 匹配兜底。
  owner?: string
}

// v0.6：克关系战术预览卡（backend TacticsView）。
// predicted_zone ∈ 侮|微|衡|乘|中性；无离火时 my_essence 与
// predicted_zone 为空串，note 提示"无离火，无法反击"。
export interface TacticsView {
  enemy_id: number
  my_essence: string // 我方离火本源（五行中文名）
  enemy_essence: string // 敌方本源（五行中文名）
  predicted_zone: string
  my_power_guess: number // 预估作用功率（默认 600）
  note: string // 一句战术建议
}

// v0.4 计划：explored 元素从纯坐标扩展为带地形记忆的格子
// （essence/danger/passable 由服务端下发）；当前服务端只发 {q,r}，
// 前端把"已探索未感知"渲染为中性暗色，不猜测地形（Reality ≠ Agent Knowledge）。
export type ExploredCell = Hex & {
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
  order?: Hex | null
}

// ---- v0.7 对局录像与重放（backend/match/recording.go 的 JSON 契约） ----

// Recording 的一帧：该 tick 实际进入引擎的玩家侧意图与随之产生的事件。
// player_intents 原样透传（前端不解析意图结构，回放时整包回传服务端）；
// Go 侧空意图 slice 序列化为 null，前端需 ?? [] 兜底。
export interface ReplayFrame {
  tick: number
  player_intents: unknown[] | null
  auto: boolean
  resulting_events: Event[]
}

// GET /api/match/{id}/recording 的返回；POST /api/replay 的 body 主体
// （内联 frames）+ until_tick。
export interface Recording {
  id: string
  seed: number
  scenario: string
  frames: ReplayFrame[]
  guidance?: Guidance[]
}

// POST /api/replay 请求体：Recording 展开（内联）+ 重演到的 tick。
export interface ReplayRequest extends Recording {
  until_tick: number
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
