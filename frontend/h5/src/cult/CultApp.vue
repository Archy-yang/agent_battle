<script setup lang="ts">
// src/cult/CultApp.vue —— 修炼模式外壳：会话头 + 五屏切换 + 底部导航。
import { onMounted, ref } from 'vue'
import { cult, ensureCultSession } from './state'
import CultHome from './CultHome.vue'
import CultBody from './CultBody.vue'
import CultInsight from './CultInsight.vue'
import CultArts from './CultArts.vue'
import CultSelf from './CultSelf.vue'

type Screen = 'fu' | 'shi' | 'wu' | 'jue' | 'zun'
const screen = ref<Screen>('fu')

const SCREENS: { key: Screen; ic: string; label: string }[] = [
  { key: 'fu', ic: '府', label: '洞府' },
  { key: 'shi', ic: '视', label: '内视' },
  { key: 'wu', ic: '悟', label: '参悟' },
  { key: 'jue', ic: '诀', label: '功法' },
  { key: 'zun', ic: '尊', label: '本尊' },
]

onMounted(() => { void ensureCultSession() })
</script>

<template>
  <div class="cult-root">
    <div class="c-status ui">
      <span>一炁周天</span>
      <span v-if="cult.view">{{ cult.view.realm }} · 第 {{ cult.view.tick }} 息</span>
    </div>
    <div class="c-head">
      <div class="who">
        <div class="avatar">炁</div>
        <div>
          <div class="nm">分身</div>
          <div class="rl">
            {{ cult.view ? `${cult.view.realm} · ${cult.view.pilot_kind === 'llm' ? '灵机分身' : '脚本分身'}` : '初醒…' }}
          </div>
        </div>
      </div>
      <div class="mini">seed {{ cult.view?.seed ?? '—' }}</div>
    </div>
    <div class="c-err ui" v-if="cult.error">{{ cult.error }}</div>

    <div class="c-body">
      <div v-if="!cult.view" class="c-card ui">正在静室中唤醒分身……</div>
      <CultHome v-else-if="screen === 'fu'" />
      <CultBody v-else-if="screen === 'shi'" />
      <CultInsight v-else-if="screen === 'wu'" />
      <CultArts v-else-if="screen === 'jue'" />
      <CultSelf v-else />
    </div>

    <nav class="c-nav">
      <button v-for="s in SCREENS" :key="s.key" class="it" :class="{ on: screen === s.key }" @click="screen = s.key">
        <span class="ic">{{ s.ic }}</span>{{ s.label }}
      </button>
    </nav>
  </div>
</template>
