// src/cult/state.ts —— 修炼模式的状态store：会话生命周期、轮询、动作。
// 全部数据来自 /api/cult/* 的定性视图（见 api.ts 的隔离律注记）。

import { reactive } from 'vue'
import {
  createCultSession, getCultView, tickCult, sendCultIntents,
  sendGuidance, askCult, startCultProtocol,
  type CultView, type PlayerIntent, type PointView,
} from './api'
interface CultState {
  view: CultView | null
  error: string
  askAnswer: string
  askDegraded: string
  busy: boolean
  // gate = 开局仪式（入定）尚未完成。开局是产品语义的一部分
  // （第一人称内观），pilot 选择不属于玩家语义——默认请 llm，
  // 后端未配置时自动落 script 并告知。
  gate: boolean
  awakening: boolean
}

export const cult = reactive<CultState>({
  view: null,
  error: '',
  askAnswer: '',
  askDegraded: '',
  busy: false,
  gate: true,
  awakening: false,
})

/**
 * 第一人称语态过滤：后端旧文案可能仍带「分身／本尊」的他者框架词，
 * 渲染进玩家界面前一律转成第一人称内观（player 即修炼者本人）。
 * 仅用于展示层，view 字段原文照存。
 */
export function firstPerson(msg: string): string {
  return String(msg)
    .replace(/【本尊】/g, '')
    .replace(/（分身入定，未留言。）/g, '（我已入定，未留言。）')
    .replace(/本尊/g, '你')
    .replace(/分身/g, '我')
    .replace(/它仍在凭感知修行/g, '身内流转仍在自行演化')
}

/**
 * 白话转写层（洞府行动台改版，2026-10-18 设计稿 §3）：引擎叙事文案一个字
 * 不动，渲染进玩家界面前经此集中转写——先过 firstPerson（第一人称内观），
 * 再过词表 + 句式映射，把骨架性文案转成"白话为主、文句点睛"。
 * 感知隔离律不变：只换说法，不减不增信息，零数字零机器词。
 */
const VERNAC_RULES: [RegExp, string][] = [
  // 流水标记 → 直说
  [/^【回执】/, ''],
  [/^【附言】/, '心里的话——'],
  // 回执措辞（ReceiptView 词表）
  [/于(.+?)纳气/g, '在$1引气'],
  [/（少量）/g, '（浅浅一试）'],
  [/（大量）/g, '（放开了引）'],
  [/已生效/g, '气应了'],
  [/此路不通/g, '这条路走不通'],
  [/穴未开或满溢不入/g, '那处窍穴还没开，或已经装不下了'],
  [/意图不明/g, '没说清要怎么做'],
  [/已记下/g, '记下了'],
  // 感知句式
  [/气机停滞/g, '气机没有动静'],
  // 事件句（api.go eventText 词表，经 firstPerson 后的人称）
  [/候你示下/g, '等你拿主意'],
  [/不敢妄进，特此请示/g, '不敢贸然往前，等你拿主意'],
  [/其故难明，已停功请示/g, '缘由难明，已先停功，等你拿主意'],
  [/不假我之思/g, '不再需要我时时操心'],
  [/暂歇（另有心意接管行功）/g, '先歇下来（另有一份心意接过了行功）'],
  // 请示卡措辞（requestCardFor 词表）
  [/触护卫而停（气机过盛）。是否再行功？/g, '被护卫按停了（气息太猛）。要接着行功吗？'],
  [/准其试探（有伤脉之险），抑或止之？/g, '由它试一试（可能伤着经脉），还是就此打住？'],
  [/是否续行？/g, '要继续吗？'],
]

export function vernacular(msg: string): string {
  let s = firstPerson(msg)
  for (const [re, to] of VERNAC_RULES) s = s.replace(re, to)
  return s
}

// ---------------------------------------------------------------------------
// 心念卡与引导语的共同推导（与 CultFu 的 choices 同源）：
// 传递的是状态判断，不是玩法分支——底层意图映射不变。
// ---------------------------------------------------------------------------

export const OPEN_RANK: Record<string, number> = { 闭: 0, 微开: 1, 半开: 2, 大开: 3, 洞开: 4 }

/** 气在流转：任一穴位气流非停滞（只作状态判断，不渲染）。 */
export function movingQi(view: CultView | null): boolean {
  return (view?.points ?? []).some((p) => p.flow !== '停滞')
}

/**
 * 可纳之穴：已显名（引擎 tier 给档）、且开度非「闭」，取开度最高者
 * （只对已得名字的穴出建议——自由引气同样只认真名，引擎按档门控）。
 */
export function absorbablePoint(
  view: CultView | null,
): { name: string; rank: number } | null {
  if (!view) return null
  let best: { name: string; rank: number } | null = null
  for (const p of view.points) {
    if (!isNamed(p)) continue
    const rank = OPEN_RANK[p.openness] ?? 0
    if (rank > 0 && (!best || rank > best.rank)) best = { name: p.name, rank }
  }
  return best
}

/**
 * 丹田是否已显名：自举的消失条件。全无感时没有任何节点可点、探索型
 * 分身在安静期也不出招——「意守丹田」（inspect 下丹田）是修真体感的
 * 第一步，也是世界推进的起点；丹田显名后由「自此引气」与常规建议接管。
 */
export function dantianNamed(view: CultView | null): boolean {
  return !!view?.points.some((p) => p.name === '下丹田')
}

/** 引导语：主屏行动区顶部一行「现在可以：……」，随状态推导。 */
export function guideLine(view: CultView | null): string {
  if (!view) return ''
  const activeProto = view.active ? view.protocols.find((x) => x.name === view.active) : undefined
  if (activeProto?.paused) return '现在可以：有功法在等你定夺'
  if (activeProto && activeProto.active && !activeProto.paused) return '现在可以：气息已在流动，可以继续运转或收势'
  if (movingQi(view)) return '现在可以：气息已在流动，可以继续运转或收势'
  if (!dantianNamed(view)) return '现在可以：先意守丹田，看身体会不会回应'
  if (absorbablePoint(view)) return '现在可以：试试把气息引进来'
  return '现在可以：接着摸索这具身体'
}

/** 修行进度：五关的档位白话解释（档位词 → 一句直说）。 */
export const FIVE_GLOSS: Record<string, string> = {
  循环: '气息能不能自己走成一圈',
  衡定: '身内几路气息之间是否相安',
  自持: '不靠外推，这份流转能不能自己撑住',
  守静: '气息起落大的时候，是否仍守得住',
  内化: '这套功夫是否长进了身体，成了它自己的',
}

/** 错误转译：后端错误可能带机器词（LLM/Runtime/pilot），玩家界面只许灵机话术。 */
export function humanizeError(msg: string): string {
  return firstPerson(String(msg)
    .replace(/LLM 未配置/g, '灵机未启，以内景演示入定')
    .replace(/LLM|大模型|语言模型/g, '灵机')
    .replace(/[Rr]untime/g, '内景')
    .replace(/[Pp]ilot/g, '心神')
    .replace(/HTTP \d+/g, '灵路不通'))
}

// ---------------------------------------------------------------------------
// 感知档（渐进感知，specs/2026-10-18-progressive-perception-v1.md §2）：
// 已知判定全改引擎给的 tier——旧 knownPoints 启发式（feelings 非空或
// 感知流提过名）已退役。渲染只认档位：无感不渲染；隐约/有感无名（以
// 方位语指称）；显名才得名字与浮卡渐全。熟识度数值引擎绝不外泄，前端
// 也只见档位词。
// ---------------------------------------------------------------------------

export const TIER_NONE = '无感'
export const TIER_VAGUE = '隐约'
export const TIER_FELT = '有感'
export const TIER_NAMED = '显名'

/** 未显名档的浮卡措辞（白话转写层：档位词 → 一句体感话，零数字零机器词）。 */
export const TIER_GLOSS: Record<string, string> = {
  [TIER_VAGUE]: '似有一点动静，辨不真切',
  [TIER_FELT]: '那处有了实感，还叫不出它的名字',
}

/** 已显名（得名字）判定：唯一口径是引擎 tier，前端不再自行推断。 */
export function isNamed(p: PointView): boolean {
  return p.tier === TIER_NAMED && !!p.name
}

let pollTimer: ReturnType<typeof setInterval> | null = null

function startPolling(): void {
  if (pollTimer == null) {
    pollTimer = setInterval(() => { void refreshCult() }, 3000)
  }
}

/** 开局仪式「入定」：建会话并开始轮询。llm 未配置时自动落脚本。 */
export async function awaken(pilot: 'llm' | 'script', goal: string): Promise<void> {
  if (cult.awakening) return
  cult.awakening = true
  try {
    if (goal.trim()) {
      try { localStorage.setItem('cult_self_goal', goal) } catch { /* 私有浏览态 */ }
    }
    try {
      cult.view = await createCultSession(pilot)
    } catch (e) {
      if (pilot === 'llm' && String(e instanceof Error ? e.message : e).includes('LLM 未配置')) {
        cult.view = await createCultSession('script')
        cult.error = '灵机未启，以内景演示入定（不耗调用）'
      } else {
        throw e
      }
    }
    cult.gate = false
    cult.error = ''
    startPolling()
  } catch (e) {
    cult.error = humanizeError(e instanceof Error ? e.message : e)
  } finally {
    cult.awakening = false
  }
}

export function stopCultPolling(): void {
  if (pollTimer != null) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

export async function refreshCult(): Promise<void> {
  if (!cult.view || cult.busy) return
  try {
    cult.view = await getCultView(cult.view.id)
    // 注意：轮询成功不清 error——动作报错（如起功被拒）须留给玩家读到，
    // 只有下一次玩家动作成功才清除（见 act/guide/startProtocol）。
  } catch (e) {
    cult.error = humanizeError(e instanceof Error ? e.message : e)
  }
}

export async function newCultSession(pilot: 'script' | 'llm' = 'script'): Promise<void> {
  cult.busy = true
  try {
    cult.view = await createCultSession(pilot)
    cult.error = ''
    cult.askAnswer = ''
    cult.askDegraded = ''
  } catch (e) {
    cult.error = humanizeError(e instanceof Error ? e.message : e)
  } finally {
    cult.busy = false
  }
}

/** 发意图并立即推进一个 Tick（四动作按钮：结果马上可见）。 */
export async function act(intents: PlayerIntent[], ticks = 1): Promise<void> {
  if (!cult.view || cult.busy) return
  cult.busy = true
  try {
    cult.view = await tickCult(cult.view.id, ticks, intents)
    cult.error = ''
  } catch (e) {
    cult.error = humanizeError(e instanceof Error ? e.message : e)
  } finally {
    cult.busy = false
  }
}

/** 只发意图不推进（如挂起/恢复功法、请示的"再等等"）。 */
export async function intend(intents: PlayerIntent[]): Promise<void> {
  if (!cult.view || cult.busy) return
  cult.busy = true
  try {
    cult.view = await sendCultIntents(cult.view.id, intents)
    cult.error = ''
  } catch (e) {
    cult.error = humanizeError(e instanceof Error ? e.message : e)
  } finally {
    cult.busy = false
  }
}

/** 推进 n 个 Tick（修炼加速）。 */
export async function advance(n: number): Promise<void> {
  await act([], n)
}

export async function guide(kind: string, text: string): Promise<void> {
  if (!cult.view || cult.busy) return
  cult.busy = true
  try {
    cult.view = await sendGuidance(cult.view.id, kind, text)
    cult.error = ''
  } catch (e) {
    cult.error = humanizeError(e instanceof Error ? e.message : e)
  } finally {
    cult.busy = false
  }
}

export async function ask(question: string): Promise<void> {
  if (!cult.view || cult.busy) return
  cult.busy = true
  cult.askAnswer = ''
  cult.askDegraded = ''
  try {
    const r = await askCult(cult.view.id, question)
    if (r.degraded) {
      cult.askDegraded = r.reason ? humanizeError(r.reason) : '心念一时无法应答'
    } else {
      cult.askAnswer = r.answer
    }
  } catch (e) {
    cult.askDegraded = humanizeError(e instanceof Error ? e.message : e)
  } finally {
    cult.busy = false
  }
}

export async function startProtocol(name: string): Promise<void> {
  if (!cult.view || cult.busy) return
  cult.busy = true
  try {
    cult.view = await startCultProtocol(cult.view.id, name)
    cult.error = ''
  } catch (e) {
    cult.error = humanizeError(e instanceof Error ? e.message : e)
  } finally {
    cult.busy = false
  }
}
