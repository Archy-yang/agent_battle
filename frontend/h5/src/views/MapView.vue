<script setup lang="ts">
// v0.4 地图主页：单 <canvas> 无限六边形世界（设计 §4）。
// pointy-top / axial 坐标（{q,r} 契约），相机 {cx, cy, zoom} 平移缩放，
// 迷雾三态（未探索 / 已探索离开感知 / 感知内），点选已探索格下达 move_to。
// 六边形数学全部来自 ../hex 纯函数模块（无 Vue 依赖，Flutter 移植参考）；
// 数据源只有 ../state（Reality ≠ Agent Knowledge）：未探索格不请求、
// 不猜测地形；服务端当前只下发坐标，地形五行色在 essence 记忆下发后自动生效。
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import {
  TABS, GUIDANCE_OPTIONS, PREDICT_OPTIONS,
  spawnCell, orderTarget, exploredSet, visibleSet, exploredCellMap,
  view, busy, error, tab,
  guideKind, guideText, guideError, sendGuidance,
  question, askBusy, askError, ask,
  predictBusy, predictError, predict, pause,
  newMatch, step, runToEnd, sendMoveTo, sendTransfer, waitOneTurn, flashHint, mapHint,
  requestBattleNav, FOG,
  derivedCognition, costOf, choiceLabel,
} from '../state'
import { essenceColor, essenceColorByName, type Hex } from '../api'
import {
  fogOf, hexCorners, hexCenter, hexKey, pickHex, visibleHexes, worldToScreen, zoomAt,
  type Camera,
} from '../hex'

// ---- 常量 ----
const HEX_SIZE = 28 // 世界像素下的外接圆半径（zoom=1）
const ACCENT = '#d4a94e' // 与 --accent 一致
const FIRE = '#e05545' // 与 --fire 一致
const WATER = '#4a8fd4' // 与 --water 一致
const SELF_RING = '#9fd0ff'
const HEX_BORDER = 'rgba(15, 18, 22, 0.55)'

// ---- 相机 / 画布 ----
const shellRef = ref<HTMLDivElement | null>(null)
const canvasRef = ref<HTMLCanvasElement | null>(null)
const cam = ref<Camera>({ cx: 0, cy: 0, zoom: 1 })
const follow = ref(true) // 相机跟随 Agent；拖拽后解除，双击恢复
const vp = ref({ width: 640, height: 420 })
let dirty = true
const invalidate = () => { dirty = true }

// ---- 数据投影（全部来自 state，本组件不做 fetch） ----
const selfHex = computed<Hex | null>(() => {
  const o = view.value?.observation
  return o ? { q: o.self.pos.q, r: o.self.pos.r } : null
})

interface MarkerVm { q: number; r: number; glyph: string; color?: string; diamond?: boolean }
// 实体层：◆ 晶簇 / 💎 结晶 / ❓ 敌对实体（observation.entities + view.enemies，
// 五行色描边；晶簇按其本源上五行色菱形）
const markers = computed<MarkerVm[]>(() => {
  const out: MarkerVm[] = []
  const seen = new Set<string>()
  for (const e of view.value?.observation?.entities ?? []) {
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
  for (const en of view.value?.enemies ?? []) {
    const k = hexKey(en.pos)
    if (seen.has(k)) continue
    seen.add(k)
    out.push({ q: en.pos.q, r: en.pos.r, glyph: '❓', color: essenceColorByName(en.essence) })
  }
  return out
})

// 站上晶簇格时可采集：该晶簇实体 id（无则 null）
const gatherTarget = computed<number | null>(() => {
  const o = view.value?.observation
  if (!o) return null
  const p = o.self.pos
  const res = o.entities.find(e => e.kind === 'resource' && e.pos.q === p.q && e.pos.r === p.r)
  return res?.id ?? null
})

function centerOnSelf() {
  const h = selfHex.value
  if (!h) return
  const p = hexCenter(h, HEX_SIZE)
  cam.value = { ...cam.value, cx: p.x, cy: p.y }
  invalidate()
}

// 换局：恢复跟随并居中；每回合 self 移动且跟随开启时居中
watch(selfHex, () => { if (follow.value) centerOnSelf() })
watch(() => view.value?.id, () => { follow.value = true; centerOnSelf() })

// ---- 渲染 ----
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

  // 未探索 = 深底（不绘制的部分自然露出）
  ctx.fillStyle = FOG.unexplored
  ctx.fillRect(0, 0, width, height)
  if (s < 3) return

  const explored = exploredSet.value
  const visible = visibleSet.value
  const cells = exploredCellMap.value
  const order = orderTarget.value
  const spawn = spawnCell.value

  // 地形层：只渲染服务端已知格（explored / observation.visible）
  for (const h of visibleHexes(c, vp.value, HEX_SIZE)) {
    const fog = fogOf(h, explored, visible)
    if (fog === 'unexplored') continue
    const w = hexCenter(h, HEX_SIZE)
    const p = worldToScreen(w.x, w.y, c, vp.value)
    hexPath(ctx, p.x, p.y, r)
    if (fog === 'visible') {
      // 感知内全亮：服务端下发五行记忆（ExploredCell.essence）时按五行上色，
      // 否则中性亮灰 —— 客户端不猜测地形
      const cell = cells.get(hexKey(h))
      ctx.fillStyle = cell?.essence != null ? essenceColor(cell.essence) : FOG.visible
      ctx.fill()
      ctx.strokeStyle = HEX_BORDER
      ctx.lineWidth = 1
      ctx.stroke()
      if (cell?.danger) {
        // Danger 叠加：火色描边 + 斜线纹理
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
      // 已探索离开感知：45% 记忆色
      ctx.globalAlpha = 0.45
      ctx.fillStyle = FOG.remembered
      ctx.fill()
      ctx.globalAlpha = 1
      ctx.strokeStyle = HEX_BORDER
      ctx.lineWidth = 1
      ctx.stroke()
    }
  }

  // 移动订单目标：强调金虚线框
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

  // 实体层
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
    if (m.diamond) { // 晶簇：五行色菱形（站上去采集）
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
    if (m.color) { // 敌对实体：五行色描边
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

// 脏检查 + rAF：只在相机 / 数据 / 尺寸变化时重绘
let raf = 0
function frame() {
  if (dirty) {
    dirty = false
    draw()
  }
  raf = requestAnimationFrame(frame)
}

// ---- 尺寸（devicePixelRatio 适配） ----
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

// ---- 交互：拖拽平移 / 滚轮与双指缩放 / 点击拾取 ----
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
  // setPointerCapture 对未激活 pointerId 会抛 NotFoundError（合成事件/个别触屏驱动），
  // 失败只影响拖拽捕获，不阻断拾取
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
      follow.value = false // 用户拖拽后解除跟随
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
  const had = pointers.delete(e.pointerId)
  if (canvasRef.value?.hasPointerCapture?.(e.pointerId)) {
    canvasRef.value.releasePointerCapture(e.pointerId)
  }
  if (pointers.size < 2) pinchPrevDist = 0
  if (had && pointers.size === 0 && !dragMoved) {
    const p = ptPos(e)
    onPick(p.x, p.y)
  }
}

// 点击拾取 → 已探索格发 move_to；未探索格只提示（信息边界：不向服务端查询）
function onPick(sx: number, sy: number) {
  const v = view.value
  if (!v || busy.value) return
  if (v.status !== 'running') {
    flashHint('对局已结束')
    return
  }
  const h = pickHex(sx, sy, cam.value, vp.value, HEX_SIZE)
  const k = hexKey(h)
  if (!exploredSet.value.has(k) && !visibleSet.value.has(k)) {
    flashHint(`未探明的区域 (${h.q},${h.r})`)
    return
  }
  sendMoveTo(h.q, h.r) // 服务端判定可通行/可达，拒绝时回显 intent_rejected 文案
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
  ro = new ResizeObserver(resize)
  ro.observe(shellRef.value!)
  resize()
  centerOnSelf()
  canvasRef.value?.addEventListener('wheel', onWheel, { passive: false })
  raf = requestAnimationFrame(frame)
})

onBeforeUnmount(() => {
  cancelAnimationFrame(raf)
  ro?.disconnect()
  canvasRef.value?.removeEventListener('wheel', onWheel)
})
</script>

<template>
  <div class="main">
    <div class="map-wrap">
      <!-- 场景选择 + 当前 pack -->
      <div class="topbar">
        <span class="pack" :title="'当前场景包'">{{ view?.pack ?? '未开局' }}</span>
        <span class="spacer" />
        <button @click="newMatch('mvp')" :disabled="busy">新对局 · 经典局</button>
        <button @click="newMatch('v02')" :disabled="busy">新对局 · 大地图局</button>
        <button @click="newMatch('v04')" :disabled="busy">新对局 · 无限世界（v0.4）</button>
      </div>

      <!-- 遭遇战横幅 -->
      <div v-if="view?.in_combat" class="combat-banner">
        <span>⚠ 遭遇战中</span>
        <button class="go-battle" @click="requestBattleNav()">前往对战</button>
      </div>

      <!-- 六边形无限地图：单 canvas，拖拽平移 / 滚轮·双指缩放 / 点选下单 -->
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
        <span><i class="sw" :style="{ background: FOG.unexplored }" />未探索</span>
        <span><i class="sw" :style="{ background: FOG.remembered, opacity: 0.45 }" />已探索（记忆）</span>
        <span><i class="sw" :style="{ background: FOG.visible }" />感知内</span>
        <span><i class="sw sw-order" />移动订单</span>
        <span class="res-leg">◆ 晶簇（站上去采集）</span>
        <span class="tip">
          拖拽平移 · 滚轮/双指缩放（0.5~2.0） · 点击已探索格下达移动订单（每回合自动走一格） · 双击恢复跟随
        </span>
      </div>

      <div class="btns">
        <button
          v-if="gatherTarget !== null"
          class="gather"
          @click="sendTransfer(gatherTarget)"
          :disabled="busy || view?.status !== 'running'"
        >采集晶簇</button>
        <button @click="step(true)" :disabled="busy || view?.status !== 'running'">自动一步</button>
        <button @click="waitOneTurn" :disabled="busy || view?.status !== 'running'">待命一回合</button>
        <button @click="runToEnd" :disabled="busy || view?.status !== 'running'">自动到结束</button>
      </div>
      <p v-if="mapHint" class="hint">{{ mapHint }}</p>
      <p v-if="error" class="err">{{ error }}</p>
    </div>

    <aside class="panel">
      <nav class="tabs">
        <button
          v-for="t in TABS" :key="t.key"
          :class="{ active: tab === t.key }"
          @click="tab = t.key"
        >{{ t.label }}</button>
      </nav>

      <!-- 认知 -->
      <section v-show="tab === 'cognition'">
        <div v-if="view && view.status !== 'running'" class="review-card">
          <h2>赛后复盘</h2>
          <template v-if="view.review">
            <p class="review-summary">{{ view.review.summary }}</p>
            <ul class="list lessons">
              <li v-for="(l, i) in view.review.lessons" :key="i">教训：{{ l }}</li>
            </ul>
          </template>
          <p v-else class="final">对局已结束：{{ view.status }}</p>
        </div>

        <h2>Agent 认知摘要 <small>{{ view?.cognition ? 'LLM 自报' : 'v0.1 派生' }}</small></h2>
        <dl v-if="view?.cognition">
          <dt>目标</dt><dd>{{ view.cognition.goal }}</dd>
          <dt>计划</dt><dd>{{ view.cognition.plan }}</dd>
          <dt>已知</dt><dd>{{ view.cognition.known }}</dd>
          <dt>疑点</dt><dd>{{ view.cognition.suspicions }}</dd>
          <dt>风险</dt><dd>{{ view.cognition.risk }}</dd>
          <dt>置信</dt><dd>{{ view.cognition.confidence.toFixed(2) }}</dd>
        </dl>
        <dl v-else-if="derivedCognition.length">
          <template v-for="[k, v] in derivedCognition" :key="k">
            <dt>{{ k }}</dt>
            <dd>{{ v }}</dd>
          </template>
        </dl>
        <p v-else>—</p>
      </section>

      <!-- 事件 -->
      <section v-show="tab === 'events'">
        <h2>最近事件</h2>
        <ul class="list" v-if="(view?.events ?? []).length">
          <li v-for="(e, i) in (view?.events ?? []).slice(-8).reverse()" :key="i">
            <em>T{{ e.tick }}</em> {{ e.text }}
          </li>
        </ul>
        <p v-else>—</p>
      </section>

      <!-- 对话 -->
      <section v-show="tab === 'qa'">
        <h2>Ask <small>免费提问，Agent 第一人称回答</small></h2>
        <div class="guide">
          <input v-model="question" placeholder="例如：你现在最怕什么？" @keyup.enter="ask" />
          <button @click="ask" :disabled="askBusy || !view">提问</button>
        </div>
        <p v-if="askError" class="sub-err">{{ askError }}</p>
        <ul class="qa-list" v-if="(view?.qa ?? []).length">
          <li v-for="(q, i) in (view?.qa ?? []).slice().reverse()" :key="'q' + i">
            <p class="q">Q：{{ q.question }}</p>
            <p class="a">A：{{ q.answer }}</p>
          </li>
        </ul>
        <p v-else>—</p>
      </section>

      <!-- 预测 -->
      <section v-show="tab === 'predict'">
        <h2>Predict <small>猜中 Agent 下一步 +0.5</small></h2>
        <div class="predict-btns">
          <button
            v-for="o in PREDICT_OPTIONS" :key="o.value"
            @click="predict(o.value)"
            :disabled="predictBusy || !view || view.status !== 'running'"
          >{{ o.label }}</button>
        </div>
        <p v-if="predictError" class="sub-err">{{ predictError }}</p>
        <p v-if="view?.pending" class="pending">
          已预测：{{ choiceLabel(view.pending.choice) }}，等下一回合结算
        </p>
        <div class="pause-row">
          <button
            class="danger"
            @click="pause"
            :disabled="predictBusy || !view || view.status !== 'running'"
          >紧急暂停 (⚡{{ (costOf('pause') || 3).toFixed(1) }})</button>
          <small>每局一次，消耗影响点</small>
        </div>
        <h2>预测历史</h2>
        <ul class="list" v-if="(view?.predictions ?? []).length">
          <li v-for="(p, i) in (view?.predictions ?? []).slice().reverse()" :key="'p' + i">
            <em>T{{ p.turn }}</em>
            {{ choiceLabel(p.choice) }}
            <template v-if="p.resolved">
              → 实际 {{ choiceLabel(p.actual) }} ·
              <b :class="p.hit ? 'hit' : 'miss'">{{ p.hit ? '命中 ✓' : '未中 ✗' }}</b>
            </template>
            <template v-else> · 待结算</template>
          </li>
        </ul>
        <p v-else>—</p>
      </section>

      <h2>Guidance <small>给偏好，不给动作</small></h2>
      <div class="guide">
        <select v-model="guideKind">
          <option v-for="o in GUIDANCE_OPTIONS" :key="o.value" :value="o.value">
            {{ o.label }} (⚡{{ costOf(o.value).toFixed(1) }})
          </option>
        </select>
        <input v-model="guideText" placeholder="例如：先确认它的储气再决定是否接战" @keyup.enter="sendGuidance" />
        <button @click="sendGuidance" :disabled="busy || !view">发送</button>
      </div>
      <p v-if="guideError" class="sub-err">{{ guideError }}</p>
      <ul class="list">
        <li v-for="(g, i) in (view?.guidance ?? []).slice().reverse()" :key="'g' + i">
          <em>T{{ g.turn }} {{ g.kind }}</em> {{ g.text }}
        </li>
      </ul>
    </aside>
  </div>
</template>

<style scoped>
.main { display: flex; gap: 20px; flex-wrap: wrap; align-items: flex-start; }
.map-wrap { flex: 1 1 520px; min-width: 0; }
.panel { flex: 0 0 320px; width: 320px; }

/* ---- 顶栏：pack + 场景选择 ---- */
.topbar { display: flex; align-items: center; gap: 8px; margin-bottom: 8px; flex-wrap: wrap; }
.pack { font-size: 13px; color: var(--accent); font-variant-numeric: tabular-nums; }
.spacer { flex: 1; }

/* ---- 遭遇战横幅 ---- */
.combat-banner {
  display: flex; align-items: center; justify-content: space-between;
  padding: 6px 12px; margin-bottom: 8px;
  border: 1px solid #5a3a3a; border-radius: var(--radius);
  background: #2a1d20; color: var(--fire); font-size: 13px; font-weight: 600;
}
.go-battle { padding: 3px 10px; font-size: 12px; }

/* ---- 画布容器：单 canvas 无限地图 ---- */
.canvas-shell {
  position: relative;
  height: clamp(360px, 58vh, 620px);
  border: 1px solid var(--border);
  background: #12161d;
  overflow: hidden;
}
.canvas-shell canvas {
  position: absolute; inset: 0;
  display: block;
  cursor: grab;
  touch-action: none; /* pointer events 接管触屏拖拽/双指 */
  user-select: none;
}
.canvas-shell canvas:active { cursor: grabbing; }
.follow-badge {
  position: absolute; right: 10px; bottom: 10px;
  padding: 3px 10px; border: 1px solid var(--border); border-radius: var(--radius);
  background: rgba(23, 28, 36, 0.85); color: var(--text-dim); font-size: 12px;
  pointer-events: none;
}

/* ---- 图例 ---- */
.legend {
  display: flex; gap: 12px; align-items: center; flex-wrap: wrap;
  margin-top: 8px; font-size: 11.5px; color: var(--text-dim);
}
.legend .sw {
  display: inline-block; width: 10px; height: 10px;
  margin-right: 4px; border: 1px solid var(--border); vertical-align: -1px;
}
.legend .sw-order { background: transparent; border: 1.5px dashed var(--accent); }
.legend .res-leg { color: var(--wood); }
.legend .tip { flex-basis: 100%; color: #59616d; }

.btns { display: flex; gap: 8px; margin-top: 10px; flex-wrap: wrap; }
.btns .gather { border-color: var(--wood); color: var(--wood); }
.hint { color: var(--accent); font-size: 12.5px; margin: 8px 0 0; }

.panel h2 { font-size: 14px; margin: 18px 0 6px; color: var(--nav-active); }
.panel h2 small { color: #59616d; font-weight: normal; }
.review-card {
  margin-top: 14px; padding: 10px 12px;
  border: 1px solid #3a4150; border-radius: var(--radius); background: var(--panel);
}
.review-card h2 { margin-top: 0; }
.review-summary { font-size: 13px; line-height: 1.6; margin: 4px 0; }
.lessons li:last-child { border-bottom: none; }
.final { color: var(--text-dim); font-size: 13px; margin: 4px 0; }
.predict-btns { display: flex; gap: 6px; flex-wrap: wrap; }
.pending { color: var(--accent); font-size: 12.5px; margin: 8px 0 0; }
.pause-row { display: flex; align-items: center; gap: 8px; margin-top: 12px; }
.pause-row small { color: #59616d; }
.qa-list { list-style: none; margin: 10px 0 0; padding: 0; font-size: 12.5px; }
.qa-list li { padding: 6px 0; border-bottom: 1px dashed var(--border); }
.qa-list p { margin: 2px 0; line-height: 1.55; }
.qa-list .q { color: var(--nav-active); }
.qa-list .a { color: var(--text); }
</style>
