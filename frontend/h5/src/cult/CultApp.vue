<script setup lang="ts">
// src/cult/CultApp.vue —— 修炼模式外壳：开局仪式 + 会话头 + 五屏切换 + 底部导航。
// 修炼模式接管整个视口（手机全屏），通过「战斗」按钮退出。
import { computed, ref } from 'vue'
import { cult, awaken } from './state'
import CultHome from './CultHome.vue'
import CultBody from './CultBody.vue'
import CultInsight from './CultInsight.vue'
import CultArts from './CultArts.vue'
import CultSelf from './CultSelf.vue'

const emit = defineEmits<{ exit: [] }>()

type Screen = 'fu' | 'shi' | 'wu' | 'jue' | 'zun'
const screen = ref<Screen>('fu')

const SCREENS: { key: Screen; ic: string; label: string }[] = [
  { key: 'fu', ic: '府', label: '洞府' },
  { key: 'shi', ic: '视', label: '内视' },
  { key: 'wu', ic: '悟', label: '参悟' },
  { key: 'jue', ic: '诀', label: '功法' },
  { key: 'zun', ic: '尊', label: '本尊' },
]

// —— 开局仪式 ——
const goal = ref('')
const useScript = ref(false) // 演示入口：勾选则以脚本分身入定（不耗调用）
const gated = computed(() => cult.gate && !cult.view)

function doAwaken() {
  void awaken(useScript.value ? 'script' : 'llm', goal.value)
}
</script>

<template>
  <div class="cult-root">
    <!-- 开局仪式：本尊入世（pilot 选择不是玩家语义，收敛为一颗按钮 + 演示入口） -->
    <div class="c-gate" v-if="gated">
      <div class="seal" aria-hidden="true">炁</div>
      <h2 class="kai">一炁周天</h2>
      <p class="sub ui">本尊沉睡，分身初醒。它不知修炼法门，只凭身体感受摸索。</p>
      <p class="sub ui">你定方向，断风险；它探索、试错、悟出功法。</p>
      <textarea
        v-model="goal"
        rows="3"
        placeholder="写下方向（可留白）：如「先立一门稳妥吐纳，再探火性」"
      ></textarea>
      <button class="awaken" :disabled="cult.awakening" @click="doAwaken">
        {{ cult.awakening ? '入定中……' : '唤醒分身' }}
      </button>
      <label class="demo ui">
        <input type="checkbox" v-model="useScript" /> 脚本分身演示（不耗灵机）
      </label>
      <p class="err ui" v-if="cult.error">{{ cult.error }}</p>
    </div>

    <!-- 入定后：会话头 + 五屏 -->
    <template v-else>
      <div class="c-status ui">
        <button class="back" @click="emit('exit')">‹ 战斗</button>
        <span>一炁周天</span>
        <span>{{ cult.view ? `${cult.view.realm} · 第 ${cult.view.tick} 息` : '初醒…' }}</span>
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
      <div class="c-err ui" v-if="cult.error && !cult.gate">{{ cult.error }}</div>

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
    </template>
  </div>
</template>
