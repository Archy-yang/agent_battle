<script setup lang="ts">
// 属性页：角色属性（形/气/热 + 本源 + 移动订单）与 Agent 养成档案。
// 档案数据由本页自行 fetch（GET /api/profile），不进全局 state。
import { computed, onMounted, ref } from 'vue'
import { api, essenceColor, essenceName, type Profile, type StateExt } from '../api'
import { view } from '../state'

// ---- 角色属性 ----
const self = computed(() => view.value?.observation?.self ?? null)
const HEAT_MAX = 25
const heatColor = computed(() => ((self.value?.heat ?? 0) > 18 ? 'var(--fire)' : 'var(--earth)'))

const order = computed(() => (view.value as StateExt | null)?.order ?? null)

const meters = computed(() => {
  const s = self.value
  if (!s) return []
  return [
    { label: '形', value: s.integrity, max: s.max_integrity, color: 'var(--wood)' },
    { label: '气', value: s.battery, max: s.max_battery, color: 'var(--metal)' },
    { label: '热', value: s.heat, max: HEAT_MAX, color: heatColor.value },
  ]
})

// ---- 养成档案 ----
const profile = ref<Profile | null>(null)
const loadError = ref('')
const loading = ref(false)

// 本地可编辑副本
const sliders = ref({ adventure: 50, cooperation: 50, exploration: 50 })
const personaDraft = ref('')
const personaTouched = ref(false)

const SLIDER_DEFS: { key: keyof typeof sliders.value; label: string }[] = [
  { key: 'adventure', label: '冒险 ↔ 谨慎' },
  { key: 'cooperation', label: '合作 ↔ 竞争' },
  { key: 'exploration', label: '探索 ↔ 利用' },
]

const levelText = (v: number) => (v >= 67 ? '高' : v <= 33 ? '低' : '中')

async function loadProfile() {
  loading.value = true
  loadError.value = ''
  try {
    const p = await api<Profile>('/api/profile')
    profile.value = p
    const s = p.sliders ?? { adventure: 0.5, cooperation: 0.5, exploration: 0.5 }
    sliders.value = {
      adventure: Math.round((s.adventure ?? 0.5) * 100),
      cooperation: Math.round((s.cooperation ?? 0.5) * 100),
      exploration: Math.round((s.exploration ?? 0.5) * 100),
    }
    // persona 草稿只在未编辑过时随回读刷新，避免覆盖正在输入的内容
    if (!personaTouched.value) personaDraft.value = p.persona ?? ''
  } catch (e) {
    loadError.value = e instanceof Error ? e.message : String(e)
  } finally {
    loading.value = false
  }
}
onMounted(loadProfile)

const savingSliders = ref(false)
const slidersError = ref('')
async function saveSliders() {
  savingSliders.value = true
  slidersError.value = ''
  try {
    await api('/api/profile', {
      sliders: {
        adventure: sliders.value.adventure / 100,
        cooperation: sliders.value.cooperation / 100,
        exploration: sliders.value.exploration / 100,
      },
    })
    await loadProfile() // 成功：静默回读刷新
  } catch (e) {
    slidersError.value = e instanceof Error ? e.message : String(e)
  } finally {
    savingSliders.value = false
  }
}

const savingLesson = ref(-1)
const lessonError = ref('')
async function toggleLesson(i: number) {
  const cards = profile.value?.lessons ?? []
  if (!cards[i]) return
  savingLesson.value = i
  lessonError.value = ''
  try {
    await api('/api/profile', { lesson_toggle: { index: i, enabled: !cards[i].enabled } })
    await loadProfile()
  } catch (e) {
    lessonError.value = e instanceof Error ? e.message : String(e)
  } finally {
    savingLesson.value = -1
  }
}

const savingPersona = ref(false)
const personaError = ref('')
async function savePersona() {
  savingPersona.value = true
  personaError.value = ''
  try {
    await api('/api/profile', { persona: personaDraft.value })
    personaTouched.value = false
    await loadProfile()
  } catch (e) {
    personaError.value = e instanceof Error ? e.message : String(e)
  } finally {
    savingPersona.value = false
  }
}
</script>

<template>
  <div class="stats">
    <!-- 上栏：角色属性 -->
    <section class="card">
      <h2>角色属性</h2>
      <template v-if="self">
        <div class="meter" v-for="m in meters" :key="m.label">
          <span class="m-label">{{ m.label }}</span>
          <span class="m-track">
            <span class="m-fill" :style="{ width: Math.min(100, (m.value / Math.max(1, m.max)) * 100) + '%', background: m.color }"></span>
          </span>
          <span class="m-value">{{ m.value }}/{{ m.max }}</span>
        </div>
        <div class="meta">
          <span class="badge-essence" :style="{ borderColor: essenceColor(self.essence), color: essenceColor(self.essence) }">
            本源 {{ essenceName(self.essence) }}
          </span>
          <span class="order">移动订单：{{ order ? `→ (${order.x}, ${order.y})` : '无' }}</span>
        </div>
      </template>
      <p v-else class="empty">{{ view ? '暂无观测数据。' : '尚未进入对局 —— 请先到地图页开始。' }}</p>
    </section>

    <!-- 下栏：Agent 养成档案 -->
    <section class="card">
      <h2>Agent 养成档案 <small>养成的不是数值，是它的决策方式</small></h2>

      <p v-if="loadError" class="err">档案读取失败：{{ loadError }}</p>
      <p v-else-if="loading && !profile" class="empty">读取档案中…</p>

      <template v-if="profile">
        <!-- 人格滑块 -->
        <div class="slider-row" v-for="d in SLIDER_DEFS" :key="d.key">
          <span class="s-label">{{ d.label }}</span>
          <input
            type="range" min="0" max="100" step="1"
            v-model.number="sliders[d.key]"
          />
          <span class="s-preview">{{ levelText(sliders[d.key]) }}</span>
        </div>
        <div class="save-row">
          <button @click="saveSliders" :disabled="savingSliders">
            {{ savingSliders ? '保存中…' : '保存人格' }}
          </button>
          <small v-if="!slidersError" class="hint">拖动滑块调整决策倾向</small>
        </div>
        <p v-if="slidersError" class="err">{{ slidersError }}</p>

        <!-- 经验卡 -->
        <h3>经验卡</h3>
        <ul class="list lessons" v-if="profile.lessons?.length">
          <li
            v-for="(l, i) in profile.lessons" :key="i"
            class="lesson" :class="{ off: !l.enabled }"
          >
            <span class="l-text">{{ l.text }}</span>
            <button class="l-toggle" @click="toggleLesson(i)" :disabled="savingLesson === i">
              {{ l.enabled ? '停用' : '启用' }}
            </button>
          </li>
        </ul>
        <p v-else class="empty">暂无经验卡 —— 对局结束后的复盘会沉淀到这里。</p>
        <p v-if="lessonError" class="err">{{ lessonError }}</p>

        <!-- 人格补充 -->
        <h3>人格补充</h3>
        <textarea
          v-model="personaDraft"
          rows="3"
          placeholder="用几句话补充它的人格（可选）"
          @input="personaTouched = true"
        ></textarea>
        <div class="save-row">
          <button @click="savePersona" :disabled="savingPersona">
            {{ savingPersona ? '保存中…' : '保存补充' }}
          </button>
          <small v-if="profile.updated" class="hint">上次更新 {{ new Date(profile.updated).toLocaleString() }}</small>
        </div>
        <p v-if="personaError" class="err">{{ personaError }}</p>
      </template>
      <p v-else-if="!loading && !loadError" class="empty">档案为空。</p>
    </section>
  </div>
</template>

<style scoped>
.stats { max-width: 640px; display: flex; flex-direction: column; gap: 18px; }
.card {
  border: 1px solid var(--border); border-radius: var(--radius);
  background: var(--panel); padding: 12px 14px;
}
.card h2 { margin-top: 0; }
.empty { color: var(--text-dim); font-size: 13px; margin: 6px 0; }

/* 仪表条 */
.meter { display: flex; align-items: center; gap: 10px; margin: 8px 0; }
.m-label { width: 1.2em; color: var(--text-dim); font-size: 13px; }
.m-track {
  flex: 1; height: 10px; border-radius: 5px;
  background: #10141a; border: 1px solid var(--border);
  overflow: hidden; display: block;
}
.m-fill { display: block; height: 100%; border-radius: 5px; }
.m-value { color: var(--text-dim); font-size: 12px; min-width: 4.5em; text-align: right; font-variant-numeric: tabular-nums; }
.meta { display: flex; align-items: center; gap: 14px; margin-top: 10px; flex-wrap: wrap; }
.badge-essence {
  font-size: 12px; padding: 2px 9px; border-radius: 8px;
  border: 1px solid; background: transparent;
}
.order { color: var(--text-dim); font-size: 12.5px; }

/* 滑块 */
.slider-row { display: flex; align-items: center; gap: 10px; margin: 10px 0; }
.s-label { width: 7.5em; color: var(--text-dim); font-size: 13px; flex: 0 0 auto; }
.slider-row input[type="range"] { flex: 1; accent-color: var(--accent); }
.s-preview { width: 2em; text-align: center; font-size: 12.5px; color: var(--accent); flex: 0 0 auto; }
.save-row { display: flex; align-items: center; gap: 12px; margin-top: 6px; }
.hint { color: #59616d; font-size: 12px; }
h3 { font-size: 13px; margin: 18px 0 6px; color: var(--text); }

/* 经验卡 */
.lessons li {
  display: flex; align-items: center; gap: 10px;
  padding: 7px 0; border-bottom: 1px dashed var(--border);
}
.lessons li:last-child { border-bottom: none; }
.l-text { flex: 1; font-size: 13px; line-height: 1.55; }
.lesson.off .l-text { color: #59616d; text-decoration: line-through; }
.l-toggle { flex: 0 0 auto; font-size: 12px; padding: 3px 10px; }

textarea {
  width: 100%; background: var(--panel); color: var(--text);
  border: 1px solid #3a4150; border-radius: var(--radius);
  padding: 8px; font-size: 13px; font-family: inherit; resize: vertical;
}
</style>
