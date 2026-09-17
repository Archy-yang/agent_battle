<script setup lang="ts">
// M3.5 对战页：遭遇战态势 + 战斗文字流 + LLM 助手 + 快捷指令。
// 全部状态与动作来自 ../state，本组件不直接 fetch；战斗扩展类型见 ../types-battle。
import { computed, nextTick, ref, watch } from 'vue'
import {
  view, busy, error,
  GUIDANCE_OPTIONS, PREDICT_OPTIONS,
  guideKind, guideText, guideError, sendGuidance,
  question, askBusy, askError, ask,
  predictBusy, predictError, predict,
  costOf, choiceLabel,
  step, runToEnd,
  triggerLabel,
} from '../state'
import { essenceName, essenceColor, essenceColorByName } from '../api'
import type { TacticsView } from '../api'
import type { BattleStateView } from '../types-battle'

// ---- 视图（战斗扩展字段，旧契约下为 undefined → 走空态渲染） ----
const bv = computed(() => view.value as BattleStateView | null)
const self = computed(() => bv.value?.observation?.self ?? null)
const inCombat = computed(() => bv.value?.in_combat ?? false)
const enemies = computed(() => (bv.value?.enemies ?? []).slice(0, 3))
const qa = computed(() => (bv.value?.qa ?? []).slice(-3).reverse())
const running = computed(() => bv.value?.status === 'running')

// ---- 战斗文字流：全部事件，最新在上；战斗类高亮 ----
const COMBAT_TYPES = new Set(['attack', 'backlash', 'spill', 'overheat', 'destroyed'])
const isCombat = (t: string) => COMBAT_TYPES.has(t)
const events = computed(() => (bv.value?.events ?? []).slice().reverse())

// 自动滚动：最新在上 → 有新事件且用户未手动上滚时回到顶部
const logEl = ref<HTMLElement | null>(null)
const pinned = ref(true)
const onLogScroll = () => {
  if (!logEl.value) return
  pinned.value = logEl.value.scrollTop <= 4
}
watch(
  () => (bv.value?.events ?? []).length,
  () => {
    if (!pinned.value) return
    nextTick(() => logEl.value?.scrollTo({ top: 0 }))
  },
)

// ---- 快捷 Ask chips：复用共享 question ref，走同一个 ask 动作 ----
const ASK_CHIPS = [
  '它的本源强度如何？',
  '它下一步会做什么？',
  '我该撤退吗？',
  '它的火迹为什么这么强？',
]
const chipAsk = (q: string) => {
  if (askBusy.value || !view.value) return
  question.value = q
  ask()
}

// ---- 仪表条 ----
const pct = (v: number, max: number) =>
  max > 0 ? Math.max(0, Math.min(100, (v / max) * 100)) : 0
const heatPct = computed(() => pct(self.value?.heat ?? 0, 25))
const heatColor = computed(() =>
  (self.value?.heat ?? 0) > 18 ? 'var(--fire)' : 'var(--metal)')
const heatWarn = computed(() => (self.value?.heat ?? 0) > 18)

// ---- v0.6 克关系战术预览：敌人卡下的"我的火 → 它的金【衡】"一行 ----
// tactics 与 enemies 同感知边界，按 enemy_id 关联；旧对局无此字段 → 不渲染。
const tacticOf = (id: number): TacticsView | undefined =>
  (bv.value?.tactics ?? []).find((t) => t.enemy_id === id)

// 落区语义色：侮/乘凶（火色），微弱（金属灰），衡吉（木色），中性暗灰
const ZONE_COLORS: Record<string, string> = {
  侮: 'var(--fire)',
  微: 'var(--metal)',
  衡: 'var(--wood)',
  乘: 'var(--fire)',
  中性: '#8d95a1',
}
const zoneColor = (z: string): string => ZONE_COLORS[z] ?? '#8d95a1'
</script>

<template>
  <div class="battle">
    <!-- 脱离接战提示条：切页逻辑归 App.vue / MapView，这里只做状态呈现 -->
    <div v-if="!inCombat" class="off-banner">
      <span class="off-icon">🛡</span>
      <span>脱离接战 —— 感知范围内暂无交战目标，可返回地图继续探索；遭遇敌人后自动进入遭遇战。</span>
      <span class="off-hint">（地图页 ← 导航栏可切换）</span>
    </div>

    <!-- 顶部：我方仪表 + ⚔ 遭遇战 + 回合控制 + 敌方列表 -->
    <div class="topbar">
      <div class="meters" v-if="self">
        <div class="meter">
          <span class="mlabel wood-c">形</span>
          <div class="track"><div class="fill" :style="{ width: pct(self.integrity, self.max_integrity) + '%', background: 'var(--wood)' }"></div></div>
          <span class="mval">{{ self.integrity }}/{{ self.max_integrity }}</span>
        </div>
        <div class="meter">
          <span class="mlabel accent-c">气</span>
          <div class="track"><div class="fill" :style="{ width: pct(self.battery, self.max_battery) + '%', background: 'var(--accent)' }"></div></div>
          <span class="mval">{{ self.battery }}/{{ self.max_battery }}</span>
        </div>
        <div class="meter">
          <span class="mlabel" :class="{ 'fire-c': heatWarn }">热</span>
          <div class="track"><div class="fill" :style="{ width: heatPct + '%', background: heatColor }"></div></div>
          <span class="mval" :class="{ 'fire-c': heatWarn }">{{ self.heat }}/25</span>
        </div>
      </div>
      <div class="meters" v-else>
        <p class="empty">等待观测数据…</p>
      </div>

      <div class="mid">
        <div class="combat-title" :class="{ off: !inCombat }">⚔ 遭遇战</div>
        <div class="ctrl">
          <button @click="step(true)" :disabled="busy || !running">交战一步</button>
          <button @click="runToEnd" :disabled="busy || !running">自动到结束</button>
        </div>
      </div>

      <div class="foes">
        <p v-if="!enemies.length" class="empty">感知范围内无敌对实体</p>
        <div
          v-for="e in enemies" :key="e.id"
          class="foe"
          :style="{ borderColor: essenceColor(e.essence) }"
        >
          <div class="foe-head">
            <b :style="{ color: essenceColor(e.essence) }">{{ essenceName(e.essence) }}系 #{{ e.id }}</b>
            <span v-if="e.distance <= 1" class="engage">接战中</span>
            <span v-else class="dist">{{ e.distance }} 格</span>
          </div>
          <div class="foe-stats">本源 {{ essenceName(e.essence) }} · 形 {{ e.integrity }} · 热 {{ e.heat }}</div>
          <!-- v0.6 克关系战术预览：引擎演算的落区推算（无离火 → 提示行） -->
          <div v-if="tacticOf(e.id)" class="tactic">
            <template v-if="tacticOf(e.id)!.my_essence">
              <span class="t-seg">我的<b :style="{ color: essenceColorByName(tacticOf(e.id)!.my_essence) }">{{ tacticOf(e.id)!.my_essence }}</b></span>
              <span class="t-arrow">→</span>
              <span class="t-seg">它的<b :style="{ color: essenceColorByName(tacticOf(e.id)!.enemy_essence) }">{{ tacticOf(e.id)!.enemy_essence }}</b></span>
              <span class="t-seg">【<b :style="{ color: zoneColor(tacticOf(e.id)!.predicted_zone) }">{{ tacticOf(e.id)!.predicted_zone }}</b>】</span>
              <span class="t-note">{{ tacticOf(e.id)!.note }}</span>
            </template>
            <span v-else class="t-note dim">{{ tacticOf(e.id)!.note }}</span>
          </div>
        </div>
      </div>
    </div>

    <!-- v0.5 战斗反馈：最近 LLM 触发原因 + 本局调用计数 -->
    <div class="trigger-row">
      <span class="tl-label">触发原因</span>
      <b class="tl-value">{{ triggerLabel(view?.last_trigger) }}</b>
      <span class="tl-sep">·</span>
      <span>本局 LLM 判断 {{ view?.llm_calls ?? 0 }} 次</span>
    </div>

    <p v-if="error" class="err">{{ error }}</p>

    <div class="cols">
      <!-- 左：战斗文字流 + LLM 助手 -->
      <section class="left">
        <div class="log" ref="logEl" @scroll="onLogScroll">
          <p v-if="!events.length" class="empty">暂无战斗事件 —— 交战后此处滚动播报。</p>
          <div
            v-for="(e, i) in events" :key="i"
            class="ev"
            :class="{ hot: isCombat(e.type) }"
          >
            <em>T{{ e.tick }}</em>
            <b class="tag">{{ e.type }}</b>
            <span>{{ e.text }}</span>
          </div>
        </div>

        <div class="assistant">
          <div class="avatar" aria-hidden="true">
            <svg viewBox="0 0 24 24" class="aura">
              <circle cx="12" cy="12" r="11" fill="none" stroke="var(--wood)" stroke-width="1.4" opacity="0.9" />
              <circle cx="12" cy="12" r="8" fill="none" stroke="var(--water)" stroke-width="1" opacity="0.5" />
            </svg>
            <span class="face">🤖</span>
          </div>
          <div class="bubble">
            <template v-if="busy">
              <p class="thinking">判断中…</p>
            </template>
            <template v-else-if="view?.cognition">
              <p><i class="k wood-c">Goal</i>{{ view.cognition.goal }}</p>
              <p><i class="k accent-c">Plan</i>{{ view.cognition.plan }}</p>
              <p><i class="k fire-c">Risk</i>{{ view.cognition.risk }}</p>
              <p class="conf">置信度 <b>{{ view.cognition.confidence.toFixed(2) }}</b></p>
            </template>
            <template v-else>
              <p class="waiting">等待 Agent 心跳…</p>
            </template>
          </div>
        </div>
      </section>

      <!-- 右：快捷指令区 -->
      <aside class="right">
        <h2>快捷 Ask <small>点击即问，Agent 第一人称回答</small></h2>
        <div class="chips">
          <button
            v-for="c in ASK_CHIPS" :key="c"
            class="chip"
            @click="chipAsk(c)"
            :disabled="askBusy || !view"
          >{{ c }}</button>
        </div>
        <p v-if="askBusy" class="hint">Agent 思考中…</p>
        <p v-if="askError" class="err">{{ askError }}</p>

        <h3>最近回答</h3>
        <ul class="qa" v-if="qa.length">
          <li v-for="(q, i) in qa" :key="'q' + i">
            <p class="q">Q：{{ q.question }}</p>
            <p class="a">A：{{ q.answer }}</p>
          </li>
        </ul>
        <p v-else class="empty">尚无问答记录</p>

        <h2>Guidance <small>给偏好，不给动作</small></h2>
        <div class="guide">
          <select v-model="guideKind">
            <option v-for="o in GUIDANCE_OPTIONS" :key="o.value" :value="o.value">
              {{ o.label }} (⚡{{ costOf(o.value).toFixed(1) }})
            </option>
          </select>
          <input
            v-model="guideText"
            placeholder="例如：热超过 20 就撤出危险区"
            @keyup.enter="sendGuidance"
          />
          <button @click="sendGuidance" :disabled="busy || !view">发送</button>
        </div>
        <p v-if="guideError" class="err">{{ guideError }}</p>

        <h2>预测 <small>猜中 Agent 下一步 +0.5</small></h2>
        <div class="predict-btns">
          <button
            v-for="o in PREDICT_OPTIONS" :key="o.value"
            @click="predict(o.value)"
            :disabled="predictBusy || !view || !running"
          >{{ o.label }}</button>
        </div>
        <p v-if="predictError" class="err">{{ predictError }}</p>
        <p v-if="view?.pending" class="pending">
          已预测：{{ choiceLabel(view.pending.choice) }}，等下一回合结算
        </p>

        <h3>预测历史</h3>
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
        <p v-else class="empty">尚无预测记录</p>
      </aside>
    </div>
  </div>
</template>

<style scoped>
.battle { display: flex; flex-direction: column; gap: 12px; }

/* ---- 脱离接战提示条 ---- */
.off-banner {
  display: flex; align-items: center; gap: 8px; flex-wrap: wrap;
  padding: 8px 12px; border: 1px dashed var(--border); border-radius: var(--radius);
  background: var(--panel); color: var(--text-dim); font-size: 13px;
}
.off-icon { color: var(--metal); }
.off-hint { color: #59616d; font-size: 12px; margin-left: auto; }

/* ---- 顶部态势 ---- */
.topbar {
  display: flex; gap: 16px; flex-wrap: wrap; align-items: flex-start;
  padding: 10px 12px; border: 1px solid var(--border); border-radius: var(--radius);
  background: var(--panel);
}
.meters { flex: 1 1 220px; display: flex; flex-direction: column; gap: 6px; min-width: 200px; }
.meter { display: flex; align-items: center; gap: 8px; font-variant-numeric: tabular-nums; }
.mlabel { width: 16px; text-align: center; font-size: 12.5px; color: var(--text-dim); }
.track {
  flex: 1; height: 8px; border-radius: 4px; background: #0c0f13;
  border: 1px solid var(--border); overflow: hidden;
}
.fill { height: 100%; border-radius: 4px; transition: width .25s ease; }
.mval { font-size: 12px; color: #aab2bd; min-width: 52px; }

.mid { display: flex; flex-direction: column; align-items: center; gap: 8px; }
.combat-title { font-size: 15px; font-weight: 600; color: var(--fire); letter-spacing: 2px; }
.combat-title.off { color: var(--text-dim); }
.ctrl { display: flex; gap: 6px; }

.foes { flex: 1 1 240px; display: flex; flex-direction: column; gap: 6px; min-width: 220px; }
.foe {
  border: 1px solid; border-radius: var(--radius);
  background: #1a2029; padding: 6px 10px; font-size: 12.5px;
}
.foe-head { display: flex; align-items: center; gap: 8px; }
.foe-head b { font-size: 13px; }
.foe-head .dist { color: var(--text-dim); margin-left: auto; }
.foe-head .engage {
  margin-left: auto; color: var(--fire); font-size: 11px;
  border: 1px solid var(--fire); border-radius: 8px; padding: 0 6px;
}
.foe-stats { color: #aab2bd; margin-top: 3px; font-variant-numeric: tabular-nums; }

/* ---- v0.6 克关系战术预览行 ---- */
.tactic {
  margin-top: 4px; padding-top: 4px;
  border-top: 1px dashed var(--border);
  display: flex; align-items: baseline; gap: 4px; flex-wrap: wrap;
  font-size: 11.5px; color: #8d95a1;
}
.tactic .t-arrow { color: #59616d; }
.tactic .t-note { color: #8d95a1; }
.tactic .t-note.dim { color: #59616d; font-style: italic; }

/* ---- v0.5 战斗反馈条 ---- */
.trigger-row {
  display: flex; align-items: center; gap: 8px;
  padding: 6px 12px; border: 1px solid var(--border); border-radius: var(--radius);
  background: var(--panel); color: var(--text-dim); font-size: 12.5px;
  font-variant-numeric: tabular-nums;
}
.trigger-row .tl-label { color: #59616d; font-size: 11.5px; }
.trigger-row .tl-value {
  color: var(--accent); font-weight: 600;
  padding: 1px 9px; border: 1px solid var(--border); border-radius: 8px;
  background: var(--panel);
}
.trigger-row .tl-sep { color: #3a4150; }

/* ---- 双栏 ---- */
.cols { display: flex; gap: 14px; flex-wrap: wrap; align-items: stretch; }
.left { flex: 1 1 380px; min-width: 300px; display: flex; flex-direction: column; gap: 10px; }
.right { flex: 1 1 320px; min-width: 300px; }

/* ---- 战斗文字流 ---- */
.log {
  flex: 1; min-height: 260px; max-height: 380px; overflow-y: auto;
  border: 1px solid var(--border); border-radius: var(--radius);
  background: #12161d; padding: 8px 10px;
  display: flex; flex-direction: column; gap: 4px;
  font-size: 12.5px; font-variant-numeric: tabular-nums;
}
.ev { display: flex; gap: 8px; align-items: baseline; color: #8d95a1; padding: 2px 6px; }
.ev em { color: #59616d; font-style: normal; flex: none; }
.ev .tag { color: #6b7382; font-weight: 500; font-size: 11px; flex: none; }
.ev.hot {
  color: var(--text);
  border-left: 3px solid var(--fire);
  background: rgba(224, 85, 69, 0.08);
  border-radius: 0 var(--radius) var(--radius) 0;
}
.ev.hot .tag { color: var(--fire); }
.ev.hot em { color: #a08580; }

/* ---- LLM 助手 ---- */
.assistant { display: flex; gap: 10px; align-items: flex-start; }
.avatar { position: relative; width: 44px; height: 44px; flex: none; }
.aura { position: absolute; inset: 0; width: 100%; height: 100%; }
.face {
  position: absolute; inset: 0; display: flex;
  align-items: center; justify-content: center; font-size: 20px;
}
.bubble {
  flex: 1; border: 1px solid var(--border); border-radius: var(--radius);
  background: var(--panel); padding: 8px 12px; font-size: 12.5px;
}
.bubble p { margin: 2px 0; line-height: 1.55; }
.bubble .k { font-style: normal; font-weight: 600; margin-right: 8px; font-size: 11.5px; }
.bubble .conf { color: var(--text-dim); }
.bubble .conf b { color: var(--accent); }
.thinking { color: var(--nav-active); }
.waiting { color: var(--text-dim); }

/* ---- 右栏 ---- */
.chips { display: flex; flex-wrap: wrap; gap: 6px; }
.chip {
  background: var(--panel); border-color: #3a4150;
  font-size: 12px; padding: 5px 10px; color: #b9c2cf;
}
.chip:hover:not(:disabled) { border-color: var(--nav-active); color: #fff; }
.hint { color: var(--text-dim); font-size: 12px; margin: 6px 0 0; }
.qa { list-style: none; margin: 0; padding: 0; font-size: 12.5px; }
.qa li { padding: 6px 0; border-bottom: 1px dashed var(--border); }
.qa p { margin: 2px 0; line-height: 1.55; }
.qa .q { color: var(--nav-active); }
.qa .a { color: var(--text); }
h3 { font-size: 12.5px; margin: 14px 0 4px; color: var(--text-dim); font-weight: 600; }
.predict-btns { display: flex; gap: 6px; flex-wrap: wrap; }
.pending { color: var(--accent); font-size: 12.5px; margin: 8px 0 0; }

.empty { color: #59616d; font-size: 12.5px; margin: 4px 0; }
.wood-c { color: var(--wood); }
.fire-c { color: var(--fire); }
.accent-c { color: var(--accent); }
</style>
