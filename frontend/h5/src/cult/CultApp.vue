<script setup lang="ts">
// src/cult/CultApp.vue —— 修炼模式外壳：开局仪式 + 行动台式一屏
// （顶栏 / 身体场景 / 感知条 / 行动区 / 底部行，整页零滚动）。
// 设计稿：docs/specs/2026-10-18-dongfu-action-desk-redesign.md。
// 修炼模式接管整个视口（手机全屏），通过「战斗」按钮退出（App.vue 的 mode 机制零改动）。
// 顶栏 ⟲ = 玩法说明入口：重看新手引导（含玩法总述）；首次入定后自动弹一次。
import { computed, nextTick, ref, watch } from 'vue'
import { cult, awaken } from './state'
import CultFu from './CultFu.vue'
import CultTour from './CultTour.vue'

const emit = defineEmits<{ exit: [] }>()

// —— 开局仪式 ——
const goal = ref('')
const useScript = ref(false) // 演示入口：勾选则以内景演示入定（不耗调用）
const gated = computed(() => cult.gate && !cult.view)

function doAwaken() {
  void awaken(useScript.value ? 'script' : 'llm', goal.value)
}

const realm = computed(() => cult.view?.realm ?? '')

// —— 新手引导 ——
const tourOpen = ref(false)
function openTour() {
  tourOpen.value = true
}
// 首次入定后：未完成过引导则自动弹一次；localStorage 不可用（私有
// 浏览态）时按已看过处理——静默不弹，不报错。
watch(gated, (g, old) => {
  if (g || old === false) return
  let done = true
  try { done = localStorage.getItem('cult_tour_done') === '1' } catch { /* 静默降级 */ }
  if (!done) {
    void nextTick(() => { window.setTimeout(() => { tourOpen.value = true }, 500) })
  }
})
</script>

<template>
  <div class="cult-root">
    <!-- 开局仪式：第一人称入定（pilot 选择不是玩家语义，收敛为一颗按钮 + 演示入口） -->
    <div class="c-gate" v-if="gated">
      <div class="seal" aria-hidden="true">炁</div>
      <h2 class="kai">一炁周天</h2>
      <p class="sub ui">你沉入静室。这具身体于你陌生——经脉未明，窍穴难辨，唯有靠感受一点点摸清。</p>
      <p class="sub ui">写下此刻的心愿与分寸；余下的，交给时间与身体。</p>
      <textarea
        v-model="goal"
        rows="3"
        placeholder="写下方向（可留白）：如「先立一门稳妥吐纳，再探火性」"
      ></textarea>
      <button class="awaken" :disabled="cult.awakening" @click="doAwaken">
        {{ cult.awakening ? '入定中……' : '入定' }}
      </button>
      <label class="demo ui">
        <input type="checkbox" v-model="useScript" /> 演示之身（不耗灵机）
      </label>
      <p class="err ui" v-if="cult.error">{{ cult.error }}</p>
    </div>

    <!-- 入定后：行动台式一屏（五段 flex，总高恒等于视口，零滚动） -->
    <template v-else>
      <div class="cu-top">
        <h1>一炁周天</h1>
        <span class="realm"><b>{{ realm }}</b></span>
        <button class="tour ui" aria-label="玩法说明" @click="openTour">⟲</button>
        <button class="exit" @click="emit('exit')">‹ 战斗</button>
      </div>
      <div class="cu-err" v-if="cult.error">{{ cult.error }}</div>

      <div class="cu-main">
        <div v-if="!cult.view" class="cu-loading">正在静室中入定……</div>
        <CultFu v-else />
      </div>

      <!-- 新手引导：首次入定自动弹，⟲ 随时重看 -->
      <CultTour :open="tourOpen" @close="tourOpen = false" />
    </template>
  </div>
</template>
