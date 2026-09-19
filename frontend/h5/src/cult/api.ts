// src/cult/api.ts —— /api/cult/* 客户端与类型（与 backend/ctmatch/api.go
// 的 JSON 契约一致）。
//
// 感知隔离律的前端落点：本文件与全部修炼视图只消费定性词 / band 序号 /
// 记账数字（tick、周天数）；SessionView.debug 是真值载荷，类型上刻意
// 留空（unknown）且任何视图不得引用。

export interface StreamLine {
  tick: number
  kind: string // sense | receipt | event | voice
  text: string
}

export interface ReceiptLine {
  tick: number
  text: string
}

export interface PlayerLine {
  tick: number
  summary: string
}

export interface RequestCard {
  id: string
  kind: string // guard_stop | out_of_boundary | unknown_anomaly
  text: string
  tick: number
}

export interface EnvChip {
  element: string
  qi: string
  activity: string
}

export interface PointView {
  name: string
  feelings: string[]
  flow: string
  nature: string[]
  openness: string
  stability: string
  damage: string
  pressure_band: number // 0-5，供色带
}

export interface ChannelView {
  name: string
  from: string
  to: string
}

export interface HypView {
  text: string
  status: string
  evidence: string
}

export interface RangeView {
  channel: string
  element: string
  min: number
  max: number
}

export interface ProtoView {
  name: string
  status: string // draft | validated
  path: string
  guards: string[]
  ranges: RangeView[]
  active: boolean
  paused: boolean
  loop: boolean
  cycles: number
  current: number
}

export interface CycleLine {
  cycle: number
  success: boolean
  fail_why?: string
}

export interface TrialView {
  protocol: string
  cycle: number
  total: number
  loop: boolean
  recent: CycleLine[]
}

export interface FiveView {
  key: string
  word: string
  band: number // 0-4，供进度条
}

export interface CultView {
  id: string
  seed: number
  tick: number
  acuity: number
  realm: string
  pilot_kind: string
  llm_available: boolean
  env: EnvChip[]
  points: PointView[]
  body: { points: string[]; channels: ChannelView[] }
  self_report: string
  cognitions: string[]
  sense_stream: StreamLine[]
  receipts: ReceiptLine[]
  my_intents: PlayerLine[]
  requests: RequestCard[]
  ledger: string[]
  hypotheses: HypView[]
  protocols: ProtoView[]
  active: string
  trial?: TrialView
  foundation: FiveView[]
  /** 真值载荷：仅 ?debug=1 存在，前端零消费 */
  debug?: unknown
}

/** 玩家意图的请求形态（backend ctmatch.playerIntent） */
export interface PlayerIntent {
  kind: 'sense' | 'absorb' | 'guide' | 'stop' | 'inspect' | 'hypothesis' | 'resume'
  target?: string
  from?: string
  to?: string
  element?: string
  level?: string
  text?: string
}

async function req<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  })
  const data = await res.json().catch(() => ({}))
  if (!res.ok) {
    const msg = (data as { error?: string }).error ?? `HTTP ${res.status}`
    throw new Error(msg)
  }
  return data as T
}

export function createCultSession(pilot: 'script' | 'llm' = 'script', seed?: number): Promise<CultView> {
  return req<CultView>('POST', '/api/cult/session', { pilot, seed })
}

export function getCultView(id: string, debug = false): Promise<CultView> {
  return req<CultView>('GET', `/api/cult/session/${id}${debug ? '?debug=1' : ''}`)
}

export function tickCult(id: string, ticks: number, intents?: PlayerIntent[]): Promise<CultView> {
  return req<CultView>('POST', `/api/cult/session/${id}/tick`, { ticks, intents })
}

export function sendCultIntents(id: string, intents: PlayerIntent[]): Promise<CultView> {
  return req<CultView>('POST', `/api/cult/session/${id}/intents`, { intents })
}

export function sendGuidance(id: string, kind: string, text: string): Promise<CultView> {
  return req<CultView>('POST', `/api/cult/session/${id}/guidance`, { kind, text })
}

export interface AskResult {
  answer: string
  degraded: boolean
  reason?: string
}

export function askCult(id: string, question: string): Promise<AskResult> {
  return req<AskResult>('POST', `/api/cult/session/${id}/ask`, { question })
}

export function startCultProtocol(id: string, name: string): Promise<CultView> {
  return req<CultView>('POST', `/api/cult/session/${id}/protocols/${encodeURIComponent(name)}/start`, {})
}
