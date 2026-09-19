// src/cult/state.ts —— 修炼模式的状态store：会话生命周期、轮询、动作。
// 全部数据来自 /api/cult/* 的定性视图（见 api.ts 的隔离律注记）。

import { reactive } from 'vue'
import {
  createCultSession, getCultView, tickCult, sendCultIntents,
  sendGuidance, askCult, startCultProtocol,
  type CultView, type PlayerIntent,
} from './api'

interface CultState {
  view: CultView | null
  error: string
  askAnswer: string
  askDegraded: string
  busy: boolean
  // gate = 开局仪式（唤醒分身）尚未完成。开局是产品语义的一部分
  // （设计稿：本尊入世），pilot 选择不属于玩家语义——默认请 llm，
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

let pollTimer: ReturnType<typeof setInterval> | null = null

function startPolling(): void {
  if (pollTimer == null) {
    pollTimer = setInterval(() => { void refreshCult() }, 3000)
  }
}

/** 开局仪式「唤醒分身」：建会话并开始轮询。llm 未配置时自动落脚本。 */
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
        cult.error = '灵机未启，以脚本分身入定（不耗调用）'
      } else {
        throw e
      }
    }
    cult.gate = false
    cult.error = ''
    startPolling()
  } catch (e) {
    cult.error = String(e instanceof Error ? e.message : e)
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
    cult.error = ''
  } catch (e) {
    cult.error = String(e instanceof Error ? e.message : e)
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
    cult.error = String(e instanceof Error ? e.message : e)
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
    cult.error = String(e instanceof Error ? e.message : e)
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
    cult.error = String(e instanceof Error ? e.message : e)
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
    cult.error = String(e instanceof Error ? e.message : e)
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
      cult.askDegraded = r.reason ?? '分身无法应答'
    } else {
      cult.askAnswer = r.answer
    }
  } catch (e) {
    cult.askDegraded = String(e instanceof Error ? e.message : e)
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
    cult.error = String(e instanceof Error ? e.message : e)
  } finally {
    cult.busy = false
  }
}
