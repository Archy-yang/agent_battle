package ctmatch

// spikeworld.go —— SPIKE 世界构造的唯一定义点（原 cmd/cultspike/main.go
// 的 buildWorld，抽为共享构造函数供 cultspike 与 /api/cult 会话复用，
// 行为不变）。
//
// 设计目标：存在一条"新手粗档位即可达成"的稳定吐纳路径，但不给任何提示。
//   - 青关：主纳穴。开度大开、通量宽（8000），混合引流 large（每行 1280）
//     足以对冲 trace/small/medium 各档吸收的反压平衡——单穴"吸+引"即可闭环。
//   - 下丹田：巨容汇点（200000），引流去处，30+ Tick 内不会自己顶到不适档。
//     丹田双入口拓扑（修订包 B3）：青关→下丹田为捷径（快、单属性直入），
//     中庭→下丹田为主汇（慢、多属性混合后入）；丹田无出向经脉——
//     唯一可发起逆流外推的纯汇点（修订包 A2/B1，无 type 字段）。
//   - 神庭：脆穴（容量 3000、开度 0.90）。在它上面用 large 吸收会迅速过载
//     受损——为 M2 的"不适→自主调整"提供真实触发点。
//   - 环境五行接近均衡（木略盛）：混合引流才可能对冲混合吸收，
//     避免单行淤积成为纯数值陷阱。
//   - 金门开度仅 0.30（近闭）：吸收前先感开合，是感知信息的真实用途。
//
// 本构造是真值表：只进转录头/调试视图供研究者查证，绝不进 LLM/玩家载荷。

import "agentbattle/backend/cult"

// SpikeWorld 构造 SPIKE 修炼世界（8 穴位 / 8 经脉 / 五行近均衡环境）。
func SpikeWorld() *cult.World {
	pt := func(name string, aff [cult.ElementCount]int64, cap, tp, ret int64, open, stab int64) cult.Acupoint {
		return cult.Acupoint{
			Name: name, Affinity: aff, Capacity: cap, Throughput: tp,
			// open 以千分位传入（800 = 0.8）；微标度换算 ×1000（1.0 = 1e6 微）。
			Retention: ret, Openness: open * 1000, Stability: stab,
			// BaseStability 先天资质与初值稳定一致（修订包 B1 参数画像；
			// v0.1 公式未使用，仅携带）。
			BaseStability: stab,
			Activity:      [cult.ElementCount]int64{1000, 1000, 1000, 1000, 1000},
		}
	}
	all := func(v int64) [cult.ElementCount]int64 {
		return [cult.ElementCount]int64{v, v, v, v, v}
	}
	char := cult.Character{
		Affinity:   [cult.ElementCount]int64{1250, 800, 900, 750, 1100},
		Control:    700,
		Perception: 500,
		BodyGraph: cult.BodyGraph{
			Acupoints: []cult.Acupoint{
				pt("下丹田", all(1000), 200000, 20000, 1000, 800, 950),                                       // 0 汇点
				pt("青关", [cult.ElementCount]int64{1200, 750, 900, 850, 1000}, 10000, 8000, 400, 800, 950), // 1 主纳
				pt("阳枢", [cult.ElementCount]int64{1100, 900, 800, 800, 800}, 6000, 4000, 500, 500, 900),   // 2
				pt("土府", [cult.ElementCount]int64{800, 850, 1250, 750, 800}, 8000, 5000, 800, 650, 950),   // 3
				pt("金门", [cult.ElementCount]int64{800, 750, 800, 1300, 850}, 5000, 4000, 600, 300, 920),   // 4 近闭
				pt("玄泉", [cult.ElementCount]int64{900, 800, 800, 800, 1200}, 7000, 4000, 700, 700, 940),   // 5
				pt("神庭", [cult.ElementCount]int64{1050, 1050, 800, 800, 800}, 3000, 3000, 300, 900, 880),  // 6 脆
				// 中庭（修订包 B1 参数画像）：高吞吐（6000，普通穴之上）、
				// 低 Retention（300）、Affinity[5] ≈ 1.0 不偏任何一行（五行反应炉，
				// B7）、base_stability 高。特殊性全在参数取值，无 type 字段。
				pt("中庭", all(1000), 9000, 6000, 300, 750, 930), // 7 中枢
			},
			Channels: []cult.Channel{
				{Name: "青关→下丹田", From: 1, To: 0, BaseConductance: 950, Throughput: 8000},
				{Name: "青关→中庭", From: 1, To: 7, BaseConductance: 700, Throughput: 6000},
				{Name: "中庭→下丹田", From: 7, To: 0, BaseConductance: 850, Throughput: 12000},
				{Name: "阳枢→中庭", From: 2, To: 7, BaseConductance: 700, Throughput: 6000},
				{Name: "土府→中庭", From: 3, To: 7, BaseConductance: 700, Throughput: 6000},
				{Name: "金门→中庭", From: 4, To: 7, BaseConductance: 600, Throughput: 5000},
				{Name: "玄泉→中庭", From: 5, To: 7, BaseConductance: 700, Throughput: 6000},
				{Name: "神庭→青关", From: 6, To: 1, BaseConductance: 600, Throughput: 5000},
			},
		},
	}
	env := cult.Environment{
		Qi:          [cult.ElementCount]int64{3600, 2400, 2400, 1800, 2400}, // 木略盛
		Activity:    [cult.ElementCount]int64{1000, 1000, 1000, 1000, 1000},
		Temperature: 1000,
	}
	return cult.NewWorld(char, env)
}
