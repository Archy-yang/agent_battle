<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import {
  ensureMatch, influenceText, view,
  watchCombat, battleNavRequest,
} from './state'
import MapView from './views/MapView.vue'
import BattleView from './views/BattleView.vue'
import GearView from './views/GearView.vue'
import StatsView from './views/StatsView.vue'
import ReplayView from './views/ReplayView.vue'
import CultApp from './cult/CultApp.vue'
import './cult/cult.css'

// 模式切换（WP-5）：战斗 = 旧五页签（零改动）；修炼 = 新五屏（src/cult/）。
type Mode = 'battle' | 'cult'
const mode = ref<Mode>('battle')

type Page = 'map' | 'battle' | 'gear' | 'stats' | 'replay'
const page = ref<Page>('map')

const PAGES: { key: Page; label: string }[] = [
  { key: 'map', label: '地图' },
  { key: 'battle', label: '对战' },
  { key: 'gear', label: '装备' },
  { key: 'stats', label: '属性' },
  { key: 'replay', label: '回放' },
]

// 对局初始化放在外壳：外壳不随 Tab 切换卸载，切页返回不丢局。
onMounted(ensureMatch)

// M3.4 遭遇自动切换：in_combat 变 true 时自动进对战页（只切一次，直到
// in_combat 变 false 重置武装——见 state.ts watchCombat）；用户手动返回
// 地图后，再次遭遇会再次切换。battleNavRequest 也被地图页「前往对战」按钮复用。
watchCombat()
watch(battleNavRequest, () => { page.value = 'battle'; mode.value = 'battle' })
</script>

<template>
  <div class="wrap">
    <header>
      <!-- pack 名始终读全局 view（响应式）：新对局切换场景包后标题跟随 -->
      <h1>agent_battle <small>{{ mode === 'cult' ? '一炁周天' : (view?.pack ?? '未开局') }}</small></h1>
      <nav class="nav">
        <!-- 模式切换：战斗 | 修炼 -->
        <button :class="{ active: mode === 'battle' }" @click="mode = 'battle'">战斗</button>
        <button :class="{ active: mode === 'cult' }" @click="mode = 'cult'">修炼</button>
        <template v-if="mode === 'battle'">
          <button
            v-for="p in PAGES" :key="p.key"
            :class="{ active: page === p.key }"
            @click="page = p.key"
          >{{ p.label }}</button>
        </template>
      </nav>
      <div class="bar" v-if="mode === 'battle' && view">
        <span>Turn {{ view.turn }}/{{ view.turn_limit }}</span>
        <span class="influence">⚡ {{ influenceText }}</span>
        <span v-if="view.pilot" class="badge" :class="'pilot-' + view.pilot">{{ view.pilot }}</span>
        <span :class="{ warn: (view.observation?.self.heat ?? 0) > 18 }">
          热 {{ view.observation?.self.heat ?? '—' }}
        </span>
        <span>形 {{ view.observation?.self.integrity ?? '—' }}</span>
        <span>气 {{ view.observation?.self.battery ?? '—' }}</span>
        <span>状态 {{ view.status }}</span>
        <span class="seed">seed {{ view.seed }}</span>
      </div>
    </header>

    <!-- 修炼模式：五屏（府/视/悟/诀/尊），自带外壳与导航 -->
    <CultApp v-if="mode === 'cult'" />

    <main class="page" v-else>
      <MapView v-if="page === 'map'" />
      <BattleView v-else-if="page === 'battle'" />
      <GearView v-else-if="page === 'gear'" />
      <StatsView v-else-if="page === 'stats'" />
      <ReplayView v-else />
    </main>
  </div>
</template>

<style>
/* ---- 设计 token（与 design.ts 保持一致） ---- */
:root {
  --bg: #0f1216;
  --panel: #171c24;
  --border: #262e3a;
  --text: #d8dce2;
  --text-dim: #7a8290;
  --wood: #57b26a;
  --fire: #e05545;
  --earth: #c9a44a;
  --metal: #b8c0cc;
  --water: #4a8fd4;
  --accent: #d4a94e;
  --nav-active: #9fd0ff;
  --radius: 6px;
}
* { box-sizing: border-box; }
body {
  margin: 0;
  background: var(--bg);
  color: var(--text);
  font-family: system-ui, -apple-system, 'PingFang SC', sans-serif;
}

/* ---- 外壳 ---- */
.wrap { max-width: 1020px; margin: 0 auto; padding: 16px; }
header h1 { font-size: 18px; margin: 0 0 8px; }
header small { color: var(--text-dim); font-weight: normal; }
.nav {
  display: flex; gap: 4px;
  border-bottom: 1px solid var(--border);
}
.nav button {
  background: transparent; border: none; border-bottom: 2px solid transparent;
  border-radius: 0; color: var(--text-dim); padding: 6px 10px; font-size: 13px;
}
.nav button:hover:not(:disabled) { background: transparent; color: var(--text); }
.nav button.active { color: #fff; border-bottom-color: var(--nav-active); }
.page { margin-top: 14px; }

/* ---- 全局状态栏 ---- */
.bar {
  display: flex; gap: 14px; font-size: 13px; color: #aab2bd;
  flex-wrap: wrap; align-items: center; margin-top: 8px;
  font-variant-numeric: tabular-nums;
}
.bar .warn { color: var(--fire); }
.bar .seed { margin-left: auto; color: #59616d; }
.bar .influence { color: var(--accent); font-weight: 600; }
.badge {
  font-size: 11px; padding: 1px 7px; border-radius: 8px;
  border: 1px solid var(--border); color: var(--nav-active); background: var(--panel);
}
.badge.pilot-scripted { color: #aab2bd; }

/* ---- 通用控件 ---- */
button {
  background: #2a3040; color: var(--text); border: 1px solid #3a4150;
  border-radius: var(--radius); padding: 6px 12px; cursor: pointer; font-size: 13px;
}
button:hover:not(:disabled) { background: #35405a; }
button:disabled { opacity: 0.4; cursor: default; }
button.danger { border-color: #5a3a3a; color: var(--fire); }
button.danger:hover:not(:disabled) { background: #45303a; }
/* 统一错误提示：所有页面的内联红字（guideError/askError/predictError/equipError/error 等） */
.err { color: var(--fire); font-size: 13px; margin: 4px 0 0; }

/* ---- 面板通用 ---- */
h2 { font-size: 14px; margin: 18px 0 6px; color: var(--nav-active); }
h2 small { color: #59616d; font-weight: normal; }
dl { margin: 0; font-size: 13px; }
dt { color: var(--text-dim); margin-top: 8px; }
dd { margin: 2px 0 0; line-height: 1.5; }
ul.list {
  list-style: none; margin: 0; padding: 0; font-size: 12.5px; color: #aab2bd;
}
ul.list li { padding: 3px 0; border-bottom: 1px dashed var(--border); }
ul.list em { color: #59616d; font-style: normal; margin-right: 6px; }
ul.list b.hit { color: var(--wood); font-weight: 600; }
ul.list b.miss { color: var(--fire); font-weight: 600; }
.guide { display: flex; gap: 6px; }
.guide select, .guide input {
  background: var(--panel); color: var(--text); border: 1px solid #3a4150;
  border-radius: var(--radius); padding: 6px 8px; font-size: 13px;
}
.guide input { flex: 1; }
input[type="text"] { font-variant-numeric: tabular-nums; }

/* ---- 占位页 ---- */
.placeholder {
  padding: 24px 0; color: var(--text-dim); font-size: 13px;
}
.placeholder h2 { color: var(--text); margin-top: 0; }
</style>
