<script setup lang="ts">
// src/cult/CultSelf.vue —— 屏五 · 本尊：方向卡（localStorage 持久）、
// 传音（三级 chips + 文本）、筑基五关定性进度、向分身垂问（ask）。
import { onMounted, ref } from 'vue'
import { cult, guide, ask, newCultSession } from './state'

const GOAL_KEY = 'cult_self_goal'
const goal = ref('')
onMounted(() => {
  try { goal.value = localStorage.getItem(GOAL_KEY) ?? '' } catch { /* 私有浏览态 */ }
})
function saveGoal() {
  try { localStorage.setItem(GOAL_KEY, goal.value) } catch { /* 忽略 */ }
}

const KINDS = [
  { v: 'whisper', w: '轻语' },
  { v: 'guide', w: '引导' },
  { v: 'deeplink', w: '深谈' },
]
const gkind = ref('guide')
const gtext = ref('')
function sendGuide() {
  const t = gtext.value.trim()
  if (!t) return
  gtext.value = ''
  void guide(gkind.value, t)
}

const question = ref('')
function doAsk() {
  const q = question.value.trim()
  if (!q) return
  question.value = ''
  void ask(q)
}

// 五关进度条宽（band 0-4 → 15%~95%，零真值）。
const bandWidth = (b: number) => 15 + b * 20
const barColor = (b: number) => (b >= 3 ? 'var(--jade)' : b >= 1 ? 'var(--gold)' : 'rgba(42,57,65,.9)')
</script>

<template>
  <template v-if="cult.view">
    <!-- 方向卡 -->
    <div class="c-card c-goal">
      <span class="gk">当前方向 · 本尊所定</span>
      <textarea v-model="goal" @change="saveGoal" placeholder="例如：先立一门稳妥吐纳，再探火性转化——不求快，求每一次实验都有结论。"></textarea>
    </div>

    <!-- 传音 -->
    <div class="c-card c-guide">
      <div class="t">传音</div>
      <div class="chips">
        <button v-for="k in KINDS" :key="k.v" class="chip" :class="{ on: gkind === k.v }" @click="gkind = k.v">{{ k.w }}</button>
      </div>
      <div class="inp">
        <input v-model="gtext" @keydown.enter="sendGuide" placeholder="例如：金门似有滞涩，先查明再动" />
        <button class="send" :disabled="cult.busy" @click="sendGuide">送 ↗</button>
      </div>
      <div class="ui" style="font-size:9px;color:var(--mist);margin-top:5px">
        轻语随口一提，引导入分身下次征询，深谈记入感知流。</div>
    </div>

    <!-- 垂问 -->
    <div class="c-card c-guide">
      <div class="t">垂问 · 向分身问一句</div>
      <div class="inp">
        <input v-model="question" @keydown.enter="doAsk" placeholder="今日身势如何？" />
        <button class="send" :disabled="cult.busy || !cult.view.llm_available" @click="doAsk">问 ↗</button>
      </div>
      <div v-if="cult.askAnswer" class="c-voice" style="margin-top:7px">
        <div class="vt">分身答</div>{{ cult.askAnswer }}
      </div>
      <div v-if="cult.askDegraded" class="ui" style="font-size:9.5px;color:var(--mist);margin-top:5px">{{ cult.askDegraded }}</div>
    </div>

    <!-- 筑基五关 -->
    <div class="c-card c-five">
      <div class="t">筑基之路 · 五关</div>
      <div class="frow" v-for="f in cult.view.foundation" :key="f.key">
        <span class="fk">{{ f.key }}</span>
        <div class="fbar"><i :style="{ width: bandWidth(f.band) + '%', background: barColor(f.band) }"></i></div>
        <span class="fv">{{ f.word }}</span>
      </div>
      <div class="ui" style="font-size:9px;color:var(--mist);margin-top:6px">
        境界 = 引擎里的新稳定结构，不是经验条：{{ cult.view.realm }}。</div>
    </div>

    <!-- 分身簿记 -->
    <div class="c-card">
      <div class="t">静室</div>
      <div class="ui" style="font-size:10px;color:var(--mist);line-height:1.8">
        神识{{ cult.view.acuity === 1 ? '一层（初见）' : cult.view.acuity === 2 ? '二层' : '三层（熟练）' }} ·
        分身{{ cult.view.pilot_kind === 'llm' ? (cult.view.llm_available ? '已接灵机' : '灵机未接') : '循脚本而行' }} ·
        已历 {{ cult.view.tick }} 息
        <br /><button class="c-start" :disabled="cult.busy" @click="newCultSession('script')">重开静室（新会话）</button>
      </div>
    </div>
  </template>
</template>
