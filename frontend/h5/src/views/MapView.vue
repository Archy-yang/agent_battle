<script setup lang="ts">
// M3.4 地图主页：12×12（mvp）/ 24×24（v02）大地图 + 点选寻路 + 迷雾三层。
// 全部状态与动作来自 ../state，本组件不直接 fetch。
// 地形不再由客户端先验（terrainDanger/terrainEssence 已废弃）：
// 迷雾只读 view.explored 与 view.observation.visible；服务端 explored
// 目前只有坐标没有五行色，"已探索未感知"渲染为中性暗土色，
// v0.4 接入服务端地形记忆（api.ts ExploredCell）后恢复五行地形色。
import { computed } from 'vue'
import {
  TABS, GUIDANCE_OPTIONS, PREDICT_OPTIONS,
  mapDims, spawnCell, orderTarget, exploredSet, visibleSet,
  view, busy, error, tab,
  guideKind, guideText, guideError, sendGuidance,
  question, askBusy, askError, ask,
  predictBusy, predictError, predict, pause,
  newMatch, step, runToEnd, sendMoveTo, waitOneTurn, flashHint, mapHint,
  requestBattleNav,
  derivedCognition, costOf, choiceLabel,
  cellStyle, selfAt,
} from '../state'
import { essenceColor, essenceColorByName } from '../api'

const dims = mapDims

// ---- 格子数据：一次性展开为扁平数组（576 格无压力），模板只做渲染 ----
interface CellVm {
  x: number
  y: number
  key: string
  known: boolean // 已探索或当前感知内：可点选下达 move_to
  isSelf: boolean
  isSpawn: boolean
  isOrder: boolean
  marker: { glyph: string; color?: string } | null
  style: Record<string, string | number>
}

const cells = computed<CellVm[]>(() => {
  const { w, h } = dims.value
  const order = orderTarget.value
  const spawn = spawnCell.value
  const explored = exploredSet.value
  const visible = visibleSet.value

  // 实体标记：observation.entities（💎 结晶 / ❓ 其他实体）+ view.enemies
  // （同源补充：怪物标注五行色描边）。按格去重，一格一个标记。
  const markers = new Map<string, { glyph: string; color?: string }>()
  for (const e of view.value?.observation?.entities ?? []) {
    markers.set(`${e.pos.x},${e.pos.y}`, e.kind === 'fragment'
      ? { glyph: '💎' }
      : { glyph: '❓', color: essenceColor(e.essence) })
  }
  for (const en of view.value?.enemies ?? []) {
    const k = `${en.pos.x},${en.pos.y}`
    if (!markers.has(k)) markers.set(k, { glyph: '❓', color: essenceColorByName(en.essence) })
  }

  const out: CellVm[] = []
  for (let y = 0; y < h; y++) {
    for (let x = 0; x < w; x++) {
      const key = `${x},${y}`
      const known = explored.has(key) || visible.has(key)
      out.push({
        x, y, key,
        known,
        isSelf: selfAt(x, y),
        isSpawn: !!spawn && spawn.x === x && spawn.y === y,
        isOrder: !!order && order.x === x && order.y === y,
        marker: markers.get(key) ?? null,
        style: cellStyle(x, y),
      })
    }
  }
  return out
})

// ---- 点选寻路 ----
function onCell(c: CellVm) {
  if (!view.value || busy.value) return
  if (view.value.status !== 'running') {
    flashHint('对局已结束')
    return
  }
  if (!c.known) {
    flashHint('未探明的区域')
    return
  }
  sendMoveTo(c.x, c.y) // 服务端判定可通行/可达，拒绝时回显 intent_rejected 文案
}
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
      </div>

      <!-- 遭遇战横幅 -->
      <div v-if="view?.in_combat" class="combat-banner">
        <span>⚠ 遭遇战中</span>
        <button class="go-battle" @click="requestBattleNav()">前往对战</button>
      </div>

      <!-- 地图：24×24 桌面一屏（26px 格），窄屏等比缩放 -->
      <div
        class="grid"
        :style="{ '--cols': String(dims.w), '--cell-px': dims.w > 12 ? '26px' : '34px' }"
      >
        <div
          v-for="c in cells" :key="c.key"
          class="cell"
          :class="{ known: c.known, order: c.isOrder }"
          :style="c.style"
          @click="onCell(c)"
        >
          <span v-if="c.isSelf" class="glyph">🤖</span>
          <span
            v-else-if="c.marker"
            class="glyph"
            :style="c.marker.color ? { textShadow: '0 0 3px ' + c.marker.color, outline: '1px solid ' + c.marker.color } : undefined"
          >{{ c.marker.glyph }}</span>
          <span v-else-if="c.isSpawn" class="glyph spawn">⌂</span>
        </div>
      </div>

      <div class="legend">
        <span><i class="sw" :style="{ background: '#12161d' }" />未探索</span>
        <span><i class="sw" :style="{ background: '#2a2f37' }" />已探索（记忆）</span>
        <span><i class="sw" :style="{ background: '#3a424e' }" />感知内</span>
        <span><i class="sw sw-order" />移动订单</span>
        <span class="tip">点击已探索/感知内格子下达移动订单（每回合自动走一格）</span>
      </div>

      <div class="btns">
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
.map-wrap { flex: 0 1 auto; min-width: 0; }
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

/* ---- 网格：CSS 变量驱动格距，24×24 桌面 26px 一屏放下；窄屏等比缩放 ---- */
.grid {
  --cell: min(var(--cell-px), calc((100vw - 420px) / var(--cols)));
  display: grid;
  grid-template-columns: repeat(var(--cols), var(--cell));
  grid-auto-rows: var(--cell);
  border: 1px solid var(--border);
  background: #12161d;
  user-select: none;
  touch-action: manipulation;
}
@media (max-width: 900px) {
  /* 窄屏/触屏：面板换行到下方，格子按视口等比缩放 */
  .grid { --cell: min(var(--cell-px), calc((100vw - 56px) / var(--cols))); }
}
.cell {
  position: relative;
  display: flex; align-items: center; justify-content: center;
  cursor: default;
}
.cell.known { cursor: pointer; }
.cell.known:hover { outline: 1px solid var(--nav-active); outline-offset: -1px; z-index: 1; }
/* 移动订单目标格：强调金虚线框 */
.cell.order { outline: 2px dashed var(--accent); outline-offset: -2px; z-index: 2; }
.glyph { font-size: calc(var(--cell) * 0.58); line-height: 1; pointer-events: none; }
.glyph.spawn { color: var(--water); font-size: calc(var(--cell) * 0.5); }

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
.legend .tip { flex-basis: 100%; color: #59616d; }

.btns { display: flex; gap: 8px; margin-top: 10px; flex-wrap: wrap; }
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
