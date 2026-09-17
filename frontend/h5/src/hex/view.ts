// 相机模型与视口裁剪（设计 §4.1）—— 纯函数，无 Vue / DOM 依赖。
// 相机 {cx, cy, zoom}：世界中心像素坐标 + 缩放（世界 px → 屏幕 px）。
import { hexKey, hexToPixel, pixelToHexFloat, pixelToHex, type Hex } from './hex'

export interface Camera {
  cx: number // 世界中心像素 x
  cy: number // 世界中心像素 y
  zoom: number // 世界 px → 屏幕 px
}

export interface Viewport {
  width: number
  height: number
}

export const ZOOM_MIN = 0.5
export const ZOOM_MAX = 2.0

export const clampZoom = (z: number): number =>
  Math.min(ZOOM_MAX, Math.max(ZOOM_MIN, z))

/** 屏幕坐标 → 世界像素（相机逆变换） */
export function screenToWorld(
  sx: number, sy: number, cam: Camera, vp: Viewport,
): { x: number; y: number } {
  return {
    x: cam.cx + (sx - vp.width / 2) / cam.zoom,
    y: cam.cy + (sy - vp.height / 2) / cam.zoom,
  }
}

/** 世界像素 → 屏幕坐标 */
export function worldToScreen(
  wx: number, wy: number, cam: Camera, vp: Viewport,
): { x: number; y: number } {
  return {
    x: (wx - cam.cx) * cam.zoom + vp.width / 2,
    y: (wy - cam.cy) * cam.zoom + vp.height / 2,
  }
}

/** hex 拾取：屏幕坐标 → hex（§1.3 逆变换 + cube rounding） */
export function pickHex(sx: number, sy: number, cam: Camera, vp: Viewport, s: number): Hex {
  const w = screenToWorld(sx, sy, cam, vp)
  return pixelToHex(w.x, w.y, s)
}

/** 绕屏幕上一点缩放（保持该点下的世界坐标不动） */
export function zoomAt(sx: number, sy: number, factor: number, cam: Camera, vp: Viewport): Camera {
  const z1 = clampZoom(cam.zoom * factor)
  if (z1 === cam.zoom) return cam
  const w = screenToWorld(sx, sy, cam, vp)
  return {
    cx: w.x - (sx - vp.width / 2) / z1,
    cy: w.y - (sy - vp.height / 2) / z1,
    zoom: z1,
  }
}

/**
 * 视口裁剪枚举（设计 §4.1）：可视范围内的全部 hex，可见格数 ≈ 300~500。
 * 做法四角反解出轴向浮动坐标取包围盒（平行四边形），再按中心像素过滤。
 * 无限世界不枚举全图——只有这一屏。
 */
export function visibleHexes(cam: Camera, vp: Viewport, s: number): Hex[] {
  const halfW = vp.width / 2 / cam.zoom + s
  const halfH = vp.height / 2 / cam.zoom + s
  const x0 = cam.cx - halfW
  const x1 = cam.cx + halfW
  const y0 = cam.cy - halfH
  const y1 = cam.cy + halfH

  // 视口四角的 axial 浮动坐标 → 轴向包围盒（多留 1 格余量）
  const corners = [
    pixelToHexFloat(x0, y0, s), pixelToHexFloat(x1, y0, s),
    pixelToHexFloat(x0, y1, s), pixelToHexFloat(x1, y1, s),
  ]
  let qMin = Infinity, qMax = -Infinity, rMin = Infinity, rMax = -Infinity
  for (const c of corners) {
    qMin = Math.min(qMin, c.q)
    qMax = Math.max(qMax, c.q)
    rMin = Math.min(rMin, c.r)
    rMax = Math.max(rMax, c.r)
  }

  const out: Hex[] = []
  for (let q = Math.floor(qMin) - 1; q <= Math.ceil(qMax) + 1; q++) {
    for (let r = Math.floor(rMin) - 1; r <= Math.ceil(rMax) + 1; r++) {
      const p = hexToPixel(q, r, s)
      if (p.x < x0 || p.x > x1 || p.y < y0 || p.y > y1) continue
      out.push({ q, r })
    }
  }
  return out
}

/** 迷雾三态（设计 §4.2） */
export type FogState = 'unexplored' | 'remembered' | 'visible'

/**
 * 迷雾判定：visible 优先于 explored（感知内一定全亮）。
 * explored / visible 为 "q,r" 键集合（state.ts exploredSet / visibleSet）。
 */
export function fogOf(h: Hex, explored: Set<string>, visible: Set<string>): FogState {
  const k = hexKey(h)
  if (visible.has(k)) return 'visible'
  if (explored.has(k)) return 'remembered'
  return 'unexplored'
}
