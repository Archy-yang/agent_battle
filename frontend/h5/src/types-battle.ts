// M3.5 对战页扩展类型：引擎在 StateView 上追加的战斗字段。
// 正式字段合入 api.ts 后可移除本文件，改为直接 import。
import type { StateView, Vec2 } from './api'

// view.enemies：感知范围内的敌对实体（战斗视角，带距离）
export interface CombatEnemy {
  id: number
  pos: Vec2
  distance: number
  essence: number
  integrity: number
  heat: number
}

// 战斗扩展视图：in_combat / enemies 为可选，向后兼容旧契约
export interface BattleStateView extends StateView {
  in_combat?: boolean
  enemies?: CombatEnemy[]
}
