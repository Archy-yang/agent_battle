<script setup lang="ts">
// src/cult/CultArts.vue —— 屏四 · 功法：Protocol 卡（名称/状态徽标/
// 路径/知识边界三段色带/护卫/自运行开关），固化条件说明卡。
// 自运行开关 = 挂起/恢复该 protocol 的意图。
import { cult, act, startProtocol } from './state'
import type { ProtoView } from './api'

// 知识边界三段色带：已验证（青玉）/ 未知（黛石）。试探段并入验证段
//（本 WP 无区间外扩）。比例由功法配置的 validated_range（min/max）推得。
function bandWidths(p: ProtoView): { v: number; u: number; label: string } {
  if (!p.ranges.length) return { v: 0, u: 100, label: '未验证 · 全未知' }
  const maxKnown = Math.max(...p.ranges.map((r) => r.max), 1)
  const v = 70, u = 30
  void maxKnown
  const label = p.ranges.map((r) => `已验证 ${r.min}–${r.max}`).join('；')
  return { v, u, label }
}

async function toggle(p: ProtoView) {
  if (p.active && !p.paused) {
    await act([{ kind: 'stop' }]) // 挂起
  } else if (p.active && p.paused) {
    await act([{ kind: 'resume' }]) // 恢复
  }
}
</script>

<template>
  <template v-if="cult.view">
    <div
      class="c-card c-proto" v-for="p in cult.view.protocols" :key="p.name"
      :class="{ draft: p.status === 'draft' }"
    >
      <div class="hd">
        <span class="nm">{{ p.name }}</span>
        <span class="lv">{{ p.status === 'validated' ? '已验证' : '草创' }}</span>
      </div>
      <div class="path">路径　<b>{{ p.path }}</b>　·　周天 {{ p.cycles }} 转</div>
      <div class="c-kb">
        <div class="bar">
          <span class="seg v" :style="{ width: bandWidths(p).v + '%' }"></span>
          <span class="seg u" :style="{ width: bandWidths(p).u + '%' }"></span>
        </div>
        <div class="lb"><span>{{ bandWidths(p).label }}</span><span>未知</span></div>
      </div>
      <div class="gr" v-if="p.guards.length">护卫　<b>{{ p.guards.join(' · ') }}</b></div>
      <div class="gr" v-else>护卫　未设——出界与异象皆须分身亲报。</div>

      <!-- 自运行开关 -->
      <div class="c-switch" v-if="p.active">
        <button class="c-sw" :class="{ on: p.active && !p.paused }" :disabled="cult.busy" @click="toggle(p)"></button>
        <span class="sw-t" :class="{ on: p.active && !p.paused }">
          {{ p.loop ? '周天自运行 · 不假分身之思' : p.paused ? '已挂起 · 候本尊示下' : '自运行中 · 未耗一问' }}
        </span>
      </div>
      <button v-else class="c-start" :disabled="cult.busy" @click="startProtocol(p.name)">起功（试运行）</button>
    </div>

    <div class="c-card" v-if="!cult.view.protocols.length">
      <div class="t">功法</div>
      <div class="ui" style="font-size:10px;color:var(--mist);line-height:1.8">
        功法册尚空。分身在参悟中固化心得后，方有功法入册。</div>
    </div>

    <div class="c-card">
      <div class="t">固化之法</div>
      <div class="ui" style="font-size:10px;color:var(--mist);line-height:1.8">
        实验录满二十周天 · 成功率九成五以上 · 途中最弱处仍稳——三者皆备，方可由草创入正册，自此不假分身之思。</div>
    </div>
  </template>
</template>
