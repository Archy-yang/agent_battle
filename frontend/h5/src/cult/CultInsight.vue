<script setup lang="ts">
// src/cult/CultInsight.vue —— 屏三 · 参悟：假说卡（状态+凭据档位词）、
// 实验/试跑卡、请示卡（升级事件 → 准/止回发意图）。
import { computed } from 'vue'
import { cult, act, intend } from './state'

const hypTag = (status: string) =>
  status === '支持' ? { cls: 'ok', word: '已支持' }
  : status === '证伪' ? { cls: 'no', word: '已证伪' }
  : status === '存疑' ? { cls: 'no', word: '存疑' }
  : { cls: 'wait', word: '待验证' }

// 凭据分档词 → 置信条宽（词→档，零真值）。
const evidenceWidth = (word: string) =>
  word === '凭据累累' ? 92 : word === '凭据渐多' ? 65 : word === '已有初步凭据' ? 35 : 8

const trial = computed(() => cult.view?.trial)
const progressPct = computed(() => {
  const t = trial.value
  if (!t || t.loop) return 100
  return Math.min(100, Math.round((t.cycle / Math.max(t.total, 1)) * 100))
})

function approve() { void act([{ kind: 'resume' }]) }
function halt() { void act([{ kind: 'stop' }]) }
function wait() { void intend([{ kind: 'sense' }]) }
</script>

<template>
  <template v-if="cult.view">
    <!-- 请示卡（升级事件上浮，最优先） -->
    <div class="c-ask" v-for="r in cult.view.requests" :key="r.id">
      <div class="at">分身请示 · {{ r.kind === 'out_of_boundary' ? '越界之请' : r.kind === 'unknown_anomaly' ? '异象' : '护卫叫停' }}</div>
      <div class="q">{{ r.text }}</div>
      <div class="btns">
        <button class="b go" :disabled="cult.busy" @click="approve">准 · 续行</button>
        <button class="b" :disabled="cult.busy" @click="wait">再等等</button>
        <button class="b halt" :disabled="cult.busy" @click="halt">止</button>
      </div>
    </div>

    <!-- 实验/试跑卡 -->
    <div class="c-card c-exp" v-if="trial">
      <div class="t">功法试跑 · {{ trial.loop ? '自运行中' : '运行中' }}<span class="c-tag ok">第 {{ trial.cycle }}/{{ trial.total }} 周天</span></div>
      <div class="meta">「{{ trial.protocol }}」由 Runtime 自行运转，分身旁听——变量唯一，方能归因。</div>
      <div class="c-prog"><i :style="{ width: progressPct + '%' }"></i></div>
      <div class="ev c-hyp" v-for="(c, i) in trial.recent" :key="i" style="margin-top:4px">
        <span :class="c.success ? '' : ''">
          <span :style="{ color: c.success ? 'var(--jade)' : 'var(--cinnabar)' }">第 {{ c.cycle }} 周天{{ c.success ? '平稳渡过' : '未全功' }}</span>
          <template v-if="c.fail_why">——{{ c.fail_why }}</template>
        </span>
      </div>
    </div>

    <!-- 假说卡 -->
    <div class="c-card c-hyp" v-for="(h, i) in cult.view.hypotheses" :key="i">
      <div class="hd">
        <span class="hz">{{ h.text }}<span class="c-tag" :class="hypTag(h.status).cls">{{ hypTag(h.status).word }}</span></span>
        <span class="cf">{{ h.status }}</span>
      </div>
      <div class="c-confbar"><i :style="{ width: evidenceWidth(h.evidence) + '%' }"></i></div>
      <div class="ev">{{ h.evidence }}</div>
    </div>
    <div class="c-card c-hyp" v-if="!cult.view.hypotheses.length">
      <div class="t">假说</div>
      <div class="ev">分身尚未记下任何猜想。它一面摸索，一面会把心得交 record_hypothesis 存进证据账本。</div>
    </div>

    <!-- 证据账本（知识系统的定性视图） -->
    <div class="c-card c-hyp" v-if="cult.view.ledger.length">
      <div class="t">证据账本</div>
      <div class="ev" v-for="(l, i) in cult.view.ledger.slice(0, 10)" :key="i">{{ l }}</div>
    </div>
  </template>
</template>
