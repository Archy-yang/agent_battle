<script setup lang="ts">
// v0.7 回放页（world_genesis §7 / §7.1）：
//   GET  /api/match/{id}/recording → Recording（frames 逐 tick 意图 + 事件）
//   POST /api/replay {recording, until_tick} → 该 tick 的 StateView（服务端
//   从头逐帧重演并校验事件流；篡改/损坏 → 500 "replay mismatch"）。
// 回放视图只用本组件局部 ref，不读写 state.ts 的全局 view —— 不污染
// 正在对局的全局状态。地图绘制与 MapView.vue 同源（复制自其 draw()，
// 见下方标注），v0.8 抽共享渲染层。
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { api, essenceColor, essenceColorByName, type Hex, type Recording, type StateView } from '../api'
import { view as globalView, FOG } from '../state'
import {
  fogOf, hexCorners, hexCenter, hexKey, visibleHexes, worldToScreen, zoomAt,
  type Camera,
} from '../hex'

// ---- 常量（与 MapView 一致） ----
const HEX_SIZE = 28
const ACCENT = '#d4a94e'
const FIRE = '#e05545'
const WATER = '#4a8fd4'
const SELF_RING = '#9fd0ff'
const HEX_BORDER = 'rgba(15, 18, 22, 0.55)'
const PLAY_INTERVAL_MS = 600

// 战斗类事件（与 BattleView 的 COMBAT_TYPES 同源）：帧事件流里高亮
const COMBAT_TYPES = new Set(['attack', 'backlash', 'spill', 'overheat', 'destroyed'])

// ---- 回放数据（全部局部，不进全局 state） ----
const matchId = ref('') // 输入框：对局 ID
const recording = ref<Recording | null>(null)
const loading = ref(false)
const loadError = ref('')

const state = ref<StateView | null>(null) // 当前帧重演出的 StateView
const replayBusy = ref(false)
const replayError = ref('') // 含「录像校验失败（replay mismatch）」等

// 帧游标：slider 1..frames.length（value = frameIdx + 1）
const frameIdx = ref(0)
const playing = ref(false)
let playTimer: ReturnType<typeof setInterval> | null = null

const frames = computed(() => recording.value?.frames ?? [])
const frameCount = computed(() => frames.value.length)
const currentFrame = computed(() => frames.value[frameIdx.value] ?? null)

// 帧事件流：当前帧的 resulting_events（战斗类高亮）
const frameEvents = computed(() => currentFrame.value?.resulting_events ?? [])
const isCombatEvent = (e: { type: string }) => COMBAT_TYPES.has(e.type)

// 帧意图概览（auto / 手动 + 条数），只做审计展示。
// 注意：Go 侧空 slice 序列化为 null（无意图帧），需 ?? [] 兜底。
const intentSummary = computed(() => {
  const f = currentFrame.value
  if (!f) return '—'
  return `${f.auto ? 'pilot 自动' : '手动'} · ${(f.player_intents ?? []).length} 条意图`
})

// ---- 加载 ----
async function loadRecording(id: string) {
  const trimmed = id.trim()
  if (!trimmed || loading.value) return
  stopPlayback()
  loading.value = true
  loadError.value = ''
  replayError.value = ''
  recording.value = null
  state.value = null
  frameIdx.value = 0
  try {
    recording.value = await api<Recording>(`/api/match/${encodeURIComponent(trimmed)}/recording`)
    if (!frameCount.value) loadError.value = '该对局还没有任何录像帧（先跑几步）'
  } catch (e) {
    loadError.value = errText(e)
  } finally {
    loading.value = false
  }
}

// 回放当前对局：用全局 state 的当前 match id（只读 id，不写全局状态）
function loadCurrent() {
  const v = globalView.value
  if (!v?.id) {
    loadError.value = '当前没有进行中的对局'
    return
  }
  matchId.value = v.id
  loadRecording(v.id)
}

// ---- 重演：帧游标变化 → POST /api/replay（带缓存，拖回已看过的帧零请求） ----
const stateCache = new Map<number, StateView>()
let fetchSeq = 0

async function fetchFrameState(tick: number) {
  const rec = recording.value
  if (!rec) return
  const cached = stateCache.get(tick)
  if (cached) {
    state.value = cached
    replayError.value = ''
    return
  }
  const seq = ++fetchSeq
  replayBusy.value = true
  try {
    const sv = await api<StateView>('/api/replay', { ...rec, until_tick: tick })
    if (seq !== fetchSeq) return // 过期响应：已被更新的游标超越，丢弃
    if (stateCache.size > 60) stateCache.clear() // 简单容量上限
    stateCache.set(tick, sv)
    state.value = sv
    replayError.value = ''
  } catch (e) {
    if (seq !== fetchSeq) return
    const msg = errText(e)
    replayError.value = msg.includes('replay mismatch')
      ? '录像校验失败（replay mismatch）——录像被篡改或与服务端引擎版本不一致'
      : msg
    stopPlayback()
  } finally {
    if (seq === fetchSeq) replayBusy.value = false
  }
}

watch([frameIdx, recording], () => {
  const f = currentFrame.value
  if (f) void fetchFrameState(f.tick)
})

// ---- 播放控制 ----
function startPlayback() {
  if (playing.value || !frameCount.value) return
  if (frameIdx.value >= frameCount.value - 1) frameIdx.value = 0 // 到结尾再播 → 从头
  playing.value = true
  playTimer = setInterval(() => {
    if (frameIdx.value >= frameCount.value - 1) {
      stopPlayback()
      return
    }
    frameIdx.value++
  }, PLAY_INTERVAL_MS)
}
function stopPlayback() {
  playing.value = false
  if (playTimer) {
    clearInterval(playTimer)
    playTimer = null
  }
}
const stepFrame = (d: number) => {
  stopPlayback()
  const n = frameIdx.value + d
  if (n >= 0 && n < frameCount.value) frameIdx.value = n
}
const jumpToEnd = () => {
  stopPlayback()
  if (frameCount.value) frameIdx.value = frameCount.value - 1
}
const onSliderInput = () => { if (playing.value) stopPlayback() }
onBeforeUnmount(stopPlayback)

// ================= 地图绘制（与 MapView.vue 同源，v0.8 抽共享渲染层） =================
// 以下绘制代码复制自 MapView.vue 的 draw()，数据源由全局 state 换成本组件
// 的局部 state ref；交互仅保留平移/缩放/跟随，去掉 move_to / 采集等对局动作。

const shellRef = ref<HTMLDivElement | null>(null)
const canvasRef = ref<HTMLCanvasElement | null>(null)
const cam = ref<Camera>({ cx: 0, cy: 0, zoom: 1 })
const follow = ref(true)
const vp = ref({ width: 640, height: 420 })
let dirty = true
const invalidate = () => { dirty = true }

const selfHex = computed<Hex | null>(() => {
  const o = state.value?.observation
  return o ? { q: o.self.pos.q, r: o.self.pos.r } : null
})

interface MarkerVm { q: number; r: number; glyph: string; color?: string; diamond?: boolean }
const markers = computed<MarkerVm[]>(() => {
  const out: MarkerVm[] = []
  const seen = new Set<string>()
  for (const e of state.value?.observation?.entities ?? []) {
    const k = hexKey(e.pos)
    if (seen.has(k)) continue
    seen.add(k)
    if (e.kind === 'resource') {
      out.push({ q: e.pos.q, r: e.pos.r, glyph: '', color: essenceColor(e.essence), diamond: true })
    } else {
      out.push(e.kind === 'fragment'
        ? { q: e.pos.q, r: e.pos.r, glyph: '💎' }
        : { q: e.pos.q, r: e.pos.r, glyph: '❓', color: essenceColor(e.essence) })
    }
  }
  for (const en of state.value?.enemies ?? []) {
    const k = hexKey(en.pos)
    if (seen.has(k)) continue
    seen.add(k)
    out.push({ q: en.pos.q, r: en.pos.r, glyph: '❓', color: essenceColorByName(en.essence) })
  }
  return out
})

const exploredSet = computed(() => {
  const s = new Set<string>()
  for (const p of state.value?.explored ?? []) s.add(`${p.q},${p.r}`)
  return s
})
const visibleSet = computed(() => {
  const s = new Set<string>()
  for (const p of state.value?.observation?.visible ?? []) s.add(`${p.q},${p.r}`)
  return s
})
const exploredCellMap = computed(() => {
  const m = new Map<string, { essence?: number; danger?: boolean; passable?: boolean }>()
  for (const p of state.value?.explored ?? []) m.set(`${p.q},${p.r}`, p)
  return m
})
const orderTarget = computed<Hex | null>(() => state.value?.order ?? null)

// 出生点：从第一帧重演结果的自观测记下（回放没有服务端 spawn 字段）
const spawnPos = ref<Hex | null>(null)
const spawnCell = computed<Hex | null>(() => spawnPos.value)
watch(state, (sv) => {
  if (!spawnPos.value && sv?.observation && sv.turn <= 1) {
    spawnPos.value = { q: sv.observation.self.pos.q, r: sv.observation.self.pos.r }
  }
})

function centerOnSelf() {
  const h = selfHex.value
  if (!h) return
  const p = hexCenter(h, HEX_SIZE)
  cam.value = { ...cam.value, cx: p.x, cy: p.y }
  invalidate()
}
watch(selfHex, () => { if (follow.value) centerOnSelf() })
watch(recording, () => {
  spawnPos.value = null
  stateCache.clear()
  follow.value = true
  cam.value = { cx: 0, cy: 0, zoom: 1 }
  // 渲染要等第一帧重演回来，selfHex watch 会居中
})

function hexPath(ctx: CanvasRenderingContext2D, x: number, y: number, r: number) {
  ctx.beginPath()
  const pts = hexCorners(x, y, r)
  ctx.moveTo(pts[0].x, pts[0].y)
  for (let i = 1; i < 6; i++) ctx.lineTo(pts[i].x, pts[i].y)
  ctx.closePath()
}

function draw() {
  const canvas = canvasRef.value
  const ctx = canvas?.getContext('2d')
  if (!canvas || !ctx) return
  const { width, height } = vp.value
  const c = cam.value
  const s = HEX_SIZE * c.zoom
  const r = s * 0.96

  ctx.fillStyle = FOG.unexplored
  ctx.fillRect(0, 0, width, height)
  if (s < 3) return

  const explored = exploredSet.value
  const visible = visibleSet.value
  const cells = exploredCellMap.value
  const order = orderTarget.value
  const spawn = spawnCell.value

  for (const h of visibleHexes(c, vp.value, HEX_SIZE)) {
    const fog = fogOf(h, explored, visible)
    if (fog === 'unexplored') continue
    const w = hexCenter(h, HEX_SIZE)
    const p = worldToScreen(w.x, w.y, c, vp.value)
    hexPath(ctx, p.x, p.y, r)
    if (fog === 'visible') {
      const cell = cells.get(hexKey(h))
      ctx.fillStyle = cell?.essence != null ? essenceColor(cell.essence) : FOG.visible
      ctx.fill()
      ctx.strokeStyle = HEX_BORDER
      ctx.lineWidth = 1
      ctx.stroke()
      if (cell?.danger) {
        ctx.strokeStyle = FIRE
        ctx.lineWidth = 2
        hexPath(ctx, p.x, p.y, r - 1.5)
        ctx.stroke()
        ctx.lineWidth = 1.2
        const half = r * 0.45
        for (let i = -1; i <= 1; i++) {
          ctx.beginPath()
          ctx.moveTo(p.x - half, p.y + i * half * 0.7)
          ctx.lineTo(p.x + half, p.y + i * half * 0.7 - half * 0.6)
          ctx.stroke()
        }
      }
    } else {
      ctx.globalAlpha = 0.45
      ctx.fillStyle = FOG.remembered
      ctx.fill()
      ctx.globalAlpha = 1
      ctx.strokeStyle = HEX_BORDER
      ctx.lineWidth = 1
      ctx.stroke()
    }
  }

  if (order) {
    const w = hexCenter(order, HEX_SIZE)
    const p = worldToScreen(w.x, w.y, c, vp.value)
    hexPath(ctx, p.x, p.y, r - 2)
    ctx.setLineDash([5, 3])
    ctx.strokeStyle = ACCENT
    ctx.lineWidth = 2
    ctx.stroke()
    ctx.setLineDash([])
  }

  ctx.textAlign = 'center'
  ctx.textBaseline = 'middle'
  const glyph = Math.max(9, Math.min(40, s * 1.05))
  ctx.font = `${glyph}px system-ui, sans-serif`
  const at = (h: Hex) => {
    const w = hexCenter(h, HEX_SIZE)
    return worldToScreen(w.x, w.y, c, vp.value)
  }
  if (spawn) {
    const p = at(spawn)
    ctx.font = `${glyph * 0.85}px system-ui, sans-serif`
    ctx.fillStyle = WATER
    ctx.fillText('⌂', p.x, p.y + glyph * 0.04)
    ctx.font = `${glyph}px system-ui, sans-serif`
  }
  for (const m of markers.value) {
    const p = at({ q: m.q, r: m.r })
    if (m.diamond) {
      const d = r * 0.52
      ctx.beginPath()
      ctx.moveTo(p.x, p.y - d)
      ctx.lineTo(p.x + d, p.y)
      ctx.lineTo(p.x, p.y + d)
      ctx.lineTo(p.x - d, p.y)
      ctx.closePath()
      ctx.fillStyle = m.color ?? ACCENT
      ctx.fill()
      ctx.strokeStyle = HEX_BORDER
      ctx.lineWidth = 1.5
      ctx.stroke()
      continue
    }
    if (m.color) {
      ctx.beginPath()
      ctx.arc(p.x, p.y, r * 0.62, 0, Math.PI * 2)
      ctx.strokeStyle = m.color
      ctx.lineWidth = 2
      ctx.stroke()
    }
    ctx.fillText(m.glyph, p.x, p.y + glyph * 0.06)
  }
  if (selfHex.value) {
    const p = at(selfHex.value)
    ctx.beginPath()
    ctx.arc(p.x, p.y, r * 0.66, 0, Math.PI * 2)
    ctx.strokeStyle = SELF_RING
    ctx.lineWidth = 2
    ctx.stroke()
    ctx.font = `${glyph * 1.1}px system-ui, sans-serif`
    ctx.fillText('🤖', p.x, p.y + glyph * 0.06)
  }
}

let raf = 0
function frame() {
  if (dirty) {
    dirty = false
    draw()
  }
  raf = requestAnimationFrame(frame)
}

function resize() {
  const shell = shellRef.value
  const canvas = canvasRef.value
  if (!shell || !canvas) return
  const w = shell.clientWidth
  const h = shell.clientHeight
  if (w <= 0 || h <= 0) return
  const dpr = window.devicePixelRatio || 1
  vp.value = { width: w, height: h }
  canvas.width = Math.round(w * dpr)
  canvas.height = Math.round(h * dpr)
  canvas.style.width = `${w}px`
  canvas.style.height = `${h}px`
  canvas.getContext('2d')?.setTransform(dpr, 0, 0, dpr, 0, 0)
  invalidate()
}
let ro: ResizeObserver | null = null

// 画布区在 template v-if 里（加载录像后才出现）：watch 模板 ref 挂/卸
// ResizeObserver 与滚轮监听，不能用 onMounted（此刻 shellRef 还是 null）。
watch(shellRef, (el, prev) => {
  prev?.removeEventListener('wheel', onWheel)
  ro?.disconnect()
  ro = null
  if (el) {
    ro = new ResizeObserver(resize)
    ro.observe(el)
    resize()
    canvasRef.value?.addEventListener('wheel', onWheel, { passive: false })
    if (selfHex.value) centerOnSelf()
  }
})

// 平移 / 缩放（与 MapView 一致；无拾取下单 —— 回放是只读视图）
const pointers = new Map<number, { x: number; y: number }>()
let dragMoved = false
let downX = 0
let downY = 0
let pinchPrevDist = 0

const ptPos = (e: PointerEvent) => {
  const rect = canvasRef.value!.getBoundingClientRect()
  return { x: e.clientX - rect.left, y: e.clientY - rect.top }
}

function onPointerDown(e: PointerEvent) {
  try {
    canvasRef.value?.setPointerCapture(e.pointerId)
  } catch { /* ignore */ }
  const p = ptPos(e)
  pointers.set(e.pointerId, p)
  if (pointers.size === 1) {
    dragMoved = false
    downX = p.x
    downY = p.y
  } else if (pointers.size === 2) {
    const [a, b] = [...pointers.values()]
    pinchPrevDist = Math.hypot(a.x - b.x, a.y - b.y)
  }
}

function onPointerMove(e: PointerEvent) {
  const prev = pointers.get(e.pointerId)
  if (!prev) return
  const cur = ptPos(e)
  pointers.set(e.pointerId, cur)
  if (pointers.size === 1) {
    if (!dragMoved && Math.hypot(cur.x - downX, cur.y - downY) > 4) {
      dragMoved = true
      follow.value = false
    }
    if (dragMoved) {
      cam.value = {
        ...cam.value,
        cx: cam.value.cx - (cur.x - prev.x) / cam.value.zoom,
        cy: cam.value.cy - (cur.y - prev.y) / cam.value.zoom,
      }
      invalidate()
    }
  } else if (pointers.size === 2) {
    const [a, b] = [...pointers.values()]
    const d = Math.hypot(a.x - b.x, a.y - b.y)
    if (pinchPrevDist > 0 && d > 0) {
      const mid = { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 }
      cam.value = zoomAt(mid.x, mid.y, d / pinchPrevDist, cam.value, vp.value)
      invalidate()
    }
    pinchPrevDist = d
    if (!dragMoved) {
      dragMoved = true
      follow.value = false
    }
  }
}

function onPointerUp(e: PointerEvent) {
  if (canvasRef.value?.hasPointerCapture?.(e.pointerId)) {
    canvasRef.value.releasePointerCapture(e.pointerId)
  }
  pointers.delete(e.pointerId)
  if (pointers.size < 2) pinchPrevDist = 0
}

function onWheel(e: WheelEvent) {
  e.preventDefault()
  const p = ptPos(e)
  cam.value = zoomAt(p.x, p.y, Math.exp(-e.deltaY * 0.0015), cam.value, vp.value)
  invalidate()
}

function onDblClick() {
  follow.value = true
  centerOnSelf()
}

onMounted(() => {
  raf = requestAnimationFrame(frame)
})

onBeforeUnmount(() => {
  cancelAnimationFrame(raf)
  ro?.disconnect()
  canvasRef.value?.removeEventListener('wheel', onWheel)
})

// ---- 状态徽标（对局状态中文） ----
const STATUS_LABEL: Record<string, string> = {
  running: '进行中',
  won: '胜利',
  lost: '失败',
  timeout: '超时',
}
const statusLabel = computed(() => {
  const s = state.value?.status
  return s ? (STATUS_LABEL[s] ?? s) : '—'
})

function errText(e: unknown) {
  return e instanceof Error ? e.message : String(e)
}
</script>

<template>
  <div class="replay">
    <!-- 加载区 -->
    <div class="topbar">
      <input
        v-model="matchId"
        class="id-input"
        placeholder="对局 ID（如 a1b2c3）"
        @keyup.enter="loadRecording(matchId)"
      />
      <button @click="loadRecording(matchId)" :disabled="loading || !matchId.trim()">加载录像</button>
      <button @click="loadCurrent" :disabled="loading">回放当前对局</button>
      <span v-if="recording" class="pack">{{ recording.scenario }} · seed {{ recording.seed }} · {{ frameCount }} 帧</span>
    </div>
    <p v-if="loadError" class="err">{{ loadError }}</p>

    <template v-if="recording && frameCount">
      <!-- 时间轴 -->
      <div class="timeline">
        <button @click="playing ? stopPlayback() : startPlayback()" class="play">
          {{ playing ? '⏸ 暂停' : '▶ 播放' }}
        </button>
        <button @click="stepFrame(-1)" :disabled="frameIdx === 0" title="单步 −1">−1</button>
        <button @click="stepFrame(1)" :disabled="frameIdx >= frameCount - 1" title="单步 +1">+1</button>
        <button @click="jumpToEnd" :disabled="frameIdx >= frameCount - 1">跳到结尾</button>
        <input
          type="range"
          class="slider"
          min="1"
          :max="frameCount"
          :value="frameIdx + 1"
          @input="onSliderInput(); frameIdx = Number(($event.target as HTMLInputElement).value) - 1"
        />
        <span class="tick-label">
          帧 {{ frameIdx + 1 }}/{{ frameCount }} · tick {{ currentFrame?.tick ?? '—' }}
          <template v-if="state">· T{{ state.turn }}/{{ state.turn_limit }} · {{ statusLabel }}</template>
          <span v-if="replayBusy" class="busy">重演中…</span>
        </span>
      </div>
      <p v-if="replayError" class="err">{{ replayError }}</p>

      <div class="main">
        <div class="map-wrap">
          <div ref="shellRef" class="canvas-shell">
            <canvas
              ref="canvasRef"
              @pointerdown="onPointerDown"
              @pointermove="onPointerMove"
              @pointerup="onPointerUp"
              @pointercancel="onPointerUp"
              @dblclick="onDblClick"
            />
            <div v-if="!follow" class="follow-badge">已解除跟随 · 双击恢复</div>
          </div>
          <div class="legend">
            <span><i class="sw" :style="{ background: '#12161d' }" />未探索</span>
            <span><i class="sw" :style="{ background: '#2a2f37' }" />已探索（记忆）</span>
            <span><i class="sw" :style="{ background: '#3a424e' }" />感知内</span>
            <span><i class="sw sw-order" />移动订单</span>
            <span>◆ 晶簇 · 💎 结晶 · ❓ 实体 · 🤖 本体</span>
            <span class="tip">拖拽平移 · 滚轮缩放 · 双击恢复跟随 · 每 600ms 前进一帧</span>
          </div>
          <p v-if="state" class="statusline">
            观测：形 {{ state.observation?.self.integrity ?? '—' }}/{{ state.observation?.self.max_integrity ?? '—' }}
            · 气 {{ state.observation?.self.battery ?? '—' }}
            · 热 {{ state.observation?.self.heat ?? '—' }}
            · 感知 {{ state.observation?.visible.length ?? 0 }} 格
            · 实体 {{ state.observation?.entities.length ?? 0 }}
          </p>
        </div>

        <!-- 帧事件流 -->
        <aside class="panel">
          <h2>帧事件流 <small>tick {{ currentFrame?.tick ?? '—' }} · {{ intentSummary }}</small></h2>
          <ul class="list evlist" v-if="frameEvents.length">
            <li
              v-for="(e, i) in frameEvents"
              :key="i"
              :class="{ combat: isCombatEvent(e) }"
            >
              <em>{{ e.type }}</em> {{ e.text }}
            </li>
          </ul>
          <p v-else class="empty">本帧无事件</p>
          <p class="tip note">
            事件由服务端逐帧重演得出（同 seed 同意图流），非客户端推算。
          </p>
        </aside>
      </div>
    </template>
    <p v-else-if="recording && !frameCount" class="placeholder">
      录像为空 —— 回地图页跑几步后再来。
    </p>
    <p v-else class="placeholder">
      输入对局 ID 或点「回放当前对局」加载录像。
    </p>
  </div>
</template>

<style scoped>
.replay { min-width: 0; }
.topbar { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.id-input {
  flex: 0 1 240px;
  background: var(--panel); color: var(--text); border: 1px solid #3a4150;
  border-radius: var(--radius); padding: 6px 8px; font-size: 13px;
  font-variant-numeric: tabular-nums;
}
.pack { font-size: 12.5px; color: var(--accent); font-variant-numeric: tabular-nums; }

.timeline {
  display: flex; align-items: center; gap: 8px; flex-wrap: wrap;
  margin-top: 10px; padding: 8px 10px;
  border: 1px solid var(--border); border-radius: var(--radius); background: var(--panel);
}
.play { min-width: 76px; }
.slider { flex: 1 1 200px; accent-color: var(--nav-active); min-width: 140px; }
.tick-label {
  font-size: 12.5px; color: #aab2bd; font-variant-numeric: tabular-nums;
  white-space: nowrap;
}
.tick-label .busy { color: var(--accent); margin-left: 6px; }

.main { display: flex; gap: 20px; flex-wrap: wrap; align-items: flex-start; margin-top: 12px; }
.map-wrap { flex: 1 1 480px; min-width: 0; }
.panel { flex: 0 0 300px; width: 300px; }

.canvas-shell {
  position: relative;
  height: clamp(320px, 52vh, 560px);
  border: 1px solid var(--border);
  background: #12161d;
  overflow: hidden;
}
.canvas-shell canvas {
  position: absolute; inset: 0;
  display: block;
  cursor: grab;
  touch-action: none;
  user-select: none;
}
.canvas-shell canvas:active { cursor: grabbing; }
.follow-badge {
  position: absolute; right: 10px; bottom: 10px;
  padding: 3px 10px; border: 1px solid var(--border); border-radius: var(--radius);
  background: rgba(23, 28, 36, 0.85); color: var(--text-dim); font-size: 12px;
  pointer-events: none;
}

.legend {
  display: flex; gap: 12px; align-items: center; flex-wrap: wrap;
  margin-top: 8px; font-size: 11.5px; color: var(--text-dim);
}
.legend .sw {
  display: inline-block; width: 10px; height: 10px;
  margin-right: 4px; border: 1px solid var(--border); vertical-align: -1px;
}
.legend .sw-order { background: transparent; border: 1.5px dashed var(--accent); }
.legend .tip { flex-basis: 100%; color: #59616d; }
.statusline { color: #aab2bd; font-size: 12.5px; margin: 8px 0 0; font-variant-numeric: tabular-nums; }

.panel h2 { margin-top: 0; }
.evlist li.combat {
  color: var(--fire);
  background: rgba(224, 85, 69, 0.08);
  border-left: 2px solid var(--fire);
  padding-left: 6px;
}
.evlist em { color: #59616d; font-style: normal; margin-right: 6px; font-size: 11px; }
.evlist li.combat em { color: var(--fire); }
.empty { color: var(--text-dim); font-size: 13px; }
.note { color: #59616d; font-size: 11.5px; margin-top: 10px; }
</style>
