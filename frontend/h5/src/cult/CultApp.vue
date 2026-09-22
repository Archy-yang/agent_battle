<script setup lang="ts">
// src/cult/CultApp.vue —— 修炼模式外壳：开局仪式 + 单页洞府（顶栏 / 身体场景 /
// 感知流 / 心念卡 / 传念 / 知识台账 / 底部阶段行）。设计稿：docs/design/yiqi-dongfu-redesign.html。
// 修炼模式接管整个视口（手机全屏），通过「战斗」按钮退出（App.vue 的 mode 机制零改动）。
import { computed, ref } from 'vue'
import { cult, act, awaken } from './state'
import CultFu from './CultFu.vue'

const emit = defineEmits<{ exit: [] }>()

// —— 开局仪式 ——
const goal = ref('')
const useScript = ref(false) // 演示入口：勾选则以脚本分身入定（不耗调用）
const gated = computed(() => cult.gate && !cult.view)

function doAwaken() {
  void awaken(useScript.value ? 'script' : 'llm', goal.value)
}

const realm = computed(() => cult.view?.realm ?? '')

// 底部阶段行：定性描述当前所在阶段（零数字、零机器词）
const modeText = computed(() => {
  const v = cult.view
  if (!v) return { cur: '初醒', sub: '静室之中，分身尚未睁眼' }
  if (v.active) return { cur: '周天运转', sub: `依《${v.active}》行功，异常方唤心神` }
  if (v.points.some((p) => p.flow !== '停滞')) return { cur: '吐纳中', sub: '气息在身内流动，随时可止' }
  return { cur: '暗中摸索', sub: '分身正在辨认周身可承气之处' }
})

function doHold() {
  void act([{ kind: 'sense' }])
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

    <!-- 入定后：单页洞府 -->
    <template v-else>
      <div class="cu-top">
        <h1>一炁周天</h1>
        <span class="realm"><b>{{ realm }}</b></span>
        <button class="exit" @click="emit('exit')">‹ 战斗</button>
      </div>
      <div class="cu-err" v-if="cult.error">{{ cult.error }}</div>

      <div class="cu-scroll">
        <div v-if="!cult.view" class="cu-loading">正在静室中唤醒分身……</div>
        <CultFu v-else />
      </div>

      <div class="cu-footer">
        <div class="modeText">当前：<b>{{ modeText.cur }}</b><br />{{ modeText.sub }}</div>
        <button class="hold" :disabled="cult.busy" @click="doHold">凝神</button>
      </div>
    </template>
  </div>
</template>
