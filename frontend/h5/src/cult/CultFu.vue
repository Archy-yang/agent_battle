<script setup lang="ts">
// src/cult/CultFu.vue —— 洞府行动台主屏（一屏化改版，2026-10-18 设计稿 §1/§2）：
// 身体场景（弹性）+ 感知条（单行最新，点开「心声」浮层看全史）+ 行动区
// （引导语 + 心念卡≤2 + 传念行）+ 底部行（身体记事 / 修行进度浮层入口 + 忙碌态）。
// 犹豫卡改为模态置顶强提示；台账/假说/功法迁「身体记事」浮层；
// 筑基迹象迁「修行进度」浮层；历史感知流迁「心声」浮层。
// 语态：第一人称内观——后端文案经 state.vernacular 转写后再渲染
// （白话为主、文句点睛），band 数字绝不渲染。
import { computed, nextTick, ref, watch } from 'vue'
import {
  cult, act, guide, startProtocol, vernacular, guideLine,
  movingQi, absorbablePoint, dantianNamed, FIVE_GLOSS,
} from './state'
import type { ProtoView } from './api'
import CultScene from './CultScene.vue'
import CultSheet from './CultSheet.vue'

// ---------------------------------------------------------------------------
// 感知流：后端 sense_stream 新→旧，前端 reverse 成对话式正序（历史在上、
// 最新在最下）。主屏只露最新一行（感知条）；全史住在「心声」浮层里，
// 打开时贴底跟随，上翻读史不拽走。传念回执是前端本地行，按 tick 与流合并。
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
    out.push({ tick: l.tick, kind: l.kind, text: vernacular(l.text), key: `l${l.tick}-${out.length}` })
  }
  const all = [
    ...out,
    ...echoes.value.map((e, i) => ({ tick: e.tick, kind: 'echo', text: e.text, key: `e${i}-${e.tick}` })),
  ]
  all.sort((a, b) => a.tick - b.tick) // 稳定排序：同 tick 保持正序
  return all
})

// 感知条：最新一行（白话、渐隐入场）
const latest = computed(() => chron.value[chron.value.length - 1] ?? null)
const quietLine = '闭上眼，四周寂静。先静静感受一会儿，看身体会不会给你回应。'

// —— 心声浮层：全史对话式正序，贴底跟随 ——
const voiceOpen = ref(false)
const knowOpen = ref(false)
const progressOpen = ref(false)
// 浮层互斥守卫：JS 连点可让多个 sheet 同时持 show（真实触摸被遮罩挡住
// 触发不到，仍守卫之），任一开启即收起其余。
watch([voiceOpen, knowOpen, progressOpen], ([v, k, p], [ov, ok2, op]) => {
  if (v && ov !== v) { knowOpen.value = false; progressOpen.value = false }
  else if (k && ok2 !== k) { voiceOpen.value = false; progressOpen.value = false }
  else if (p && op !== p) { voiceOpen.value = false; knowOpen.value = false }
})
const streamEl = ref<HTMLElement | null>(null)
const nearBottom = ref(true)
function onStreamScroll() {
  const el = streamEl.value
  if (!el) return
  nearBottom.value = el.scrollHeight - el.scrollTop - el.clientHeight < 28
}
watch([voiceOpen, () => chron.value.length], ([open]) => {
  if (!open || !nearBottom.value) return
  nextTick(() => { const el = streamEl.value; if (el) el.scrollTop = el.scrollHeight })
})
watch(voiceOpen, (open) => {
  if (open) nearBottom.value = true
})

function lineCls(kind: string): string {
  if (kind === 'receipt' || kind === 'echo') return 'rc'
  if (kind === 'advice') return 'rc' // 会话附言（转向提示）：与回执同源的定性语
  if (kind === 'voice') return 'voice'
  if (kind === 'event') return 'ev'
  return ''
}

// ---------------------------------------------------------------------------
// 心念卡：建议层（≤2 张），背后映射真意图——act/startProtocol 调用逻辑
// 与改版前一致，只换文案与布局。推导与引导语同源（state 的
// absorbablePoint / movingQi）。
// ---------------------------------------------------------------------------
interface Choice { key: string; label: string; sub?: string; cls?: string; run: () => void }
const choices = computed<Choice[]>(() => {
  const v = cult.view
  if (!v) return []
  const out: Choice[] = []
  const activeProto = v.active ? v.protocols.find((x) => x.name === v.active) : undefined
  const running = !!(v.active && activeProto && !activeProto.paused)
  if (running) {
    out.push({ key: 'tick', label: '继续运转', sub: `气息正顺，依《${v.active}》再行一段`, cls: 'primary', run: () => void act([], 1) })
  } else if (activeProto?.paused) {
    // 已挂起：续行（resume 意图），而非空转一息
    out.push({ key: 'resume', label: `继续《${activeProto.name}》`, sub: '这门功法正等着你拿主意', cls: 'primary', run: () => void act([{ kind: 'resume' }]) })
  } else if (v.tick > 0) {
    // 后端约束：尚无观察（tick 0）时起功必被拒，先感受一息再依诀行周天
    const p = v.protocols.find((x) => !x.active)
    if (p) out.push({ key: 'start', label: `照《${p.name}》走一遍周天`, sub: '按功法的路数，把气息引回丹田', cls: 'primary', run: () => void startProtocol(p.name) })
  }
  const ab = absorbablePoint(v)
  if (ab && !running) {
    out.push({ key: 'absorb', label: '试着引气入体', sub: `从「${ab.name}」那边试试，看能否纳入身`, run: () => void act([{ kind: 'absorb', target: ab.name, level: 'small' }]) })
  }
  if (movingQi(v)) {
    out.push({ key: 'stop', label: '收势停一停', sub: '让气息慢下来，静养片刻', cls: 'warn', run: () => void act([{ kind: 'stop' }]) })
  }
  // 自举卡（意守丹田）：全无感时没有任何节点可点、探索型分身在安静期
  // 也不出招——意守丹田（inspect）是修真体感的第一步，也是世界推进的
  // 起点；排在「静静感受一会儿」之前（这一步该做的事）。丹田显名后此卡
  // 消失，由浮卡「自此引气」与常规建议接管。
  if (!dantianNamed(v) && !running) {
    out.push({
      key: 'dantian', label: '意守丹田', cls: out.length ? '' : 'primary',
      sub: '静静守着脐下那一点，看身体会不会回应',
      run: () => void act([{ kind: 'inspect', target: '下丹田' }]),
    })
  }
  out.push({ key: 'sense', label: '静静感受一会儿', sub: '看看身体现在怎样', cls: out.length ? '' : 'primary', run: () => void act([{ kind: 'sense' }]) })
  return out.slice(0, 2)
})

const gLine = computed(() => guideLine(cult.view))

// —— 犹豫卡（模态置顶）：内部分身的请示呈现为玩家自己的踌躇；
// 出现即要求表态，不挤主屏、不随历史滚走。按钮映射 resume/stop 意图不变。 ——
const req = computed(() => cult.view?.requests[0] ?? null)
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
  toast('念头收到了。它会影响你摸索的方向，不会直接改动身内的运转。')
  void guide('guide', t)
}

// ---------------------------------------------------------------------------
// 身体记事浮层：台账 + 假说（凭据措辞）+ 功法卡
// ---------------------------------------------------------------------------
const knowledgeEmpty = computed(() => {
  const v = cult.view
  if (!v) return true
  return !v.ledger.length && !v.hypotheses.length && !v.protocols.length
})

function protoState(p: ProtoView): string {
  if (p.active && p.paused) return '已挂起 · 等你拿主意'
  if (p.active) return p.loop ? '周天自转 · 不必你操心' : '运转中'
  return p.status === 'validated' ? '已验证 · 未运转' : '草创 · 未运转'
}

// ---------------------------------------------------------------------------
// 修行进度浮层：筑基五关点阵（band 只用于画点）+ 档位白话解释
// + 当前阶段一句话
// ---------------------------------------------------------------------------
const stage = computed(() => {
  const v = cult.view
  if (!v) return { cur: '初醒', sub: '静室之中，你对此身尚感陌生' }
  if (v.active) return { cur: '周天运转', sub: `依《${v.active}》行功，异常方唤心神` }
  if (movingQi(v)) return { cur: '吐纳中', sub: '气息在身内流动，随时可止' }
  return { cur: '暗中摸索', sub: '你在暗中辨认周身可承气之处' }
})

// ---------------------------------------------------------------------------
// 筑基仪式浮层（世界语感保留处）
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

    <!-- 感知条：单行最新感知，点开「心声」浮层看全史 -->
    <button class="cu-sensebar" :class="{ sensed: movingQi(cult.view) }" @click="voiceOpen = true">
      <span class="sb-k ui">感觉</span>
      <span class="sb-t" :key="latest?.key ?? 'quiet'">{{ latest?.text ?? quietLine }}</span>
      <span class="sb-more ui">心声 ›</span>
    </button>

    <!-- 行动区：引导语 + 心念卡≤2 + 传念行 -->
    <section class="cu-actions">
      <div class="cu-guideline ui">{{ gLine }}</div>
      <div class="cu-choices" :class="{ duo: choices.length === 2 }" v-if="choices.length">
        <button
          v-for="c in choices" :key="c.key" class="choice" :class="c.cls"
          :disabled="cult.busy" @click="c.run"
        >{{ c.label }}<small v-if="c.sub">{{ c.sub }}</small></button>
      </div>
      <div class="cu-intent">
        <input v-model="thought" placeholder="给身体指个方向，如：先求稳，别贪快" @keydown.enter="sendThought" />
        <button :disabled="cult.busy" @click="sendThought">传念</button>
      </div>
    </section>

    <!-- 底部行：浮层入口 + 忙碌态指示 -->
    <div class="cu-bottomrow ui">
      <button class="entry" @click="knowOpen = true">身体记事</button>
      <button class="entry" @click="progressOpen = true">修行进度</button>
      <span class="busy" :class="{ on: cult.busy }">{{ cult.busy ? '气息调运中……' : '' }}</span>
    </div>

    <!-- 犹豫卡：模态置顶强提示，出现即要求表态 -->
    <div class="cu-reqmodal" v-if="req">
      <div class="req-card">
        <div class="rt ui">需要你拿个主意</div>
        <div class="rr">{{ reqTitle(req.kind) }}</div>
        <div class="rx">{{ vernacular(req.text) }}</div>
        <div class="btns">
          <button class="go" :disabled="cult.busy" @click="approve">继续前行</button>
          <button class="halt" :disabled="cult.busy" @click="halt">就此止息</button>
        </div>
      </div>
    </div>

    <!-- 心声浮层：感知流全史 -->
    <CultSheet :open="voiceOpen" title="心声" @close="voiceOpen = false">
      <div
        ref="streamEl" class="cu-stream open"
        @scroll="onStreamScroll"
      >
        <div v-if="!chron.length" class="ln">闭上眼，四周寂静。你还不知世间有无「灵气」，也不知此身何处可承它。</div>
        <div v-for="l in chron" :key="l.key" class="ln" :class="lineCls(l.kind)">{{ l.text }}</div>
      </div>
    </CultSheet>

    <!-- 身体记事浮层：台账 + 假说 + 功法卡 -->
    <CultSheet :open="knowOpen" title="身体记事" @close="knowOpen = false">
      <div class="cu-know-empty" v-if="knowledgeEmpty">还没有记下什么。等你摸索出足够稳定的东西，这里自会添枝加叶。</div>
      <div class="cu-knowledge" v-else>
        <div class="kn" v-for="(l, i) in cult.view.ledger.slice(0, 12)" :key="'k' + i">{{ vernacular(l) }}</div>
        <div class="kn hyp" v-for="(h, i) in cult.view.hypotheses" :key="'h' + i">
          <b>{{ vernacular(h.text) }}</b>　{{ h.status }}——{{ vernacular(h.evidence) }}
        </div>
        <div class="cu-art" v-for="p in cult.view.protocols" :key="p.name">
          <b>《{{ p.name }}》</b>
          <p>{{ vernacular(p.path) }}</p>
          <p class="meta">{{ protoState(p) }}<template v-if="p.guards.length"> · 护卫：{{ p.guards.join('、') }}</template></p>
        </div>
      </div>
    </CultSheet>

    <!-- 修行进度浮层：筑基五关点阵 + 档位白话解释 + 当前阶段 -->
    <CultSheet :open="progressOpen" title="修行进度" @close="progressOpen = false">
      <div class="cu-stage ui">当前：<b>{{ stage.cur }}</b>　{{ stage.sub }}</div>
      <div class="cu-five">
        <div class="frow" :class="{ g: f.band >= 3 }" v-for="f in cult.view.foundation" :key="f.key">
          <span class="fk">{{ f.key }}</span>
          <span class="dots"><i v-for="i in 5" :key="i" :class="{ f: i <= f.band }"></i></span>
          <span class="word">{{ f.word }}</span>
        </div>
        <div class="fgloss ui" v-for="f in cult.view.foundation" :key="'g' + f.key">{{ f.key }}——{{ FIVE_GLOSS[f.key] ?? '' }}</div>
      </div>
    </CultSheet>

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
