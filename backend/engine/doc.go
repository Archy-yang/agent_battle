// Package engine hosts the deterministic World Engine — the only place
// where the true World State lives (docs/world_rule.md §32).
//
// Planned contents, mapped to the design docs:
//
//   - entity.go      Entity / Component / Port / Connection graph (world_rule §4–5)
//   - laws.go        五行 essences, 生克 matrix, 侮微衡乘 curve (world_laws §5)
//   - energy.go      源→储→用 energy flow (world_laws §4 律七)
//   - curves.go      阴阳 curves: 耗气/生热/显迹 ∝ x 或 x² (world_laws §3)
//   - resolver.go    Intent validate → resolve (world_rule §12, §30 module 3)
//   - observation.go Reality → Perception: fog + signal (world_rule §19)
//
// All simulation arithmetic uses fixed-point integers, never floats,
// so that replays are bit-for-bit reproducible (world_genesis §7).
package engine
