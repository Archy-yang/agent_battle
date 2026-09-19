<script setup lang="ts">
// src/cult/CultBody.vue —— 屏二 · 内视：8 穴位 SVG 身体图（节点=状态色，
// 经脉线、损脉虚线），点穴出感知卡——全部来自 Observation，零数字。
import { computed, ref } from 'vue'
import { cult } from './state'
import type { PointView } from './api'

// 穴位布点（与设计稿屏二同构；未知穴位落到底部一排）。
const POS: Record<string, [number, number]> = {
  神庭: [150, 32],
  中庭: [150, 96],
  青关: [150, 156],
  下丹田: [150, 256],
  阳枢: [104, 216],
  土府: [196, 216],
  金门: [88, 308],
  玄泉: [212, 308],
}
const FALLBACK: [number, number][] = [[60, 250], [240, 250], [60, 150], [240, 150]]

const selected = ref('')

const pointByName = computed(() => {
  const m = new Map<string, PointView>()
  for (const p of cult.view?.points ?? []) m.set(p.name, p)
  return m
})

const nodes = computed(() => {
  const list = cult.view?.points ?? []
  let fb = 0
  return list.map((p) => {
    const pos = POS[p.name] ?? FALLBACK[fb++ % FALLBACK.length]
    return { name: p.name, x: pos[0], y: pos[1], sense: p }
  })
})

// 状态色：充盈金 / 温热火 / 平稳玉 / 沉寂灰 / 滞涩朱砂（损伤优先）。
function nodeColor(p: PointView): string {
  if (p.damage !== '无损') return 'var(--cinnabar)'
  const b = p.pressure_band
  if (b >= 4) return 'var(--w-fire)'
  if (b === 3) return 'var(--gold)'
  if (b >= 1) return 'var(--jade)'
  return 'var(--grey-dim)'
}
function nodeFill(color: string): string {
  return color === 'var(--gold)' ? '#201B10'
    : color === 'var(--jade)' ? '#141F1B'
    : color === 'var(--w-fire)' ? '#1C1613'
    : color === 'var(--cinnabar)' ? '#1A1210'
    : '#16211F'
}
function nodeWord(p: PointView): string {
  if (p.damage !== '无损') return p.damage
  if (p.pressure_band >= 3) return '胀'
  if (p.flow === '停滞') return '沉寂'
  return p.flow
}
function channelClass(from: string, to: string): string {
  const f = pointByName.value.get(from)
  const t = pointByName.value.get(to)
  if ((f && f.damage !== '无损') || (t && t.damage !== '无损')) return 'hurt'
  if ((f && f.flow !== '停滞') || (t && t.flow !== '停滞')) return 'flow'
  return ''
}
const current = computed(() => (selected.value ? pointByName.value.get(selected.value) : undefined))
</script>

<template>
  <template v-if="cult.view">
    <div class="c-card c-bodymap">
      <div class="t">身体图 · {{ cult.view.body.points.length }} 穴 · {{ cult.view.body.channels.length }} 脉 · 神识{{ cult.view.acuity === 1 ? '一层' : cult.view.acuity === 2 ? '二层' : '三层' }}</div>
      <div class="c-legend">
        <span><i style="background:var(--gold)"></i>充盈</span>
        <span><i style="background:var(--w-fire)"></i>温热</span>
        <span><i style="background:var(--jade)"></i>平稳</span>
        <span><i style="background:var(--grey-dim)"></i>沉寂</span>
        <span><i style="background:var(--cinnabar)"></i>滞涩</span>
      </div>
      <svg viewBox="0 0 300 330">
        <!-- 经脉（损脉虚线、行气青玉） -->
        <template v-for="c in cult.view.body.channels" :key="c.name">
          <path
            v-if="POS[c.from] && POS[c.to]"
            class="c-meridian" :class="channelClass(c.from, c.to)"
            :d="`M${POS[c.from][0]} ${POS[c.from][1]} L${POS[c.to][0]} ${POS[c.to][1]}`"
          />
        </template>
        <!-- 穴位 -->
        <g v-for="n in nodes" :key="n.name" @click="selected = n.name">
          <circle class="c-acu" :cx="n.x" :cy="n.y" :r="n.name === '下丹田' ? 12 : 9"
                  :fill="nodeFill(nodeColor(n.sense))" :stroke="nodeColor(n.sense)"
                  :stroke-width="selected === n.name ? 2 : 1.2"/>
          <circle class="c-acu-ring" :cx="n.x" :cy="n.y" :r="n.name === '下丹田' ? 18 : 13" :stroke="nodeColor(n.sense)"/>
          <text class="c-node-label" :x="n.x" :y="n.y - 18" text-anchor="middle">{{ n.name }}</text>
          <text class="c-node-sub" :x="n.x" :y="n.y + 27" text-anchor="middle">{{ nodeWord(n.sense) }}</text>
        </g>
      </svg>
    </div>

    <!-- 点穴感知卡 -->
    <div class="c-card c-detail" v-if="current">
      <div class="t">{{ current.name }} · 所感</div>
      <div class="row"><span class="k">感觉</span><span class="v">{{ current.feelings.length ? current.feelings.join(' · ') : '无明显感觉' }}</span></div>
      <div class="row"><span class="k">气机</span><span class="v">{{ current.flow }}</span></div>
      <div class="row"><span class="k">开度</span><span class="v">{{ current.openness }}</span></div>
      <div class="row"><span class="k">稳固</span><span class="v" :class="{ jade: current.stability.startsWith('稳') }">{{ current.stability }}</span></div>
      <div class="row"><span class="k">损伤</span><span class="v" :class="{ jade: current.damage === '无损' }">{{ current.damage }}</span></div>
      <div class="row"><span class="k">五行倾向</span><span class="v">{{ current.nature.length ? current.nature.join('、') : '辨不出' }}</span></div>
      <div class="note">所见皆感知——数值藏在世界里，须经实验方能参悟。</div>
    </div>
    <div class="c-card c-detail" v-else>
      <div class="t">点一穴，观其所感</div>
      <div class="note">穴位的状态只用定性词呈现；「滞涩」者有伤，虚线之脉慎引。</div>
    </div>
  </template>
</template>
