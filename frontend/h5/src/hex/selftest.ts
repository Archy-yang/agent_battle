// 开发用纯函数自测：无测试框架依赖（任务约束不引入新依赖）。
// 抽查方式一：esbuild 打包后 node 运行（见模块底部用法注释）；
// 抽查方式二：浏览器控制台 `hexSelfTest()`。
// 覆盖设计 §1.3 换算往返、§1.4 距离/邻居、§4.1 视口裁剪、§4.2 迷雾判定。
import {
  hexDist, hexKey, hexNeighbors, hexToPixel, pixelToHex, HEX_DIRECTIONS,
  type Hex,
} from './hex'
import { clampZoom, fogOf, pickHex, visibleHexes, ZOOM_MAX, ZOOM_MIN } from './view'

export interface SelfTestResult {
  ok: boolean
  failures: string[]
  checked: number
}

export function hexSelfTest(): SelfTestResult {
  const failures: string[] = []
  let checked = 0
  const assert = (cond: boolean, msg: string) => {
    checked++
    if (!cond) failures.push(msg)
  }

  const s = 26 // 任取外接圆半径

  // 1) 换算往返：像素 → hex 与原格一致（§1.3，双端一致性目标 < 0.5px）
  const sample: Hex[] = []
  for (let q = -30; q <= 30; q++) {
    for (let r = -30; r <= 30; r++) sample.push({ q, r })
  }
  let maxErr = 0
  for (const h of sample) {
    const p = hexToPixel(h.q, h.r, s)
    const back = pixelToHex(p.x, p.y, s)
    assert(back.q === h.q && back.r === h.r, `roundtrip 失败 (${h.q},${h.r})`)
    // 中心点像素距离：换算再反算，中心误差应为 0
    const c = hexToPixel(back.q, back.r, s)
    maxErr = Math.max(maxErr, Math.hypot(c.x - p.x, c.y - p.y))
    // 邻居中心像素必须落在该邻格（拾取正确性）
  }
  assert(maxErr === 0, `中心往返误差 ${maxErr}px ≠ 0`)

  // 2) 邻居：6 个、互异、与自身 HexDist 均为 1（§1.4）
  const n0 = hexNeighbors({ q: 3, r: -4 })
  assert(n0.length === 6, `邻居数 ${n0.length} ≠ 6`)
  assert(new Set(n0.map(hexKey)).size === 6, '邻居重复')
  for (const n of n0) assert(hexDist({ q: 3, r: -4 }, n) === 1, '邻居距离 ≠ 1')
  assert(hexDist({ q: 0, r: 0 }, { q: 2, r: -1 }) === 2, 'HexDist(0,0 → 2,-1) ≠ 2')
  assert(hexDist({ q: 5, r: 5 }, { q: 5, r: 5 }) === 0, 'HexDist 自身 ≠ 0')
  // 方形网格的角接触歧义在六边形下不存在：对角格距离 2，无"斜角"
  assert(HEX_DIRECTIONS.length === 6, '方向表 ≠ 6')

  // 3) 拾取边界：两格中点处的浮点抖动必须仍落在某邻格（cube rounding）
  const a = hexToPixel(0, 0, s)
  const b = hexToPixel(1, 0, s)
  const mid = pixelToHex((a.x + b.x) / 2 + 0.01, (a.y + b.y) / 2, s)
  const dMid = Math.min(hexDist(mid, { q: 0, r: 0 }), hexDist(mid, { q: 1, r: 0 }))
  assert(dMid <= 1, `两格中点拾取距离 ${dMid} > 1`)

  // 4) 视口裁剪（§4.1）：可见格数在合理量级，且全部中心在视口内
  const cam = { cx: 0, cy: 0, zoom: 1 }
  const vp = { width: 980, height: 560 }
  const vis = visibleHexes(cam, vp, s)
  assert(vis.length > 100 && vis.length < 1200, `可见格数 ${vis.length} 异常`)
  for (const h of vis) {
    const p = hexToPixel(h.q, h.r, s)
    assert(
      p.x >= cam.cx - vp.width / 2 - s && p.x <= cam.cx + vp.width / 2 + s &&
      p.y >= cam.cy - vp.height / 2 - s && p.y <= cam.cy + vp.height / 2 + s,
      `格 (${h.q},${h.r}) 在视口外`,
    )
  }
  // 屏幕拾取与直接拾取一致（相机变换可逆）
  const sp = { sx: 300, sy: 200 }
  const wx = cam.cx + (sp.sx - vp.width / 2) / cam.zoom
  const wy = cam.cy + (sp.sy - vp.height / 2) / cam.zoom
  const picked = pickHex(sp.sx, sp.sy, cam, vp, s)
  const direct = pixelToHex(wx, wy, s)
  assert(picked.q === direct.q && picked.r === direct.r, 'pickHex 与直接逆变换不一致')

  // 5) 缩放钳制 0.5~2.0
  assert(clampZoom(0.1) === ZOOM_MIN && clampZoom(9) === ZOOM_MAX, 'zoom 钳制失效')

  // 6) 迷雾三态（§4.2）
  const explored = new Set(['0,0', '1,0'])
  const visible = new Set(['1,0'])
  assert(fogOf({ q: 0, r: 0 }, explored, visible) === 'remembered', '已探索离开感知 ≠ remembered')
  assert(fogOf({ q: 1, r: 0 }, explored, visible) === 'visible', '感知内 ≠ visible')
  assert(fogOf({ q: 9, r: 9 }, explored, visible) === 'unexplored', '未探索 ≠ unexplored')

  return { ok: failures.length === 0, failures, checked }
}

// node 抽查（不引入新依赖，用 vite 自带的 esbuild）：
//   npx esbuild src/hex/selftest.ts --bundle --outfile=/tmp/hex-selftest.cjs && node /tmp/hex-selftest.cjs
// 浏览器打包里 process 不存在，此分支不执行。
const g = globalThis as { process?: { argv?: string[]; exitCode?: number } }
if (g.process?.argv?.[1]?.includes('selftest')) {
  const r = hexSelfTest()
  console.log(JSON.stringify(r, null, 2))
  if (!r.ok && g.process) g.process.exitCode = 1
}
