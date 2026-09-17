// 全局共享状态 + 对局动作：所有视图组件只通过本模块通信，不在 view 里重复 fetch 逻辑。
import { computed, ref, watch } from 'vue'
import {
  api, essenceName,
  type BlueprintSpec, type ExploredCell, type GuidanceKind, type Hex,
  type PredictChoice, type StateView,
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

// M3.4 接入：敌人 ≤2 格或本回合有交战事件时为 true（服务端 in_combat 字段），
// 外壳据此自动切到对战页。
export const inCombat = computed(() => !!view.value?.in_combat)

// ---- 遭遇自动切换（M3.4）----
// combatArmed：切页武装位。in_combat 变 true 时只切一次；手动返回地图后，
// 直到 in_combat 变 false 重置武装，再次变 true 才会再次自动切换。
export const combatArmed = ref(true)
// 对战页导航请求计数（watchCombat 命中 +1；「前往对战」按钮 +1）
export const battleNavRequest = ref(0)
export const requestBattleNav = () => {
  battleNavRequest.value++
}

// 外壳（App.vue）挂载时调用一次：in_combat true → 请求切到对战页。
export function watchCombat() {
  watch(inCombat, (c) => {
    if (c) {
      if (combatArmed.value) {
        combatArmed.value = false
        requestBattleNav()
      }
    } else {
      combatArmed.value = true // 解除遭遇：重新武装
    }
  })
}

// ---------- 地图常量与渲染辅助 ----------
// v0.4 坐标硬切：{x,y} → {q,r}（设计 §7 决定 #2）。
// 兼容保留：MVP 场景的出生点（新代码请用 spawnCell）。
export const W = 12
export const H = 12
export const SPAWN: Hex = { q: 0, r: 10 }

// M3.4：地图尺寸由服务端 pack 名推导。v0.4 无限世界返回 null
// （渲染不依赖地图尺寸）；StateView 未下发 W/H。
export const mapDims = computed<{ w: number; h: number } | null>(() => {
  const pack = view.value?.pack ?? ''
  if (pack.includes('v02')) return { w: 24, h: 24 }
  if (pack.includes('v04') || pack.includes('infinite')) return null
  return { w: 12, h: 12 }
})

// v0.4 无限世界场景包（scenario/infinite-0.4）
export const isV04 = computed(() => {
  const pack = view.value?.pack ?? ''
  return pack.includes('v04') || pack.includes('infinite')
})

// 出生点：开新局（turn 0）时记下 self 初始位置；中途刷新页面拿不到
// 服务端 spawn 字段时不渲染 ⌂（mvp 场景回退到 SPAWN 常量）。
export const spawnPos = ref<Hex | null>(null)
export const spawnCell = computed<Hex | null>(() => {
  const pack = view.value?.pack ?? ''
  if (spawnPos.value) return spawnPos.value
  return pack.includes('v02') || pack.includes('infinite') || pack.includes('v04')
    ? null
    : SPAWN
})

function rememberSpawn() {
  const v = view.value
  if (!spawnPos.value && v?.observation && v.turn <= 1) {
    spawnPos.value = { q: v.observation.self.pos.q, r: v.observation.self.pos.r }
  }
}

export const exploredSet = computed(() => {
  const s = new Set<string>()
  for (const p of view.value?.explored ?? []) s.add(`${p.q},${p.r}`)
  return s
})

export const visibleSet = computed(() => {
  const s = new Set<string>()
  for (const p of view.value?.observation?.visible ?? []) s.add(`${p.q},${p.r}`)
  return s
})

// 已探索格的地形记忆（v0.4 服务端尚未下发 essence/danger；拿到时地图
// 直接按五行上色 / 火色描边，客户端绝不自造地形）。
export const exploredCellMap = computed(() => {
  const m = new Map<string, ExploredCell>()
  for (const p of view.value?.explored ?? []) m.set(`${p.q},${p.r}`, p)
  return m
})

export const entityAt = (q: number, r: number) =>
  view.value?.observation?.entities?.find(e => e.pos.q === q && e.pos.r === r)

export const selfAt = (q: number, r: number) =>
  view.value?.observation?.self.pos.q === q && view.value!.observation!.self.pos.r === r

// 已废弃（M3.4）：客户端地形先验。地形信息只能来自服务端 explored /
// observation（Reality ≠ Agent Knowledge），客户端不再自造地形。
// 保留导出仅为兼容，勿在新代码中使用；v0.4 接入服务端地形记忆后删除。
export const terrainDanger = (x: number, y: number) => x >= 3 && x <= 7 && y >= 3 && y <= 7
export const terrainEssence = (x: number, y: number): number => {
  if (terrainDanger(x, y)) return 1 // 火
  if (y <= 1) return 3 // 金
  if (x === 0) return 4 // 水
  return 2 // 土
}

// 迷雾三层（M3.4）：只读 view.explored 与 view.observation.visible。
// 服务端 explored 目前只有坐标、没有五行色 —— 「已探索未感知」渲染为
// 中性暗土色，可见格为亮一档的中性色；v0.4 接入服务端地形记忆后
// （explored 元素带 essence/danger 字段，见 api.ts ExploredCell）恢复五行地形色。
export const FOG = {
  unexplored: '#12161d',
  remembered: '#2a2f37',
  visible: '#3a424e',
}

export const cellStyle = (q: number, r: number) => {
  const key = `${q},${r}`
  const explored = exploredSet.value.has(key)
  const visible = visibleSet.value.has(key)
  if (!explored) {
    // 未探索：深底色
    return { background: FOG.unexplored }
  }
  if (!visible) {
    // 已探索离开感知：45% 透明度
    return { background: FOG.remembered, opacity: 0.45 }
  }
  // 当前感知内：全亮
  return { background: FOG.visible }
}

// 当前移动订单目标（move_to）：地图上用强调金虚线框标注
export const orderTarget = computed<Hex | null>(() => view.value?.order ?? null)

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
      ? seen.map(e => `${e.kind === 'fragment' ? '结晶' : e.owner === 'unknown' ? '未知体' : e.kind}·${essenceName(e.essence)} @${e.pos.q},${e.pos.r}`).join('；')
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
  rememberSpawn()
}

// 开新对局；scenario 选场景包："mvp"（经典 12×12，默认）| "v02"（大地图 24×24）
// | "v04"（无限六边形世界，scenario/infinite-0.4）。
// 追加可选参数，旧调用 newMatch() 行为不变。
export async function newMatch(scenario?: 'mvp' | 'v02' | 'v04') {
  busy.value = true
  error.value = ''
  try {
    view.value = await api<StateView>('/api/match', scenario ? { scenario } : {})
    spawnPos.value = null
    rememberSpawn()
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

// ---------- M3.4 点选寻路 ----------
// 地图页轻提示（点雾 / 不可通行 / 订单反馈），数秒后自动消失。
export const mapHint = ref('')
let mapHintTimer: ReturnType<typeof setTimeout> | null = null
export function flashHint(text: string) {
  mapHint.value = text
  if (mapHintTimer) clearTimeout(mapHintTimer)
  mapHintTimer = setTimeout(() => { mapHint.value = '' }, 3500)
}

// 点选寻路：一回合推进一步。POST tick {intents:[{type:"move_to",to:{q,r}}]} ——
// 订单不占意图预算，引擎每 tick 自动走一格；到达发 order_done，被挡发 order_blocked。
// 服务端无法通行/不可达时回 intent_rejected，从本回合事件里取出来做轻提示。
export async function sendMoveTo(q: number, r: number) {
  if (!view.value || busy.value || view.value.status !== 'running') return
  busy.value = true
  error.value = ''
  try {
    view.value = await api<StateView>(`/api/match/${view.value.id}/tick`, {
      intents: [{ type: 'move_to', to: { q, r } }],
    })
    rememberSpawn()
    const turn = view.value.turn
    const recent = (view.value.events ?? []).filter(e => e.tick === turn)
    const rejected = recent.find(e => e.type === 'intent_rejected')
    if (rejected) {
      flashHint(rejected.text) // 如「目标格不可通行 {q,r}」「目标格不可达 {q,r}」
    } else {
      flashHint(`移动订单：前往 (${q},${r})`)
    }
  } catch (e) {
    error.value = errText(e)
  } finally {
    busy.value = false
  }
}

// 待命一回合：POST tick 空意图（原地恢复散热）。与 step(false) 等价，
// 保留语义化导出供地图页按钮使用。
export const waitOneTurn = () => step(false)

// ---------- v0.5 采集与装备（WPC） ----------

// 采集：玩家站上晶簇格后发 transfer 意图（与拾取结晶同意图）。transfer
// 指向 Kind "resource" 的实体时由 match 层结算（引擎冻结）：成功事件
// type "gather"（物料入库），失败走 intent_rejected。
export async function sendTransfer(target: number) {
  if (!view.value || busy.value || view.value.status !== 'running') return
  busy.value = true
  error.value = ''
  try {
    view.value = await api<StateView>(`/api/match/${view.value.id}/tick`, {
      intents: [{ type: 'transfer', target }],
    })
    const turn = view.value.turn
    const recent = (view.value.events ?? []).filter(e => e.tick === turn)
    const rejected = recent.find(e => e.type === 'intent_rejected')
    const gathered = recent.find(e => e.type === 'gather')
    if (rejected) flashHint(rejected.text) // 如「必须站在晶簇所在格」
    else if (gathered) flashHint(gathered.text)
    else flashHint('采集请求已提交')
  } catch (e) {
    error.value = errText(e)
  } finally {
    busy.value = false
  }
}

// 装备自身：POST tick {intents:[{type:"equip",blueprint:[…]}]}。躯壳(0)/
// 储气(1) 会被后端拒绝（身体之根，不可 equip）；身体组件数上限 8。
// 结果反馈取返回 state 的 events 里最近 3 条 equip / intent_rejected。
export const equipBusy = ref(false)
export const equipError = ref('')
export const equipLog = ref<{ tick: number; text: string; rejected: boolean }[]>([])

export async function sendEquip(blueprint: BlueprintSpec[]) {
  if (!view.value || equipBusy.value || view.value.status !== 'running') return
  equipBusy.value = true
  equipError.value = ''
  try {
    view.value = await api<StateView>(`/api/match/${view.value.id}/tick`, {
      intents: [{ type: 'equip', blueprint }],
    })
    equipLog.value = (view.value.events ?? [])
      .filter(e => e.type === 'equip' || e.type === 'intent_rejected')
      .slice(-3)
      .map(e => ({ tick: e.tick, text: e.text, rejected: e.type === 'intent_rejected' }))
  } catch (e) {
    equipError.value = errText(e)
  } finally {
    equipBusy.value = false
  }
}

// LLM 触发原因中文映射（BattleView 顶部反馈条；threshold:* 带后缀，前缀匹配）。
// 覆盖 combat/arrive(ral)/encounter/resource/threshold/heartbeat/guidance；
// 未知值回退显示原文（triggerLabel 尾部 return t），不显示空白。
const TRIGGER_LABELS: Record<string, string> = {
  combat: '接战',
  arrive: '抵达',
  arrival: '抵达',
  encounter: '遭遇',
  resource: '发现',
  threshold: '状态告警',
  heartbeat: '例行反思',
  guidance: '玩家干预',
}

export const triggerLabel = (t?: string): string => {
  if (!t) return '—'
  if (TRIGGER_LABELS[t]) return TRIGGER_LABELS[t]
  if (t.startsWith('threshold')) return TRIGGER_LABELS.threshold
  return t
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
