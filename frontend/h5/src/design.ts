// 设计 token：供各页面 <style> 参照。
// 同一组值以 CSS 变量形式定义在 App.vue 的 :root 中，样式内优先用 var(--*)。
export const DESIGN = {
  // 底色 / 面板 / 边框 / 文字
  bg: '#0f1216',
  panel: '#171c24',
  border: '#262e3a',
  text: '#d8dce2',
  textDim: '#7a8290',

  // 五行语义色（贯穿全 UI 的核心视觉语言）
  wood: '#57b26a',
  fire: '#e05545',
  earth: '#c9a44a',
  metal: '#b8c0cc',
  water: '#4a8fd4',

  // 强调金：只用于 Influence 与关键行动
  accent: '#d4a94e',

  // 顶部导航当前页下划线
  navActive: '#9fd0ff',

  radius: '6px',
} as const
