<script setup lang="ts">
// 装备页：已装组件 / 仓库物料 / 组装器（Blueprint）/ 装配结果反馈。
// 组件与物料数据来自全局 view（state.ts），组装通过 tick intent 发送。
import { computed, ref } from 'vue'
import {
  api,
  type BlueprintSpec, type ComponentView, type SelfExt, type StateExt, type StateView,
} from '../api'
import { view, busy, sendEquip, equipBusy, equipError, equipLog } from '../state'

// ---- 五行中文名 → 颜色 / 序号（components/materials 里是中文字符串） ----
const WUXING = ['木', '火', '土', '金', '水'] as const
const WUXING_COLORS: Record<string, string> = {
  木: '#57b26a', 火: '#e05545', 土: '#c9a44a', 金: '#b8c0cc', 水: '#4a8fd4',
}
const wuxingColor = (n: string) => WUXING_COLORS[n] ?? '#888'
const wuxingIndex = (n: string) => WUXING.indexOf(n as typeof WUXING[number])

// ---- 已装组件 ----
const components = computed<ComponentView[]>(() => {
  const v = view.value as StateExt | null
  return ((v?.observation?.self ?? null) as SelfExt | null)?.components ?? []
})
const canToggle = (type: string) => type === '感识' || type === '离火'

// ---- 仓库物料 ----
const materials = computed<[string, number][]>(() => {
  const m = (view.value as StateExt | null)?.materials ?? {}
  return Object.entries(m)
    .filter(([, n]) => n > 0)
    .sort((a, b) => {
      const ia = wuxingIndex(a[0]); const ib = wuxingIndex(b[0])
      return (ia < 0 ? 9 : ia) - (ib < 0 ? 9 : ib)
    })
})

// ---- 组装器 ----
// 类型映射与 engine.ComponentType 一致：躯壳0/储气1/灵机2/感识3/识海4/离火5/仓廪6/清源7
const COMP_TYPES = ['躯壳', '储气', '灵机', '感识', '识海', '离火', '仓廪', '清源'] as const
const COMP_INDEX: Record<string, number> = Object.fromEntries(COMP_TYPES.map((t, i) => [t, i]))
// essence -1 表示不选材质（引擎按组件默认本源）
const rows = ref([
  { type: '躯壳', essence: -1 },
  { type: '储气', essence: -1 },
  { type: '灵机', essence: -1 },
])

const assembleBusy = ref(false)
const assembleError = ref('')
const assembleLog = ref<{ tick: number; text: string; rejected: boolean }[]>([])

async function assemble() {
  if (!view.value || assembleBusy.value) return
  assembleBusy.value = true
  assembleError.value = ''
  try {
    const blueprint: BlueprintSpec[] = rows.value.map(r => {
      const spec: BlueprintSpec = { type: COMP_INDEX[r.type] ?? 0 }
      if (r.essence >= 0) spec.essence = r.essence
      return spec
    })
    const next = await api<StateView>(`/api/match/${view.value.id}/tick`, {
      intents: [{ type: 'assemble', blueprint }],
    })
    view.value = next
    // 组装结果在返回 state 的 events 尾部：成功（assemble）或 intent_rejected 都展示
    assembleLog.value = next.events
      .filter(e => e.type === 'assemble' || e.type === 'intent_rejected')
      .slice(-5)
      .map(e => ({ tick: e.tick, text: e.text, rejected: e.type === 'intent_rejected' }))
  } catch (e) {
    assembleError.value = e instanceof Error ? e.message : String(e)
  } finally {
    assembleBusy.value = false
  }
}

// ---- 装备自身（v0.5 WPC）----
// equip 可装组件：灵机/感识/识海/离火/仓廪/清源。躯壳(0)/储气(1) 是
// 身体之根，后端会拒绝，不进下拉。
const EQUIP_TYPES = ['灵机', '感识', '识海', '离火', '仓廪', '清源'] as const
const equipType = ref<(typeof EQUIP_TYPES)[number]>('灵机')
// 材质：-1 不发 essence（引擎取组件默认本源），0-4 与 BlueprintSpec.essence 对齐
const equipEssence = ref(-1)

// 当前身体组件数（复杂度预算 ≤8，后端 MaxBodyComponents）
const compCount = computed(() => components.length)

async function install() {
  const spec: BlueprintSpec = { type: COMP_INDEX[equipType.value] ?? 0 }
  if (equipEssence.value >= 0) spec.essence = equipEssence.value
  await sendEquip([spec])
}
</script>

<template>
  <div class="gear">
    <!-- 已装组件 -->
    <section>
      <h2>已装组件 <small>{{ components.length ? components.length + ' 件' : '' }}</small></h2>
      <ul class="list comp-list" v-if="components.length">
        <li v-for="c in components" :key="c.id" class="comp-row">
          <span class="cube" :style="{ background: wuxingColor(c.essence) }"></span>
          <b class="comp-name">{{ c.type }}</b>
          <span class="dot" :style="{ background: wuxingColor(c.essence) }"></span>
          <span class="essence">{{ c.essence }}</span>
          <span v-if="canToggle(c.type)" class="state" :class="{ on: c.active }">
            {{ c.active ? '开' : '关' }}
          </span>
          <span v-else class="state dim">{{ c.active ? '激活' : '未激活' }}</span>
        </li>
      </ul>
      <p v-else class="empty">暂无已装组件 —— 可在下方组装。</p>
    </section>

    <!-- 仓库物料 -->
    <section>
      <h2>仓库物料</h2>
      <div class="mats" v-if="materials.length">
        <span v-for="[name, n] in materials" :key="name" class="mat">
          <span class="dot" :style="{ background: wuxingColor(name) }"></span>
          {{ name }} <b>×{{ n }}</b>
        </span>
      </div>
      <p v-else class="empty">仓库为空。</p>
    </section>

    <!-- 组装器 -->
    <section>
      <h2>组装 Blueprint</h2>
      <p class="note">
        选择组件与材质，由引擎校验并消耗物料。
        <b>材质即天敌</b>：金怕火、木怕金、土怕木穿、火怕水、水怕土堰。
      </p>
      <div class="bp-row" v-for="(r, i) in rows" :key="i">
        <span class="bp-label">{{ i + 1 }}</span>
        <select v-model="r.type">
          <option v-for="t in COMP_TYPES" :key="t" :value="t">{{ t }}</option>
        </select>
        <select v-model.number="r.essence">
          <option :value="-1">默认本源</option>
          <option v-for="(w, wi) in WUXING" :key="w" :value="wi">{{ w }}</option>
        </select>
      </div>
      <button class="assemble" @click="assemble" :disabled="assembleBusy || busy || !view">
        发送组装
      </button>
      <p v-if="assembleError" class="sub-err">{{ assembleError }}</p>
    </section>

    <!-- 装备自身（v0.5：equip 意图，把组件装进自己） -->
    <section>
      <h2>装备自身 <small>身体组件 {{ compCount }}/8（复杂度预算）</small></h2>
      <p class="note">躯壳与储气是身体之根，不可安装；材质即天敌。</p>
      <div class="bp-row">
        <select v-model="equipType">
          <option v-for="t in EQUIP_TYPES" :key="t" :value="t">{{ t }}</option>
        </select>
        <select v-model.number="equipEssence">
          <option :value="-1">默认材质</option>
          <option v-for="(w, wi) in WUXING" :key="w" :value="wi">{{ w }}</option>
        </select>
        <button
          class="install"
          @click="install"
          :disabled="equipBusy || busy || !view || view.status !== 'running'"
        >安装</button>
      </div>
      <p v-if="equipError" class="sub-err">{{ equipError }}</p>
      <ul class="list" v-if="equipLog.length">
        <li v-for="(e, i) in equipLog.slice().reverse()" :key="'eq' + i">
          <em>T{{ e.tick }}</em>
          <span :class="e.rejected ? 'reject' : 'ok'">{{ e.text }}</span>
        </li>
      </ul>
      <p v-else class="empty">尚未安装组件。</p>
    </section>

    <!-- 装配结果反馈 -->
    <section>
      <h2>装配结果</h2>
      <ul class="list" v-if="assembleLog.length">
        <li v-for="(e, i) in assembleLog.slice().reverse()" :key="i">
          <em>T{{ e.tick }}</em>
          <span :class="e.rejected ? 'reject' : 'ok'">{{ e.text }}</span>
        </li>
      </ul>
      <p v-else class="empty">尚未发送组装。</p>
    </section>
  </div>
</template>

<style scoped>
.gear { max-width: 560px; }
.empty { color: var(--text-dim); font-size: 13px; margin: 4px 0; }
.comp-row { display: flex; align-items: center; gap: 8px; padding: 5px 0; }
.comp-row b.comp-name { color: var(--text); font-weight: 600; min-width: 3em; }
.cube, .dot {
  display: inline-block; width: 8px; height: 8px;
  border-radius: 2px; flex: 0 0 auto;
}
.dot { border-radius: 50%; }
.essence { color: var(--text-dim); }
.state { margin-left: auto; font-size: 12px; color: var(--text-dim); }
.state.on { color: var(--wood); }
.state.dim { color: #59616d; }
.mats { display: flex; gap: 8px; flex-wrap: wrap; }
.mat {
  display: inline-flex; align-items: center; gap: 6px;
  border: 1px solid var(--border); border-radius: var(--radius);
  background: var(--panel); padding: 4px 10px; font-size: 13px;
  font-variant-numeric: tabular-nums;
}
.mat b { color: var(--text); font-weight: 600; }
.note {
  color: var(--text-dim); font-size: 12.5px; line-height: 1.6;
  margin: 4px 0 10px;
}
.note b { color: var(--accent); }
.bp-row { display: flex; align-items: center; gap: 8px; margin-bottom: 6px; }
.bp-row select {
  background: var(--panel); color: var(--text); border: 1px solid #3a4150;
  border-radius: var(--radius); padding: 5px 8px; font-size: 13px;
}
.bp-label { color: #59616d; font-size: 12px; width: 14px; text-align: right; }
.assemble { margin-top: 6px; }
.install { border-color: var(--wood); color: var(--wood); }
ul.list .reject { color: var(--fire); }
ul.list .ok { color: var(--wood); }
</style>
