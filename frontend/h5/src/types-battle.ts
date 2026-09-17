// v0.4 集成统一：战斗字段（in_combat / enemies / order）已正式合入
// api.ts 的 StateView，本文件仅保留兼容导出。
export type { EnemyView as CombatEnemy, StateView as BattleStateView } from './api'
