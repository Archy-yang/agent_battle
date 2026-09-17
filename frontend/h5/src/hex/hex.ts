// 六边形坐标核心数学（pointy-top / axial）—— 设计文档 §1.1、§1.3 公式的
// TS 实现，与 backend/engine/hex.go 同公式同浮点顺序。
// 本模块无 Vue / DOM 依赖，可逐行移植到 Flutter（设计 §4.3）。

/** 轴向坐标（JSON 契约 {q, r}，与 engine.Hex 一致） */
export interface Hex {
  q: number
  r: number
}

const SQRT3 = Math.sqrt(3)

export const hexKey = (h: Hex): string => `${h.q},${h.r}`
export const hexEqual = (a: Hex, b: Hex): boolean => a.q === b.q && a.r === b.r
export const hexAdd = (a: Hex, b: Hex): Hex => ({ q: a.q + b.q, r: a.r + b.r })

/**
 * 六邻居固定顺序（设计 §1.1，替代方形网格 bfsDirs）：
 * E NE NW W SW SE
 */
export const HEX_DIRECTIONS: readonly Hex[] = [
  { q: 1, r: 0 },
  { q: 1, r: -1 },
  { q: 0, r: -1 },
  { q: -1, r: 0 },
  { q: -1, r: 1 },
  { q: 0, r: 1 },
] as const

export function hexNeighbors(h: Hex): Hex[] {
  return HEX_DIRECTIONS.map(d => ({ q: h.q + d.q, r: h.r + d.r }))
}

/** HexDist：6 邻域上的真实步数（设计 §1.4，替代 Chebyshev） */
export function hexDist(a: Hex, b: Hex): number {
  const dq = a.q - b.q
  const dr = a.r - b.r
  return (Math.abs(dq) + Math.abs(dr) + Math.abs(dq + dr)) / 2
}

/** axial → 像素（pointy-top，s = 外接圆半径；设计 §1.3） */
export function hexToPixel(q: number, r: number, s: number): { x: number; y: number } {
  return { x: s * SQRT3 * (q + r / 2), y: s * 1.5 * r }
}

export const hexCenter = (h: Hex, s: number): { x: number; y: number } =>
  hexToPixel(h.q, h.r, s)

/** 像素 → axial 浮点（拾取第一步，设计 §1.3） */
export function pixelToHexFloat(x: number, y: number, s: number): { q: number; r: number } {
  return {
    q: ((SQRT3 / 3) * x - (1 / 3) * y) / s,
    r: ((2 / 3) * y) / s,
  }
}

/**
 * 像素 → hex（拾取）：cube rounding —— round 后修正最大分量误差（设计 §1.3）。
 * 输入像素以原点 hex(0,0) 的中心为基准（hexToPixel 的逆）。
 */
export function pixelToHex(x: number, y: number, s: number): Hex {
  const { q: qf, r: rf } = pixelToHexFloat(x, y, s)
  const sf = -qf - rf
  let q = Math.round(qf)
  let r = Math.round(rf)
  const sc = Math.round(sf)
  const dq = Math.abs(q - qf)
  const dr = Math.abs(r - rf)
  const ds = Math.abs(sc - sf)
  if (dq > dr && dq > ds) {
    q = -r - sc
  } else if (dr > ds) {
    r = -q - sc
  }
  return { q, r }
}

/** 六个顶点（pointy-top：角度 60°·i − 30°），供 canvas path 使用 */
export function hexCorners(cx: number, cy: number, s: number): { x: number; y: number }[] {
  const pts: { x: number; y: number }[] = []
  for (let i = 0; i < 6; i++) {
    const a = (Math.PI / 180) * (60 * i - 30)
    pts.push({ x: cx + s * Math.cos(a), y: cy + s * Math.sin(a) })
  }
  return pts
}
