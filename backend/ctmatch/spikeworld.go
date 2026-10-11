package ctmatch

// spikeworld.go —— SPIKE 世界构造的唯一定义点（原 cmd/cultspike/main.go
// 的 buildWorld，抽为共享构造函数供 cultspike 与 /api/cult 会话复用，
// 行为不变）。
//
// 设计目标：存在一条"新手粗档位即可达成"的稳定吐纳路径，但不给任何提示。
//   - 青关：主纳穴。开度大开、通量宽（8000），混合引流 large（每行 1280）
//     足以对冲 trace/small/medium 各档吸收的反压平衡——单穴"吸+引"即可闭环。
//   - 下丹田：巨容储库（200000）与主环的泵；驱动容量全图唯一显著主导
//     （24000，0.5.0 核点判据 cult.CorePointOf——定稿裁决①，逆流外推唯它
//     推得动）。
//   - 主环拓扑（修订包 A1 v0.2，取代旧归流树）：
//
//	下丹田 → 土府 → 中庭 → 阳枢 → 神庭 → 青关 → 归下丹田
//
//     环向 = 天然流向（顺流省力）；旁支 青关/金门/玄泉 → 中庭 维持归流
//     枢纽角色。引气入体不变：青关 → 丹田仍为天然向，M1 开局体验原样。
//   - 神庭：脆穴（容量 3000、开度 0.90）。在它上面用 large 吸收会迅速过载
//     受损——为 M2 的"不适→自主调整"提供真实触发点。
//   - 环境五行接近均衡（木略盛）：混合引流才可能对冲混合吸收，
//     避免单行淤积成为纯数值陷阱。
//   - 金门开度仅 0.30（近闭）：吸收前先感开合，是感知信息的真实用途。
//
// 本构造是真值表：只进转录头/调试视图供研究者查证，绝不进 LLM/玩家载荷。

import "agentbattle/backend/cult"

// SpikeWorld 构造 SPIKE 修炼世界（8 穴位 / 9 经脉 / 五行近均衡环境）。
//
// 驱动容量画像（0.5.0，定稿阶段二范围 1 / 评审稿 §4.4 表格 [v0.1 参数]）：
// 下丹田 24000（全图唯一显著主导——高于其余每一穴 2 倍以上，0.5.0 核点
// 判据 CorePointOf 的唯一结构依据，逆流外推唯它推得动）；普通穴 8000
// （≥ 0.8× 各自经脉通量上限 8000×0.8=6400——large 档引流不被驱动容量
// 限幅，既有玩法手感不变）；驱动效率丹田 950 / 普通穴 850（评审稿 §10.1
// 做功除数，[v0.1 参数]）。
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
			// 驱动画像（0.5.0）：普通穴默认档，见函数头注释。
			DriveCapacity:   8000,
			DriveEfficiency: 850,
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
				// 丹田亲和画像 [v0.1 参数]（SA6 P3 调参）：温和逐元素梯度，
				// 主行（木，CharacterAffinity 主导行）1000，其余 920~950 不等——
				// 给 D3 筛选"留与散"提供丹田侧差分（原先全 1000 无差可筛，
				// 炼化吸引子 0.271 爬不进中档）。
				pt("下丹田", [cult.ElementCount]int64{1000, 930, 950, 940, 920}, 200000, 20000, 1000, 800, 950), // 0 汇点
				pt("青关", [cult.ElementCount]int64{1200, 750, 900, 850, 1000}, 10000, 8000, 400, 800, 950),    // 1 主纳
				pt("阳枢", [cult.ElementCount]int64{1100, 900, 800, 800, 800}, 6000, 4000, 500, 500, 900),      // 2
				pt("土府", [cult.ElementCount]int64{800, 850, 1250, 750, 800}, 8000, 5000, 800, 650, 950),      // 3
				pt("金门", [cult.ElementCount]int64{800, 750, 800, 1300, 850}, 5000, 4000, 600, 300, 920),      // 4 近闭
				pt("玄泉", [cult.ElementCount]int64{900, 800, 800, 800, 1200}, 7000, 4000, 700, 700, 940),      // 5
				pt("神庭", [cult.ElementCount]int64{1050, 1050, 800, 800, 800}, 3000, 3000, 300, 900, 880),     // 6 脆
				// 中庭（修订包 B1 参数画像）：高吞吐（6000，普通穴之上）、
				// 低 Retention（300）、Affinity[5] ≈ 1.0 不偏任何一行（五行反应炉，
				// B7）、base_stability 高。特殊性全在参数取值，无 type 字段。
				pt("中庭", all(1000), 9000, 6000, 300, 750, 930), // 7 中枢
			},
			Channels: []cult.Channel{
				// 主环（修订包 A1 v0.2：环向 = 天然流向，丹田是环上的泵与储库）。
				// 下丹田→土府：[v0.1 参数] 新增（泵的出口），量级沿用原主汇
				// 中庭→丹田（850/12000）。
				{Name: "下丹田→土府", From: 0, To: 3, BaseConductance: 850, Throughput: 12000},
				// 土府→中庭：M1 原边，方向不变。
				{Name: "土府→中庭", From: 3, To: 7, BaseConductance: 700, Throughput: 6000},
				// 中庭→阳枢：原 阳枢→中庭 反向（A1 v0.2 唯一改向段），参数沿用。
				{Name: "中庭→阳枢", From: 7, To: 2, BaseConductance: 700, Throughput: 6000},
				// 阳枢→神庭：[v0.1 参数] 新增，量级沿用原 阳枢→中庭（700/6000）。
				{Name: "阳枢→神庭", From: 2, To: 6, BaseConductance: 700, Throughput: 6000},
				// 神庭→青关：M1 原边，方向不变。
				{Name: "神庭→青关", From: 6, To: 1, BaseConductance: 600, Throughput: 5000},
				// 青关→下丹田：M1 原边（主环收尾段，兼作引气入体捷径），方向不变。
				{Name: "青关→下丹田", From: 1, To: 0, BaseConductance: 950, Throughput: 8000},
				// 非环旁支（A1 v0.2 保留）：维持中庭的归流枢纽角色。
				{Name: "青关→中庭", From: 1, To: 7, BaseConductance: 700, Throughput: 6000},
				{Name: "金门→中庭", From: 4, To: 7, BaseConductance: 600, Throughput: 5000},
				{Name: "玄泉→中庭", From: 5, To: 7, BaseConductance: 700, Throughput: 6000},
			},
		},
	}
	env := cult.Environment{
		Qi:          [cult.ElementCount]int64{3600, 2400, 2400, 1800, 2400}, // 木略盛
		Activity:    [cult.ElementCount]int64{1000, 1000, 1000, 1000, 1000},
		Temperature: 1000,
	}
	w := cult.NewWorld(char, env)
	// 丹田驱动画像覆盖（0.5.0 [v0.1 参数]）：驱动容量 24000 = 普通穴 8000
	// 的 3 倍（> CoreDriveSignificance 2 倍判据——全图唯一核点）；泵的出口
	// 通量画像（1.2× 丹田吞吐 20000）。
	w.Character.BodyGraph.Acupoints[0].DriveCapacity = 24000
	w.Character.BodyGraph.Acupoints[0].DriveEfficiency = 950
	// 丹田炼化归一化锚定 [v0.1 参数]（SA6 P3 调参，SA4 根因③）：
	// MatchMax 原推导锚 = max_e AffinityEff（假设其余四行归零的完全收敛），
	// 但相生循环使木恒有 ~4%/Tick 转化为火，单行构成物理不可达——区间偏宽，
	// 炼化吸引子（实测构成 Match≈1006）被映射到 0.28。锚定改为相生循环下
	// 可达收敛构成：MatchMin=910（≈均匀构成锚）、MatchMax=1080（≈可达收敛
	// 构成 Match）。仅此穴位钉死区间，公式结构与相对性不变。
	w.Character.BodyGraph.Acupoints[0].RefineMatchMin = 910
	w.Character.BodyGraph.Acupoints[0].RefineMatchMax = 1080
	// 土府敛热画像（0.8.0，修订包 B7 注记 2026-10-07 环经济学裁决
	// [v0.1 参数]（调参起点））：土府立为周天敛热之穴——散热率 0.60
	//（全局 0.30 的 2 倍）。世界语义：经土府行转之环，土府段的沉积热
	// （传输损耗热）在此快速敛散，不长程积累；绕开土府的环必须以核点
	// 逆流段补环，逆流做功热与逆流损耗热全部沉积在共享纳气入口（青关
	// ——无敛热画像），长程热积累与走火风险随之上升。全环与短环由此
	// 各有所长（稳与快），无对所有行功方式无条件最优的单一轨道。
	// 画像层取值，底层零专名（引擎只见「某穴散热率 0.60」，宪法 §8）。
	w.Character.BodyGraph.Acupoints[3].HeatDissipation = 600
	return w
}
