<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  api, essenceColor, essenceName,
  type StateView, type Vec2,
} from './api'

const view = ref<StateView | null>(null)
const busy = ref(false)
const error = ref('')
const guideKind = ref('concern')
const guideText = ref('')

const W = 12
const H = 12
const SPAWN: Vec2 = { x: 1, y: 10 }

const exploredSet = computed(() => {
  const s = new Set<string>()
  for (const p of view.value?.explored ?? []) s.add(`${p.x},${p.y}`)
  return s
})

const visibleSet = computed(() => {
  const s = new Set<string>()
  for (const p of view.value?.observation?.visible ?? []) s.add(`${p.x},${p.y}`)
  return s
})

const entityAt = (x: number, y: number) =>
  view.value?.observation?.entities.find(e => e.pos.x === x && e.pos.y === y)

const selfAt = (x: number, y: number) =>
  view.value?.observation?.self.pos.x === x && view.value!.observation!.self.pos.y === y

// v0.1 地形渲染是客户端先验（旧地图）；正式版应由 Observation 供给。
const terrainDanger = (x: number, y: number) => x >= 3 && x <= 7 && y >= 3 && y <= 7
const terrainEssence = (x: number, y: number): number => {
  if (terrainDanger(x, y)) return 1 // 火
  if (y <= 1) return 3 // 金
  if (x === 0) return 4 // 水
  return 2 // 土
}

const cellStyle = (x: number, y: number) => {
  const key = `${x},${y}`
  const explored = exploredSet.value.has(key)
  const visible = visibleSet.value.has(key)
  const base = explored ? essenceColor(terrainEssence(x, y)) : '#222'
  return {
    background: base,
    opacity: explored ? (visible ? 1 : 0.45) : 1,
    outline: terrainDanger(x, y) ? '1px solid rgba(255,80,60,.4)' : 'none',
    outlineOffset: '-2px',
  }
}

const cognition = computed(() => {
  const v = view.value
  if (!v?.observation) return []
  const s = v.observation.self
  const rows: [string, string][] = [
    ['Goal', s.carrying.length ? '携带 Fragment 返回出生点' : '前往危险区取得 Fragment'],
    ['Plan', '直线推进，功率 0.4（阴偏稳）'],
    ['Known', v.observation.entities.length
      ? v.observation.entities.map(e => `${e.kind === 'fragment' ? '结晶' : e.owner === 'unknown' ? '未知体' : e.kind}·${essenceName(e.essence)} @${e.pos.x},${e.pos.y}`).join('；')
      : '感知范围内无其他实体'],
    ['Risk', `热 ${s.heat}/25 · 气 ${s.battery}/${s.max_battery}`],
    ['Suspicions', v.observation.entities.some(e => e.owner === 'unknown') ? 'Unknown Agent 在附近（火迹明显，虚张声势？）' : '无'],
  ]
  return rows
})

async function refresh() {
  if (!view.value) return
  view.value = await api<StateView>(`/api/match/${view.value.id}/state`)
}

async function newMatch() {
  busy.value = true
  error.value = ''
  try {
    view.value = await api<StateView>('/api/match', {})
  } catch (e) {
    error.value = String(e)
  } finally {
    busy.value = false
  }
}

async function step(auto: boolean) {
  if (!view.value) return
  busy.value = true
  error.value = ''
  try {
    view.value = await api<StateView>(
      `/api/match/${view.value.id}/${auto ? 'auto' : 'tick'}`, auto ? {} : { intents: [] })
  } catch (e) {
    error.value = String(e)
  } finally {
    busy.value = false
  }
}

async function runToEnd() {
  if (!view.value) return
  busy.value = true
  error.value = ''
  try {
    for (let i = 0; i < 60; i++) {
      view.value = await api<StateView>(`/api/match/${view.value.id}/auto`, {})
      if (view.value.status !== 'running') break
    }
  } catch (e) {
    error.value = String(e)
  } finally {
    busy.value = false
  }
}

async function sendGuidance() {
  if (!view.value || !guideText.value.trim()) return
  busy.value = true
  error.value = ''
  try {
    view.value = await api<StateView>(`/api/match/${view.value.id}/guidance`, {
      kind: guideKind.value,
      text: guideText.value.trim(),
    })
    guideText.value = ''
  } catch (e) {
    error.value = String(e)
  } finally {
    busy.value = false
  }
}

onMounted(newMatch)
</script>

<template>
  <div class="wrap">
    <header>
      <h1>agent_battle <small>scenario/mvp-0.1</small></h1>
      <div class="bar" v-if="view">
        <span>Turn {{ view.turn }}/{{ view.turn_limit }}</span>
        <span :class="{ warn: (view.observation?.self.heat ?? 0) > 18 }">
          热 {{ view.observation?.self.heat ?? '—' }}
        </span>
        <span>形 {{ view.observation?.self.integrity ?? '—' }}</span>
        <span>气 {{ view.observation?.self.battery ?? '—' }}</span>
        <span>状态 {{ view.status }}</span>
        <span class="seed">seed {{ view.seed }}</span>
      </div>
    </header>

    <div class="main">
      <div class="map-wrap">
        <div class="grid">
          <div v-for="y in H" :key="y" class="row">
            <div
              v-for="x in W" :key="x"
              class="cell"
              :style="cellStyle(x - 1, y - 1)"
            >
              <span v-if="selfAt(x - 1, y - 1)">🤖</span>
              <span v-else-if="entityAt(x - 1, y - 1)?.kind === 'fragment'">💎</span>
              <span v-else-if="entityAt(x - 1, y - 1)">❓</span>
              <span v-else-if="x - 1 === SPAWN.x && y - 1 === SPAWN.y" class="spawn">⌂</span>
            </div>
          </div>
        </div>
        <div class="btns">
          <button @click="newMatch" :disabled="busy">新对局</button>
          <button @click="step(true)" :disabled="busy || view?.status !== 'running'">自动一步</button>
          <button @click="runToEnd" :disabled="busy || view?.status !== 'running'">自动到结束</button>
        </div>
        <p v-if="error" class="err">{{ error }}</p>
      </div>

      <aside class="panel">
        <h2>Agent 认知摘要 <small>v0.1 派生</small></h2>
        <dl v-if="cognition.length">
          <template v-for="[k, v] in cognition" :key="k">
            <dt>{{ k }}</dt>
            <dd>{{ v }}</dd>
          </template>
        </dl>
        <p v-else>—</p>

        <h2>最近事件</h2>
        <ul class="events">
          <li v-for="(e, i) in (view?.events ?? []).slice(-8).reverse()" :key="i">
            <em>T{{ e.tick }}</em> {{ e.text }}
          </li>
        </ul>

        <h2>Guidance <small>给偏好，不给动作</small></h2>
        <div class="guide">
          <select v-model="guideKind">
            <option value="goal">Goal</option>
            <option value="preference">Preference</option>
            <option value="concern">Concern</option>
            <option value="information">Information</option>
          </select>
          <input v-model="guideText" placeholder="例如：先确认它的储气再决定是否接战" @keyup.enter="sendGuidance" />
          <button @click="sendGuidance" :disabled="busy || !view">发送</button>
        </div>
        <ul class="events">
          <li v-for="(g, i) in (view?.guidance ?? []).slice().reverse()" :key="'g' + i">
            <em>T{{ g.turn }} {{ g.kind }}</em> {{ g.text }}
          </li>
        </ul>
      </aside>
    </div>
  </div>
</template>

<style>
* { box-sizing: border-box; }
body {
  margin: 0;
  background: #14161a;
  color: #d8dce2;
  font-family: system-ui, -apple-system, 'PingFang SC', sans-serif;
}
.wrap { max-width: 1020px; margin: 0 auto; padding: 16px; }
header h1 { font-size: 18px; margin: 0 0 8px; }
header small { color: #7a8290; font-weight: normal; }
.bar { display: flex; gap: 14px; font-size: 13px; color: #aab2bd; flex-wrap: wrap; }
.bar .warn { color: #ff6b57; }
.bar .seed { margin-left: auto; color: #59616d; }
.main { display: flex; gap: 20px; margin-top: 14px; flex-wrap: wrap; }
.map-wrap { flex: 0 0 auto; }
.grid { display: inline-block; border: 1px solid #2a2f37; }
.row { display: flex; }
.cell {
  width: 38px; height: 38px;
  display: flex; align-items: center; justify-content: center;
  font-size: 18px; position: relative;
}
.cell .spawn { color: #8fd; font-size: 12px; }
.btns { display: flex; gap: 8px; margin-top: 10px; flex-wrap: wrap; }
button {
  background: #2a3040; color: #d8dce2; border: 1px solid #3a4150;
  border-radius: 6px; padding: 6px 12px; cursor: pointer; font-size: 13px;
}
button:hover:not(:disabled) { background: #35405a; }
button:disabled { opacity: 0.4; cursor: default; }
.panel { flex: 1 1 320px; min-width: 300px; }
.panel h2 { font-size: 14px; margin: 18px 0 6px; color: #9fd0ff; }
.panel h2 small { color: #59616d; font-weight: normal; }
dl { margin: 0; font-size: 13px; }
dt { color: #7a8290; margin-top: 8px; }
dd { margin: 2px 0 0; line-height: 1.5; }
.events { list-style: none; margin: 0; padding: 0; font-size: 12.5px; color: #aab2bd; }
.events li { padding: 3px 0; border-bottom: 1px dashed #242933; }
.events em { color: #59616d; font-style: normal; margin-right: 6px; }
.guide { display: flex; gap: 6px; }
.guide select, .guide input {
  background: #1d2129; color: #d8dce2; border: 1px solid #3a4150;
  border-radius: 6px; padding: 6px 8px; font-size: 13px;
}
.guide input { flex: 1; }
.err { color: #ff6b57; font-size: 13px; }
</style>
