<script setup lang="ts">
// src/cult/CultScene.vue —— 身体场景：人体剪影 + 8 穴位（固定坐标）+ 经脉线。
// 感知隔离律落点（渐进感知，specs/2026-10-18-progressive-perception-v1.md §5）：
//  · 穴位渲染按引擎 tier：无感不渲染；隐约=虚淡圈微亮化（无名）；有感=
//    实感小圆点（无字无名）；显名=得名字。已知判定全改引擎 tier——旧
//    knownPoints 启发式已退役（state.ts）。
//  · 前端虽经 body.points 持有全量真名（拓扑层），渲染严格按档：未显名
//    处任何位置不得出现真名（DOM 文本、data-name、aria-label 一体），
//    真名只走意图通道（凝神细察/自此引气的 target）。
//  · 未显名之穴以方位语（point.region）指称；浮卡对隐约/有感只见感受词
//    + 凝神细察（inspect），对显名渐全 + 自此引气（absorb）。
//  · 经脉按档：初显=淡虚线（行气流动动画照旧），熟路=实线；无迹不渲染；
//    端点未全显名不画（引擎已把 from/to 置空，按非空判断）。
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { cult, act, isNamed, TIER_NONE, TIER_GLOSS } from './state'
import type { PointView } from './api'

// 8 穴位固定坐标（抄自 docs/design/yiqi-dongfu-redesign.html 的 SVG 布点，
// viewBox 250×390，与人体剪影对位）。坐标表键用真名——只作拓扑定位，
// 不进任何玩家可见文本。
const POS: Record<string, [number, number]> = {
  神庭: [125, 52],
  青关: [62, 118],
  阳枢: [188, 118],
  中庭: [125, 162],
  下丹田: [125, 300],
  土府: [80, 284],
  金门: [186, 224],
  玄泉: [64, 224],
}

const points = computed<PointView[]>(() => cult.view?.points ?? [])

// 运转中（任一穴气流非停滞）→ 呼吸圈起振
const moving = computed(() => points.value.some((p) => p.flow !== '停滞'))

// 五行倾向 → 穴位描边色（与设计稿一致：木青玉 / 火暖 / 土鎏金 / 水青蓝 / 金霜白）
const ELEM_CLS: Record<string, string> = { 火: 'fire', 土: 'earth', 水: 'water', 金: 'metal' }
function elemCls(p: PointView): string {
  return ELEM_CLS[p.nature[0] ?? ''] ?? ''
}

// 节点字：下丹田取「丹」，其余取首字
function nodeChar(name: string): string {
  return name === '下丹田' ? '丹' : name.slice(0, 1)
}

interface NodeItem {
  key: string    // 渲染键（下标）：与 view.points / body.points 对齐
  target: string // 意图通道用的真名（inspect/absorb 的 target）——绝不渲染
  label: string  // 节点字：仅显名档非空
  title: string  // 浮卡标题：显名=真名，未显名=方位语
  x: number
  y: number
  p: PointView
  named: boolean
}

// 穴位节点：按档渲染。无感不渲染（渐进感知 §2）；显名判定用引擎 tier
// （state.isNamed），position 从 body.points 下标取（拓扑层）。
const nodes = computed<NodeItem[]>(() => {
  const v = cult.view
  if (!v) return []
  const out: NodeItem[] = []
  v.points.forEach((p, i) => {
    if (p.tier === TIER_NONE) return
    const real = v.body.points[i] ?? ''
    const pos = POS[real]
    if (!pos) return
    const named = isNamed(p)
    out.push({
      key: String(i),
      target: real,
      label: named ? nodeChar(p.name) : '',
      title: named ? p.name : (p.region || '某处'),
      x: pos[0], y: pos[1], p, named,
    })
  })
  return out
})

// 经脉线：按档渲染。无迹不画；端点未全显名不画（引擎已把 from/to 置空）。
const chans = computed(() => {
  const v = cult.view
  if (!v) return []
  const out: { key: string; tier: string; x1: number; y1: number; x2: number; y2: number }[] = []
  v.body.channels.forEach((c, i) => {
    if (c.tier === '无迹' || !c.from || !c.to) return
    const a = POS[c.from]
    const b = POS[c.to]
    if (!a || !b) return
    out.push({ key: String(i), tier: c.tier, x1: a[0], y1: a[1], x2: b[0], y2: b[1] })
  })
  return out
})

// —— 一次性流动动画 ——
// 只对「已画出（初显/熟路且两端已显名）」的经脉放行：未画线不动画
// （向未显名处画流动轨迹会泄拓扑，感知隔离）。
const flowChans = ref<Set<string>>(new Set())
const glowChans = ref<Set<string>>(new Set())
let animToken = 0
watch(
  () => cult.view?.tick,
  (t, old) => {
    if (t == null || t === old) return
    const v = cult.view
    if (!v) return
    const active = new Set(
      v.points.filter((p) => p.flow !== '停滞' && isNamed(p)).map((p) => p.name),
    )
    if (!active.size) return
    const keys = v.body.channels
      .map((c, i) => ({ c, key: String(i) }))
      .filter(({ c }) => c.tier !== '无迹' && c.from && c.to && (active.has(c.from) || active.has(c.to)))
      .map(({ key }) => key)
    if (!keys.length) return
    const token = ++animToken
    flowChans.value = new Set(keys)
    glowChans.value = new Set()
    window.setTimeout(() => {
      if (token !== animToken) return
      glowChans.value = new Set(keys)
      flowChans.value = new Set()
      window.setTimeout(() => {
        if (token === animToken) glowChans.value = new Set()
      }, 2800)
    }, 1500)
  },
)

// —— 点穴详情浮卡（按档：隐约/有感=感受词+凝神细察；显名=渐全+自此引气）——
const picked = ref('')
const current = computed(() => nodes.value.find((n) => n.key === picked.value) ?? null)
const currentFeelings = computed(() =>
  (current.value?.p.feelings ?? []).filter((f) => f && f !== '无明显感觉'),
)
function tap(n: NodeItem) {
  picked.value = picked.value === n.key ? '' : n.key
}

// 浮卡动作（复用既有 act 机制与忙碌禁用态）：
//  · 凝神细察 = inspect（target 走真名——真名只在意图通道）；
//  · 自此引气 = absorb level small（任意已显名之穴自由引气）。
function inspectCurrent() {
  if (!current.value || cult.busy) return
  void act([{ kind: 'inspect', target: current.value.target }])
}
function absorbCurrent() {
  if (!current.value || cult.busy) return
  void act([{ kind: 'absorb', target: current.value.target, level: 'small' }])
}

// —— 弹性高度等比缩放（2026-10-18 改版）：场景区 flex:1 占满剩余高度，
// 身体画布固定 250×390 坐标系，整体按容器等比 transform:scale 适配；
// 点位/经脉/浮卡机制一律不动。 ——
const sceneEl = ref<HTMLElement | null>(null)
const scale = ref(1)
let ro: ResizeObserver | null = null
function fit(): void {
  const el = sceneEl.value
  if (!el || !el.clientWidth || !el.clientHeight) return
  const s = Math.min(el.clientWidth / 250, el.clientHeight / 390)
  scale.value = Math.max(0.5, Math.min(s, 1.6))
}
onMounted(() => {
  fit()
  if (typeof ResizeObserver === 'undefined') return
  ro = new ResizeObserver(fit)
  if (sceneEl.value) ro.observe(sceneEl.value)
})
onBeforeUnmount(() => {
  ro?.disconnect()
  ro = null
})
</script>

<template>
  <section ref="sceneEl" class="cu-scene" :class="{ sensed: moving }">
    <div class="mist" aria-hidden="true">
      <i style="left: 16%; top: 22%"></i>
      <i class="m2" style="right: 15%; top: 30%"></i>
      <i class="m3" style="left: 28%; bottom: 20%"></i>
      <i style="right: 26%; bottom: 30%"></i>
    </div>
    <div class="cu-body" :style="{ transform: `scale(${scale})` }">
      <div class="sil" aria-hidden="true"></div>
      <div class="breath" :class="{ on: moving }" aria-hidden="true"></div>
      <svg class="chan" viewBox="0 0 250 390" aria-hidden="true">
        <line
          v-for="c in chans" :key="c.key"
          :x1="c.x1" :y1="c.y1" :x2="c.x2" :y2="c.y2"
          :class="[c.tier === '熟路' ? 'familiar' : 'faintline', { flow: flowChans.has(c.key), afterglow: glowChans.has(c.key) }]"
        />
      </svg>
      <button
        v-for="n in nodes" :key="n.key"
        class="node"
        :class="n.named
          ? ['known', elemCls(n.p), { picked: picked === n.key }]
          : (n.p.tier === '有感' ? ['felt', { picked: picked === n.key }] : ['faint', { picked: picked === n.key }])"
        :style="{ left: n.x + 'px', top: n.y + 'px' }"
        :data-name="n.named ? n.title : undefined"
        :aria-label="n.title"
        @click="tap(n)"
      >{{ n.label }}</button>

      <div class="nodedetail" :class="{ show: current }" v-if="current">
        <button class="x" @click="picked = ''">✕</button>
        <b>{{ current.title }}</b>
        <!-- 显名档：浮卡渐全（引擎已按档门控，前端照渲染） -->
        <span v-if="current.named">
          <template v-if="currentFeelings.length">所感：{{ currentFeelings.join('、') }}。</template>
          <template v-else>所感：无甚可辨。</template>
          气机{{ current.p.flow }}，开度{{ current.p.openness }}，{{ current.p.stability }}，{{ current.p.damage }}。
          <template v-if="current.p.nature.length">气带{{ current.p.nature.join('、') }}性。</template>
          ——所见皆是感受，其余都藏在这具身体里。
        </span>
        <!-- 未显名档（隐约/有感）：只见感受词与方位语，不落名不落数 -->
        <span v-else>
          <i class="nd-gloss ui">{{ TIER_GLOSS[current.p.tier] ?? '' }}</i>
          <template v-if="currentFeelings.length">所感：{{ currentFeelings.join('、') }}。</template>
          <template v-else>所感：似有似无，一时说不上来。</template>
          那处的名字，还要靠气一遍遍走过才能认出。
        </span>
        <div class="nd-btns">
          <button
            v-if="!current.named"
            class="nd-act" :disabled="cult.busy" @click="inspectCurrent"
          >凝神细察</button>
          <button
            v-else
            class="nd-act" :disabled="cult.busy" @click="absorbCurrent"
          >自此引气</button>
        </div>
      </div>
    </div>
    <div class="cu-hint">{{ moving ? '气息在身内流动' : '静下心来，雾气深处隐有浮光' }}</div>
  </section>
</template>
