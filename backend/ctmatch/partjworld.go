package ctmatch

// partjworld.go —— Part J 标定世界（docs/specs/2026-09-22-partj-passive-
// circulation.md J8/P7-P9）：六穴单环、全开度、高传导的"修行有成"身体。
//
// 用途：被动环流的结构性标定（P7 衰减-结构、P8 环必要性、P9 筑基可达性）
// 与 T-J6 自然筑基验收。它与 SpikeWorld 的差别全在参数画像：SpikeWorld 是
// 引气期新手身体（低开度、小穴位），其传导上限使 FlowClosure 天花板低于
// 筑基门槛（§37 Closure≥0.80 不可达）——筑基按 J6 因果链需要长期行环把
// 开度/适应修上去，标定台用本世界代表"结构已演化到位"的状态。
//
// 全部数值 [v0.1 参数]，与 cultmc 报告一并标定。

import (
	"agentbattle/backend/cult"
)

// PartJCalibrationWorld 构造 Part J 标定世界（六穴主环，确定性）。
func PartJCalibrationWorld() *cult.World {
	pt := func(name string, cap, tp int64, qi [cult.ElementCount]int64) cult.Acupoint {
		return cult.Acupoint{
			Name:          name,
			Affinity:      [cult.ElementCount]int64{1000, 1000, 1000, 1000, 1000},
			Capacity:      cap,
			Throughput:    tp,
			Retention:     900,
			Openness:      cult.MicroOne,
			Stability:     950,
			BaseStability: 950,
			Activity:      [cult.ElementCount]int64{1000, 1000, 1000, 1000, 1000},
			Adaptation:    [cult.ElementCount]int64{1000000, 1000000, 1000000, 1000000, 1000000},
			Qi:            qi,
			// 驱动画像（0.5.0 [v0.1 参数]）：普通穴 10000（≥ 各经脉 large 档
			// 0.8×10000=8000，引流手感不变）；下丹田在下方覆盖为 24000——
			// 全图唯一显著主导（0.5.0 核点判据，逆流外推唯它推得动）。
			DriveCapacity:   10000,
			DriveEfficiency: 900,
		}
	}
	mid := [cult.ElementCount]int64{4800, 4800, 4800, 4800, 4800}       // 总量 24000/容量 30000 → LR≈0.8
	full := [cult.ElementCount]int64{24000, 24000, 24000, 24000, 24000} // 总量 120000/容量 150000 → LR 0.8
	char := cult.Character{
		Affinity:   [cult.ElementCount]int64{1000, 1000, 1000, 1000, 1000},
		Control:    1000,
		Perception: 500,
		BodyGraph: cult.BodyGraph{
			Acupoints: []cult.Acupoint{
				pt("下丹田", 150000, 20000, full),
				pt("青关", 30000, 10000, mid),
				pt("土府", 30000, 10000, mid),
				pt("中庭", 30000, 10000, mid),
				pt("阳枢", 30000, 10000, mid),
				pt("神庭", 30000, 10000, mid),
			},
			Channels: []cult.Channel{
				{Name: "下丹田→土府", From: 0, To: 2, BaseConductance: 980, Throughput: 10000},
				{Name: "土府→中庭", From: 2, To: 3, BaseConductance: 980, Throughput: 10000},
				{Name: "中庭→阳枢", From: 3, To: 4, BaseConductance: 980, Throughput: 10000},
				{Name: "阳枢→神庭", From: 4, To: 5, BaseConductance: 980, Throughput: 10000},
				{Name: "神庭→青关", From: 5, To: 1, BaseConductance: 980, Throughput: 10000},
				{Name: "青关→下丹田", From: 1, To: 0, BaseConductance: 980, Throughput: 10000},
			},
		},
	}
	env := cult.Environment{
		Qi:          [cult.ElementCount]int64{3000, 3000, 3000, 3000, 3000},
		Activity:    [cult.ElementCount]int64{1000, 1000, 1000, 1000, 1000},
		Temperature: 1000,
	}
	w := cult.NewWorld(char, env)
	// 丹田驱动画像覆盖（0.5.0 [v0.1 参数]）：下丹田（下标 0）24000——普通穴
	// 10000 的 2.4 倍（> CoreDriveSignificance 判据，全图唯一核点）。
	w.Character.BodyGraph.Acupoints[0].DriveCapacity = 24000
	return w
}
