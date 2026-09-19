<script setup lang="ts">
// src/cult/CultHome.vue —— 屏一 · 洞府：环境五行 chips、分身运转图、
// 感知流（回执行内嵌其中，新消息置顶）、分身自述、四动作（感/纳/引/止）。
import { computed, ref } from 'vue'
import { cult, act } from './state'
import type { PlayerIntent } from './api'

const ELEMENT_COLORS: Record<string, string> = {
  木: 'var(--w-wood)', 火: 'var(--w-fire)', 土: 'var(--w-earth)',
  金: 'var(--w-metal)', 水: 'var(--w-water)',
}

const view = () => cult.view!

const moving = computed(() => {
  const v = cult.view
  if (!v) return false
  return v.points.some((p) => p.flow !== '停滞')
})

// 感知流展示上限（新消息置顶——后端已按新→旧给出）。
const shownLines = computed(() => (cult.view?.sense_stream ?? []).slice(0, 14))

// —— 四动作 ——
const sheet = ref<'' | 'absorb' | 'guide'>('')
const target = ref('')
const fromPt = ref('')
const toPt = ref('')
const level = ref('small')
const LEVELS = [
  { v: 'trace', w: '微微' },
  { v: 'small', w: '少量' },
  { v: 'medium', w: '中等' },
  { v: 'large', w: '大量' },
]

function openSheet(kind: 'absorb' | 'guide') {
  const pts = view().points.map((p) => p.name)
  target.value = pts.includes('青关') ? '青关' : pts[0] ?? ''
  fromPt.value = target.value
  toPt.value = pts.includes('下丹田') ? '下丹田' : pts[pts.length - 1] ?? ''
  level.value = 'small'
  sheet.value = kind
}

function submitSheet() {
  let intents: PlayerIntent[]
  if (sheet.value === 'absorb') {
    intents = [{ kind: 'absorb', target: target.value, level: level.value }]
  } else {
    intents = [{ kind: 'guide', from: fromPt.value, to: toPt.value, level: level.value }]
  }
  sheet.value = ''
  void act(intents)
}

function doSense() { void act([{ kind: 'sense' }]) }
function doStop() { void act([{ kind: 'stop' }]) }
</script>

<template>
  <template v-if="cult.view">
    <!-- 环境五行（定性 chips） -->
    <div class="c-env">
      <div v-for="e in cult.view.env" :key="e.element" class="e">
        <span class="d" :style="{ background: ELEMENT_COLORS[e.element] ?? 'var(--mist)' }"></span>
        <div class="w">{{ e.element }} {{ e.qi }}</div>
      </div>
    </div>

    <!-- 分身运转图 -->
    <div class="c-figure">
      <svg viewBox="0 0 300 150" aria-hidden="true">
        <g class="c-spin">
          <circle cx="150" cy="75" r="60" fill="none" stroke="rgba(127,184,164,.35)" stroke-width="1" stroke-dasharray="2 7"/>
        </g>
        <g class="c-spin2">
          <circle cx="150" cy="75" r="46" fill="none" stroke="rgba(201,164,84,.30)" stroke-width="1" stroke-dasharray="14 9"/>
        </g>
        <circle cx="150" cy="75" r="35" fill="rgba(127,184,164,.05)" stroke="rgba(127,184,164,.45)" stroke-width="1"/>
      </svg>
      <div class="qi">炁</div>
      <div class="st"><b>{{ moving ? '吐纳中' : '入定' }}</b> · {{ cult.view.realm }}</div>
    </div>

    <!-- 感知流（定性文字滚动；回执行内嵌其中） -->
    <div class="c-card c-sense">
      <div class="t">感知</div>
      <div
        v-for="(l, i) in shownLines" :key="`${l.tick}-${i}`"
        class="ln" :class="{ hl: l.kind === 'event' || l.kind === 'voice', rc: l.kind === 'receipt' }"
      >
        <em>T{{ l.tick }}</em><span>{{ l.text }}</span>
      </div>
      <div v-if="!shownLines.length" class="ln"><span>静室无声，万籁俱寂。</span></div>
    </div>

    <!-- 分身自述（最近 cognition） -->
    <div class="c-voice">
      <div class="vt">分身自述</div>
      {{ cult.view.self_report }}
    </div>

    <!-- 意图面板（纳/引） -->
    <div class="c-sheet" v-if="sheet">
      <div class="row" v-if="sheet === 'absorb'">
        <span>纳气之穴</span>
        <select v-model="target">
          <option v-for="p in cult.view.points" :key="p.name" :value="p.name">{{ p.name }}（{{ p.openness }}）</option>
        </select>
      </div>
      <div class="row" v-if="sheet === 'guide'">
        <span>引气</span>
        <select v-model="fromPt">
          <option v-for="p in cult.view.points" :key="p.name" :value="p.name">{{ p.name }}</option>
        </select>
        <span>→</span>
        <select v-model="toPt">
          <option v-for="p in cult.view.points" :key="p.name" :value="p.name">{{ p.name }}</option>
        </select>
      </div>
      <div class="row">
        <span>力度</span>
        <button v-for="l in LEVELS" :key="l.v" class="c-chip" :class="{ on: level === l.v }" @click="level = l.v">{{ l.w }}</button>
      </div>
      <div class="row">
        <button class="go" @click="submitSheet">下这心意</button>
        <button class="cancel" @click="sheet = ''">作罢</button>
      </div>
    </div>

    <!-- 四动作 -->
    <div class="c-acts">
      <button class="a" :disabled="cult.busy" @click="doSense"><span class="c">感</span><span class="w">感知</span></button>
      <button class="a main" :disabled="cult.busy" @click="openSheet('absorb')"><span class="c">纳</span><span class="w">吐纳</span></button>
      <button class="a" :disabled="cult.busy" @click="openSheet('guide')"><span class="c">引</span><span class="w">引灵</span></button>
      <button class="a halt" :disabled="cult.busy" @click="doStop"><span class="c">止</span><span class="w">止息</span></button>
    </div>
  </template>
</template>
