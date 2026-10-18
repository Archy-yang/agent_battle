<script setup lang="ts">
// src/cult/CultSheet.vue —— 通用底部滑出浮层：遮罩 + 圆角上滑 +
// 下滑（点「收起」）/ 点遮罩关闭。心声 / 身体记事 / 修行进度三个
// 实例共用；犹豫卡（请示）走独立模态，不经此容器。
defineProps<{ open: boolean; title: string }>()
const emit = defineEmits<{ close: [] }>()
</script>

<template>
  <div class="cu-sheet" :class="{ show: open }">
    <div class="mask" @click="emit('close')"></div>
    <div class="panel" role="dialog" :aria-label="title">
      <div class="grab" aria-hidden="true"></div>
      <div class="head ui">
        <b>{{ title }}</b>
        <button @click="emit('close')">收起</button>
      </div>
      <div class="sbody"><slot /></div>
    </div>
  </div>
</template>
