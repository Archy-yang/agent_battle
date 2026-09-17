// v0.8 共享世界渲染层：MapView.vue / ReplayView.vue 唯一的 canvas 六边形
// 绘制来源（v0.7 两 view 各自内联一份，本文件为它们的并集，行为以
// MapView 为基准）。纯绘制函数，无 Vue 依赖；交互（点击下单 / 拖拽缩放 /
// 播放器）仍归各 view 自有。
//
// 覆盖的绘制能力：迷雾三态底色、五行地形色（服务端记忆 essence）、
// 晶簇菱形、实体标记（💎 / ❓ + 五行描边）、出生点 ⌂ / 移动订单虚线框、
// 危险格火色描边 + 斜线纹理、本体 🤖 高亮环。
import {
  essenceColor, essenceColorByName,
  type ExploredCell, type Hex, type SeenEntity, type SelfView,
} from './api'
import {
  fogOf, hexCorners, hexCenter, hexKey, visibleHexes, worldToScreen,
  type Camera, type Viewport,
} from './hex'

// ---- 常量（原两 view 各自内联的同名值，收敛为单一来源） ----
/** hexsize：世界像素下的外接圆半径（zoom=1） */
export const DEFAULT_HEX_SIZE = 28
export const ACCENT = '#d4a94e' // 与 --accent 一致
export const FIRE = '#e05545' // 与 --fire 一致
export const WATER = '#4a8fd4' // 与 --water 一致
export const SELF_RING = '#9fd0ff'
export const HEX_BORDER = 'rgba(15, 18, 22, 0.55)'
/** 迷雾三态底色（与 state.ts FOG 同值；绘制层不依赖 state，避免 Vue 耦合） */
export const FOG_COLORS = {
  unexplored: '#12161d',
  remembered: '#2a2f37',
  visible: '#3a424e',
} as const

/** 绘制所需的最小数据面（StateView.observation + enemies 的投影） */
export interface WorldMapView {
  /** 本体视图；null = 尚无观测（不画本体高亮环） */
  self: SelfView | null
  /** 当前感知范围内的格坐标 */
  visible: Hex[]
  /** 感知内的实体（observation.entities） */
  entities: SeenEntity[]
  /** 敌对实体视图（essence 为五行中文名，与 SeenEntity.essence 数字不同） */
  enemies?: { pos: Hex; essence: string }[]
}

export interface WorldRenderOpts {
  ctx: CanvasRenderingContext2D
  view: WorldMapView
  /** 已探索格键集（"q,r"）——迷雾 remembered 判定 */
  explored: Set<string>
  /** 已探索格地形记忆（essence / danger），服务端未下发时不猜测 */
  cells: Map<string, ExploredCell>
  camera: Camera
  viewport: Viewport
  /** hexsize（外接圆半径，zoom=1），一般传 DEFAULT_HEX_SIZE */
  size: number
  /** 出生点 ⌂ / 移动订单目标虚线框；缺省不画 */
  markers?: { spawn?: Hex | null; order?: Hex | null }
  /**
   * true = 对局视图（可交互）；false = 回放等只读模式。
   * 目前两模式绘制完全一致（hover 高亮是 v0.8+ 的预留位），参数先
   * 固化语义：只读模式未来不画 hover / 拾取反馈。
   */
  interactions?: boolean
}

interface MarkerVm { q: number; r: number; glyph: string; color?: string; diamond?: boolean }

/** 实体层 VM：◆ 晶簇 / 💎 结晶 / ❓ 敌对实体（entities + enemies 去重合并） */
function buildMarkers(view: WorldMapView): MarkerVm[] {
  const out: MarkerVm[] = []
  const seen = new Set<string>()
  for (const e of view.entities) {
    const k = hexKey(e.pos)
    if (seen.has(k)) continue
    seen.add(k)
    if (e.kind === 'resource') {
      out.push({ q: e.pos.q, r: e.pos.r, glyph: '', color: essenceColor(e.essence), diamond: true })
    } else {
      out.push(e.kind === 'fragment'
        ? { q: e.pos.q, r: e.pos.r, glyph: '💎' }
        : { q: e.pos.q, r: e.pos.r, glyph: '❓', color: essenceColor(e.essence) })
    }
  }
  for (const en of view.enemies ?? []) {
    const k = hexKey(en.pos)
    if (seen.has(k)) continue
    seen.add(k)
    out.push({ q: en.pos.q, r: en.pos.r, glyph: '❓', color: essenceColorByName(en.essence) })
  }
  return out
}

function hexPath(ctx: CanvasRenderingContext2D, x: number, y: number, r: number) {
  ctx.beginPath()
  const pts = hexCorners(x, y, r)
  ctx.moveTo(pts[0].x, pts[0].y)
  for (let i = 1; i < 6; i++) ctx.lineTo(pts[i].x, pts[i].y)
  ctx.closePath()
}

/**
 * 世界单帧绘制（唯一实现）。调用方负责 canvas 尺寸 / dpr 适配与重绘调度；
 * 本函数只按传入的 camera / viewport 画一帧。
 */
export function drawWorld(opts: WorldRenderOpts): void {
  const { ctx, view, explored, cells, camera: c, viewport, size: HEX_SIZE } = opts
  const { width, height } = viewport
  const s = HEX_SIZE * c.zoom
  const r = s * 0.96

  // 未探索 = 深底（不绘制的部分自然露出）
  ctx.fillStyle = FOG_COLORS.unexplored
  ctx.fillRect(0, 0, width, height)
  if (s < 3) return

  const visible = new Set<string>()
  for (const h of view.visible) visible.add(hexKey(h))
  const order = opts.markers?.order ?? null
  const spawn = opts.markers?.spawn ?? null
  void opts.interactions // 语义预留位（见 WorldRenderOpts 注释），当前两模式绘制一致

  // 地形层：只渲染服务端已知格（explored / observation.visible）
  for (const h of visibleHexes(c, viewport, HEX_SIZE)) {
    const fog = fogOf(h, explored, visible)
    if (fog === 'unexplored') continue
    const w = hexCenter(h, HEX_SIZE)
    const p = worldToScreen(w.x, w.y, c, viewport)
    hexPath(ctx, p.x, p.y, r)
    if (fog === 'visible') {
      // 感知内全亮：服务端下发五行记忆（ExploredCell.essence）时按五行上色，
      // 否则中性亮灰 —— 客户端不猜测地形
      const cell = cells.get(hexKey(h))
      ctx.fillStyle = cell?.essence != null ? essenceColor(cell.essence) : FOG_COLORS.visible
      ctx.fill()
      ctx.strokeStyle = HEX_BORDER
      ctx.lineWidth = 1
      ctx.stroke()
      if (cell?.danger) {
        // Danger 叠加：火色描边 + 斜线纹理
        ctx.strokeStyle = FIRE
        ctx.lineWidth = 2
        hexPath(ctx, p.x, p.y, r - 1.5)
        ctx.stroke()
        ctx.lineWidth = 1.2
        const half = r * 0.45
        for (let i = -1; i <= 1; i++) {
          ctx.beginPath()
          ctx.moveTo(p.x - half, p.y + i * half * 0.7)
          ctx.lineTo(p.x + half, p.y + i * half * 0.7 - half * 0.6)
          ctx.stroke()
        }
      }
    } else {
      // 已探索离开感知：45% 记忆色
      ctx.globalAlpha = 0.45
      ctx.fillStyle = FOG_COLORS.remembered
      ctx.fill()
      ctx.globalAlpha = 1
      ctx.strokeStyle = HEX_BORDER
      ctx.lineWidth = 1
      ctx.stroke()
    }
  }

  // 移动订单目标：强调金虚线框
  if (order) {
    const w = hexCenter(order, HEX_SIZE)
    const p = worldToScreen(w.x, w.y, c, viewport)
    hexPath(ctx, p.x, p.y, r - 2)
    ctx.setLineDash([5, 3])
    ctx.strokeStyle = ACCENT
    ctx.lineWidth = 2
    ctx.stroke()
    ctx.setLineDash([])
  }

  // 实体层
  ctx.textAlign = 'center'
  ctx.textBaseline = 'middle'
  const glyph = Math.max(9, Math.min(40, s * 1.05))
  ctx.font = `${glyph}px system-ui, sans-serif`
  const at = (h: Hex) => {
    const w = hexCenter(h, HEX_SIZE)
    return worldToScreen(w.x, w.y, c, viewport)
  }
  if (spawn) {
    const p = at(spawn)
    ctx.font = `${glyph * 0.85}px system-ui, sans-serif`
    ctx.fillStyle = WATER
    ctx.fillText('⌂', p.x, p.y + glyph * 0.04)
    ctx.font = `${glyph}px system-ui, sans-serif`
  }
  for (const m of buildMarkers(view)) {
    const p = at({ q: m.q, r: m.r })
    if (m.diamond) { // 晶簇：五行色菱形（站上去采集）
      const d = r * 0.52
      ctx.beginPath()
      ctx.moveTo(p.x, p.y - d)
      ctx.lineTo(p.x + d, p.y)
      ctx.lineTo(p.x, p.y + d)
      ctx.lineTo(p.x - d, p.y)
      ctx.closePath()
      ctx.fillStyle = m.color ?? ACCENT
      ctx.fill()
      ctx.strokeStyle = HEX_BORDER
      ctx.lineWidth = 1.5
      ctx.stroke()
      continue
    }
    if (m.color) { // 敌对实体：五行色描边
      ctx.beginPath()
      ctx.arc(p.x, p.y, r * 0.62, 0, Math.PI * 2)
      ctx.strokeStyle = m.color
      ctx.lineWidth = 2
      ctx.stroke()
    }
    ctx.fillText(m.glyph, p.x, p.y + glyph * 0.06)
  }
  if (view.self) {
    const p = at({ q: view.self.pos.q, r: view.self.pos.r })
    ctx.beginPath()
    ctx.arc(p.x, p.y, r * 0.66, 0, Math.PI * 2)
    ctx.strokeStyle = SELF_RING
    ctx.lineWidth = 2
    ctx.stroke()
    ctx.font = `${glyph * 1.1}px system-ui, sans-serif`
    ctx.fillText('🤖', p.x, p.y + glyph * 0.06)
  }
}
