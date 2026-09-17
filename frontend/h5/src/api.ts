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

export interface Guidance {
  turn: number
  kind: string
  text: string
}

export interface StateView {
  id: string
  seed: number
  pack: string
  status: 'running' | 'won' | 'lost' | 'timeout'
  turn: number
  turn_limit: number
  observation: Observation | null
  explored: Vec2[]
  events: Event[]
  guidance: Guidance[]
}

const ESSENCE_NAMES = ['木', '火', '土', '金', '水'] as const

export const essenceName = (e: number): string => ESSENCE_NAMES[e] ?? '?'

const ESSENCE_COLORS = ['#4a9', '#e55', '#cb6', '#ccc', '#59d'] as const

export const essenceColor = (e: number): string => ESSENCE_COLORS[e] ?? '#888'

export async function api<T>(path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method: body === undefined ? 'GET' : 'POST',
    headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error((err as { error?: string }).error ?? res.statusText)
  }
  return res.json() as Promise<T>
}
