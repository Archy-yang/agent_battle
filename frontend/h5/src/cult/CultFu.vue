<script setup lang="ts">
// src/cult/CultFu.vue —— 洞府叙事流：感知流（对话式正序）+ 心念犹豫卡 +
// 心念卡（映射真意图）+ 传念输入 + 逐渐被确认的东西（台账/假说/功法）
// + 筑基迹象（五点档点阵，band 数字绝不渲染）。
// 语态：第一人称内观——内部分身的产出一律呈现为玩家自己的感受与犹豫，
// 后端文案经 state.firstPerson 转写后再渲染。
import { computed, nextTick, ref, watch } from 'vue'
import { cult, act, guide, startProtocol, knownPoints, firstPerson } from './state'
import type { ProtoView } from './api'
import CultScene from './CultScene.vue'

// ---------------------------------------------------------------------------
// 感知流：后端 sense_stream 新→旧，前端 reverse 成对话式正序（历史在上、
// 最新在最下）。折起态只露最新两行（flex-end + 顶部渐隐），展开态限高内滚；
// 用户贴近底部时新消息自动跟随，上翻读史不拽走。传念回执是前端本地行，
// 按 tick 与流合并排序。
// ---------------------------------------------------------------------------
interface ChronLine { tick: number; kind: string; text: string; key: string }
const echoes = ref<{ tick: number; text: string }[]>([])

const chron = computed<ChronLine[]>(() => {
  const v = cult.view
  if (!v) return []
  const lines = [...v.sense_stream].reverse()
  const out: ChronLine[] = []
  for (const l of lines) {
    const prev = out[out.length - 1]
    if (prev && prev.kind === l.kind && prev.text === l.text) continue // 相邻同文不叠影
    out.push({ tick: l.tick, kind: l.kind, text: firstPerson(l.text), key: `l${l.tick}-${out.length}` })
  }
  const all = [
    ...out,
    ...echoes.value.map((e, i) => ({ tick: e.tick, kind: 'echo', text: e.text, key: `e${i}-${e.tick}` })),
  ]
  all.sort((a, b) => a.tick - b.tick) // 稳定排序：同 tick 保持正序
  return all
})

function lineCls(kind: string): string {
  if (kind === 'receipt' || kind === 'echo') return 'rc'
  if (kind === 'voice') return 'voice'
  if (kind === 'event') return 'ev'
  return ''
}

const streamOpen = ref(false)
const streamEl = ref<HTMLElement | null>(null)
const nearBottom = ref(true)
function onStreamScroll() {
  const el = streamEl.value
  if (!el) return
  nearBottom.value = el.scrollHeight - el.scrollTop - el.clientHeight < 28
}
function toggleStream() {
  streamOpen.value = !streamOpen.value
  if (streamOpen.value) nextTick(() => { const el = streamEl.value; if (el) el.scrollTop = el.scrollHeight })
}
watch(
  () => chron.value.length,
  () => {
    if (!streamOpen.value || !nearBottom.value) return
    nextTick(() => { const el = streamEl.value; if (el) el.scrollTop = el.scrollHeight })
  },
)

// ---------------------------------------------------------------------------
// 心念卡：建议层，背后映射真意图（保持 2~3 张）。
// ---------------------------------------------------------------------------
const OPEN_RANK: Record<string, number> = { 闭: 0, 微开: 1, 半开: 2, 大开: 3, 洞开: 4 }

const moving = computed(() => (cult.view?.points ?? []).some((p) => p.flow !== '停滞'))

// 可纳之穴：已知、且开度非「闭」，取开度最高者
const absorbable = computed(() => {
  const v = cult.view
  if (!v) return null
  const kn = knownPoints(v)
  let best: { name: string; rank: number } | null = null
  for (const p of v.points) {
    if (!kn.has(p.name)) continue
    const rank = OPEN_RANK[p.openness] ?? 0
    if (rank > 0 && (!best || rank > best.rank)) best = { name: p.name, rank }
  }
  return best
})

interface Choice { key: string; label: string; sub?: string; cls?: string; run: () => void }
const choices = computed<Choice[]>(() => {
  const v = cult.view
  if (!v) return []
  const out: Choice[] = []
  const activeProto = v.active ? v.protocols.find((x) => x.name === v.active) : undefined
  const running = !!(v.active && activeProto && !activeProto.paused)
  if (running) {
    out.push({ key: 'tick', label: '按功法运转', sub: `依《${v.active}》之序，再行一段`, cls: 'primary', run: () => void act([], 1) })
  } else if (activeProto?.paused) {
    // 已挂起：续行（resume 意图），而非空转一息
    out.push({ key: 'resume', label: `续行《${activeProto.name}》`, sub: '此功已挂起，只待你定夺', cls: 'primary', run: () => void act([{ kind: 'resume' }]) })
  } else if (v.tick > 0) {
    // 后端约束：尚无观察（tick 0）时起功必被拒，先静观一息再依诀行周天
    const p = v.protocols.find((x) => !x.active)
    if (p) out.push({ key: 'start', label: `依《${p.name}》行周天`, sub: '以所学之序导气归元', cls: 'primary', run: () => void startProtocol(p.name) })
  }
  const ab = absorbable.value
  if (ab && !running) {
    out.push({ key: 'absorb', label: '引一缕入体', sub: `自「${ab.name}」试试，感知到的能否纳入身`, run: () => void act([{ kind: 'absorb', target: ab.name, level: 'small' }]) })
  }
  if (moving.value) {
    out.push({ key: 'stop', label: '止息', sub: '收势静养', cls: 'warn', run: () => void act([{ kind: 'stop' }]) })
  }
  out.push({ key: 'sense', label: '静观一息', sub: '凝神内照，看看身内如何', cls: out.length ? '' : 'primary', run: () => void act([{ kind: 'sense' }]) })
  return out.slice(0, 3)
})

// —— 心念犹豫卡（置顶）：内部分身的请示呈现为玩家自己的踌躇；
// 按钮映射 resume/stop 意图（映射逻辑不变） ——
function reqTitle(kind: string): string {
  if (kind === 'out_of_boundary') return '气息似要越出熟悉的路径，你有些拿不准'
  if (kind === 'unknown_anomaly') return '身有异象，你一时不知深浅'
  return '气息过盛，行功受阻，你起了停手的念头'
}
function approve() { void act([{ kind: 'resume' }]) }
function halt() { void act([{ kind: 'stop' }]) }

// ---------------------------------------------------------------------------
// 传念：影响分身的取向，不直接改动身内流转
// ---------------------------------------------------------------------------
const thought = ref('')
const toastText = ref('')
let toastTimer = 0
function toast(t: string) {
  toastText.value = t
  window.clearTimeout(toastTimer)
  toastTimer = window.setTimeout(() => { toastText.value = '' }, 2400)
}
function sendThought() {
  const t = thought.value.trim()
  if (!t) return
  thought.value = ''
  echoes.value.push({ tick: cult.view?.tick ?? 0, text: `你传下一念：「${t}」` })
  toast('念头已至。它会牵动你摸索的取向，不直接改动身内流转。')
  void guide('guide', t)
}

// ---------------------------------------------------------------------------
// 逐渐被确认的东西：台账 + 假说（凭据措辞）+ 功法卡
// ---------------------------------------------------------------------------
const knowledgeEmpty = computed(() => {
  const v = cult.view
  if (!v) return true
  return !v.ledger.length && !v.hypotheses.length && !v.protocols.length
})

function protoState(p: ProtoView): string {
  if (p.active && p.paused) return '已挂起 · 只待你定夺'
  if (p.active) return p.loop ? '周天自转 · 不假心念' : '运转中'
  return p.status === 'validated' ? '已验证 · 未运转' : '草创 · 未运转'
}

// ---------------------------------------------------------------------------
// 筑基迹象：五点档点阵 + 档位词（band 只用于画点，数字绝不渲染）
// ---------------------------------------------------------------------------
const realm = computed(() => cult.view?.realm ?? '')

const ritual = ref(false)
let ritualShown = false
watch(realm, (r) => {
  if (r === '筑基' && !ritualShown) {
    ritualShown = true
    window.setTimeout(() => { ritual.value = true }, 700)
  }
})
</script>

<template>
  <div class="cu-page" v-if="cult.view">
    <CultScene />

    <section class="cu-panel">
      <div class="sec row">感 知 <button class="tgl" @click="toggleStream">{{ streamOpen ? '收 起' : '展 开' }}</button></div>
      <div
        ref="streamEl" class="cu-stream" :class="streamOpen ? 'open' : 'fold'"
        @scroll="onStreamScroll"
      >
        <div v-if="!chron.length" class="ln">闭上眼，四周寂静。你还不知世间有无「灵气」，也不知此身何处可承它。</div>
        <div v-for="l in chron" :key="l.key" class="ln" :class="lineCls(l.kind)">{{ l.text }}</div>
      </div>

      <!-- 犹豫卡（置顶）：呈现为玩家自己的踌躇 -->
      <div class="cu-request" v-for="r in cult.view.requests" :key="r.id">
        <div class="rt">你犹豫了——{{ reqTitle(r.kind) }}</div>
        <div class="rx">{{ firstPerson(r.text) }}</div>
        <div class="btns">
          <button class="go" :disabled="cult.busy" @click="approve">继续前行</button>
          <button class="halt" :disabled="cult.busy" @click="halt">就此止息</button>
        </div>
      </div>

      <div class="sec">此刻，你想——</div>
      <div class="cu-choices">
        <button
          v-for="c in choices" :key="c.key" class="choice" :class="c.cls"
          :disabled="cult.busy" @click="c.run"
        >{{ c.label }}<small v-if="c.sub">{{ c.sub }}</small></button>
      </div>

      <div class="cu-intent">
        <input v-model="thought" placeholder="传一个念头，如：先求稳，别贪快" @keydown.enter="sendThought" />
        <button :disabled="cult.busy" @click="sendThought">传念</button>
      </div>

      <div class="sec">逐渐被确认的东西</div>
      <div class="cu-know-empty" v-if="knowledgeEmpty">静室尚无一物可记。待你摸索出足够稳定的观察，此处自会生枝添叶。</div>
      <div class="cu-knowledge" v-else>
        <div class="kn" v-for="(l, i) in cult.view.ledger.slice(0, 12)" :key="'k' + i">{{ firstPerson(l) }}</div>
        <div class="kn hyp" v-for="(h, i) in cult.view.hypotheses" :key="'h' + i">
          <b>{{ firstPerson(h.text) }}</b>　{{ h.status }}——{{ firstPerson(h.evidence) }}
        </div>
        <div class="cu-art" v-for="p in cult.view.protocols" :key="p.name">
          <b>《{{ p.name }}》</b>
          <p>{{ firstPerson(p.path) }}</p>
          <p class="meta">{{ protoState(p) }}<template v-if="p.guards.length"> · 护卫：{{ p.guards.join('、') }}</template></p>
        </div>
      </div>

      <div class="sec">筑基迹象</div>
      <div class="cu-five">
        <div class="frow" :class="{ g: f.band >= 3 }" v-for="f in cult.view.foundation" :key="f.key">
          <span class="fk">{{ f.key }}</span>
          <span class="dots"><i v-for="i in 5" :key="i" :class="{ f: i <= f.band }"></i></span>
          <span class="word">{{ f.word }}</span>
        </div>
      </div>
    </section>

    <div class="cu-toast" :class="{ show: toastText }">{{ toastText }}</div>
    <div class="cu-ritual" :class="{ show: ritual }" @click.self="ritual = false">
      <div>
        <div class="ring" aria-hidden="true"></div>
        <h2 class="kai">筑 基</h2>
        <p>你已不再推动气息，周天却仍在自行运转。灵气往来不再依赖每一次心念，而成了这具身体自己的呼吸。</p>
        <button @click="ritual = false">继续内观</button>
      </div>
    </div>
  </div>
</template>
