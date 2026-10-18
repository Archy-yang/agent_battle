<script setup lang="ts">
// src/cult/CultTour.vue —— 新手引导：首次入定后 4 步高亮步进
// （身体图 → 感知条 → 心念卡 → 传念），每步一句话，可跳过。
// 完成/跳过态存 localStorage（cult_tour_done）；私有浏览态下
// 读写失败即静默降级（不再弹、不报错）。顶栏 ⟲ 可随时重看，
// 重看待玩法总述开场。
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ close: [] }>()

const DONE_KEY = 'cult_tour_done'

// 两三行玩法总述（重看时先行呈现；首入门也作开场）
const INTRO =
  '这是一门向内养的功夫：静静感受身体，把天地间的气息引进来，' +
  '再顺着功法的路数让它周流不息。身体会在一呼一吸里慢慢圆融，' +
  '路要自己一步一步摸出来。'

interface Step { sel: string; text: string }
const STEPS: Step[] = [
  { sel: '', text: INTRO },
  { sel: '.cu-scene', text: '这是你的身体。你与它还不熟——多感受一会儿，它就多显露一分。' },
  { sel: '.cu-sensebar', text: '身体的感觉会出现在这里；点一下，能看到完整的心声。' },
  { sel: '.cu-choices', text: '能做的事都在这些心念卡上，挑一张照做就好。' },
  { sel: '.cu-intent', text: '想给身体指个方向，就在这里说一句。' },
]

const step = ref(0)
const box = ref<{ x: number; y: number; w: number; h: number } | null>(null)
const tip = ref<{ left: number; top: number; above: boolean; width: number } | null>(null)
const cur = computed(() => STEPS[step.value])

function measure(): void {
  box.value = null
  tip.value = null
  const sel = cur.value.sel
  if (!sel) return // 总述步：卡片居中，不高亮
  const el = document.querySelector(sel) as HTMLElement | null
  if (!el) return
  const r = el.getBoundingClientRect()
  const rootEl = el.closest('.cult-root') as HTMLElement | null
  const rr = rootEl?.getBoundingClientRect()
  const ox = rr?.left ?? 0
  const oy = rr?.top ?? 0
  const rootW = rr?.width ?? window.innerWidth
  const rootH = rr?.height ?? window.innerHeight
  box.value = { x: r.left - ox, y: r.top - oy, w: r.width, h: r.height }
  const width = Math.min(330, rootW - 24)
  const left = Math.min(Math.max(12, box.value.x + box.value.w / 2 - width / 2), rootW - width - 12)
  const below = box.value.y + box.value.h + 14
  if (below + 140 < rootH) {
    tip.value = { left, top: below, above: false, width }
  } else {
    tip.value = { left, top: Math.max(12, box.value.y - 14), above: true, width }
  }
}

watch(
  () => props.open,
  (o) => {
    if (!o) {
      window.removeEventListener('resize', measure)
      return
    }
    step.value = 0
    void nextTick(() => window.setTimeout(measure, 80))
    window.addEventListener('resize', measure)
  },
)
watch(step, () => { void nextTick(measure) })
onBeforeUnmount(() => window.removeEventListener('resize', measure))

function finish(): void {
  try { localStorage.setItem(DONE_KEY, '1') } catch { /* 私有浏览态：静默降级 */ }
  window.removeEventListener('resize', measure)
  emit('close')
}
function nextStep(): void {
  if (step.value >= STEPS.length - 1) finish()
  else step.value++
}
</script>

<template>
  <div class="cu-tour" v-if="open">
    <div class="tmask"></div>
    <div
      v-if="box" class="tspot"
      :style="{ left: box.x + 'px', top: box.y + 'px', width: box.w + 'px', height: box.h + 'px' }"
      aria-hidden="true"
    ></div>
    <div
      class="tcard ui" :class="{ center: !tip, above: tip?.above }"
      :style="tip ? { left: tip.left + 'px', top: tip.top + 'px', width: tip.width + 'px' } : undefined"
    >
      <p>{{ cur.text }}</p>
      <div class="trow">
        <button class="skip" @click="finish">跳过引导</button>
        <span class="dots" aria-hidden="true">
          <i v-for="(s, i) in STEPS" :key="i" :class="{ on: i === step }"></i>
        </span>
        <button class="next" @click="nextStep">{{ step === STEPS.length - 1 ? '开始修行' : '下一步' }}</button>
      </div>
    </div>
  </div>
</template>
