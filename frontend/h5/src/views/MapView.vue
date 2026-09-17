<script setup lang="ts">
// 默认页：地图网格 + 对局控制 + 认知/事件/对话/预测面板（自 App.vue 迁入，功能不变）。
// 全部状态与动作来自 ../state，本组件不直接 fetch。
import {
  TABS, GUIDANCE_OPTIONS, PREDICT_OPTIONS,
  W, H, SPAWN,
  view, busy, error, tab,
  guideKind, guideText, guideError, sendGuidance,
  question, askBusy, askError, ask,
  predictBusy, predictError, predict, pause,
  newMatch, step, runToEnd,
  derivedCognition, costOf, choiceLabel,
  cellStyle, selfAt, entityAt,
} from '../state'
</script>

<template>
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
.main { display: flex; gap: 20px; flex-wrap: wrap; }
.map-wrap { flex: 0 0 auto; }
.grid { display: inline-block; border: 1px solid var(--border); font-variant-numeric: tabular-nums; }
.row { display: flex; }
.cell {
  width: 38px; height: 38px;
  display: flex; align-items: center; justify-content: center;
  font-size: 18px; position: relative;
}
.cell .spawn { color: var(--water); font-size: 12px; }
.btns { display: flex; gap: 8px; margin-top: 10px; flex-wrap: wrap; }
.panel { flex: 1 1 320px; min-width: 300px; }
.tabs { display: flex; gap: 4px; border-bottom: 1px solid var(--border); margin-bottom: 4px; }
.tabs button {
  background: transparent; border: none; border-bottom: 2px solid transparent;
  border-radius: 0; color: var(--text-dim); padding: 6px 10px;
}
.tabs button:hover:not(:disabled) { background: transparent; color: var(--text); }
.tabs button.active { color: #fff; border-bottom-color: var(--nav-active); }
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
