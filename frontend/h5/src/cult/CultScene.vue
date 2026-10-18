<script setup lang="ts">
// src/cult/CultScene.vue —— 身体场景：人体剪影 + 8 穴位（固定坐标）+ 经脉线。
// 感知隔离律落点：
//  · 未知穴只渲染模糊小波动（虚线淡圈），无名字、无「?」、不透露未知总数；
//    已知判定用 state.knownPoints 的前端启发式（feelings 非空或穴名出现过在感知流）。
//  · 经脉线零静态：平时完全不可见（opacity:0）。仅当 tick 推进后，
//    对「已知且 flow 非停滞」穴位所邻接的经脉（view.body.channels）
//    做一次性流动动画（~1.5s），随后余辉渐隐。
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { cult, knownPoints } from './state'
import type { PointView } from './api'

// 8 穴位固定坐标（抄自 docs/design/yiqi-dongfu-redesign.html 的 SVG 布点，
// viewBox 250×390，与人体剪影对位）
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
const known = computed<Set<string>>(() =>
  cult.view ? knownPoints(cult.view) : new Set<string>(),
)

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

interface NodeItem { name: string; x: number; y: number; p: PointView }
const nodes = computed<NodeItem[]>(() =>
  points.value
    .filter((p) => POS[p.name])
    .map((p) => ({ name: p.name, x: POS[p.name][0], y: POS[p.name][1], p })),
)

// 经脉线：两端坐标都已知的才画；class 由动画状态驱动（零静态）
const chans = computed(() => {
  const v = cult.view
  if (!v) return []
  return v.body.channels
    .filter((c) => POS[c.from] && POS[c.to])
    .map((c) => ({ name: c.name, x1: POS[c.from][0], y1: POS[c.from][1], x2: POS[c.to][0], y2: POS[c.to][1] }))
})

// —— 一次性流动动画 ——
// 只对「两端皆已知穴」的经脉放行：向未知穴画线会泄顶拓扑（感知隔离）。
const flowChans = ref<Set<string>>(new Set())
const glowChans = ref<Set<string>>(new Set())
let animToken = 0
watch(
  () => cult.view?.tick,
  (t, old) => {
    if (t == null || t === old) return
    const v = cult.view
    if (!v) return
    const kn = known.value
    const active = new Set(
      v.points.filter((p) => p.flow !== '停滞' && kn.has(p.name)).map((p) => p.name),
    )
    if (!active.size) return
    const names = v.body.channels
      .filter((c) => (active.has(c.from) || active.has(c.to)) && kn.has(c.from) && kn.has(c.to))
      .map((c) => c.name)
    if (!names.length) return
    const token = ++animToken
    flowChans.value = new Set(names)
    glowChans.value = new Set()
    window.setTimeout(() => {
      if (token !== animToken) return
      glowChans.value = new Set(names)
      flowChans.value = new Set()
      window.setTimeout(() => {
        if (token === animToken) glowChans.value = new Set()
      }, 2800)
    }, 1500)
  },
)

// —— 点穴详情浮卡（仅已知穴）——
const picked = ref('')
const current = computed(() => points.value.find((p) => p.name === picked.value))
const currentFeelings = computed(() =>
  (current.value?.feelings ?? []).filter((f) => f && f !== '无明显感觉'),
)
function tap(n: NodeItem) {
  if (!known.value.has(n.name)) return // 未知穴：不响应，不提示（保密数量）
  picked.value = picked.value === n.name ? '' : n.name
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
          v-for="c in chans" :key="c.name"
          :x1="c.x1" :y1="c.y1" :x2="c.x2" :y2="c.y2"
          :class="{ flow: flowChans.has(c.name), afterglow: glowChans.has(c.name) }"
        />
      </svg>
      <button
        v-for="n in nodes" :key="n.name"
        class="node"
        :class="known.has(n.name) ? ['known', elemCls(n.p), { picked: picked === n.name }] : 'faint'"
        :style="{ left: n.x + 'px', top: n.y + 'px' }"
        :data-name="known.has(n.name) ? n.name : undefined"
        @click="tap(n)"
      >{{ known.has(n.name) ? nodeChar(n.name) : '' }}</button>

      <div class="nodedetail" :class="{ show: current }" v-if="current">
        <button class="x" @click="picked = ''">✕</button>
        <b>{{ current.name }}</b>
        <span>
          <template v-if="currentFeelings.length">所感：{{ currentFeelings.join('、') }}。</template>
          <template v-else>所感：无甚可辨。</template>
          气机{{ current.flow }}，开度{{ current.openness }}，{{ current.stability }}，{{ current.damage }}。
          <template v-if="current.nature.length">气带{{ current.nature.join('、') }}性。</template>
          ——所见皆是感受，其余都藏在这具身体里。
        </span>
      </div>
    </div>
    <div class="cu-hint">{{ moving ? '气息在身内流动' : '静下心来，雾气深处隐有浮光' }}</div>
  </section>
</template>
