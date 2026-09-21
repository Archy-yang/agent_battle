package cult

// refinement.go —— 气炼化度（修订包 D2：导出量，绝不落存储字段）。
//
// 实现口径（对修订包 D2 的实现简化，意图一致——轨道影响通过 D3 筛选
// 改变构成而涌现，不进公式）：
//
//	AffinityEff[i,e] = CharacterAffinity[e] × AcupointAffinity[i,e]
//	                   （AcupointAffinity 取 refineAffinity：含 Adaptation 与
//	                     (1-Damage)——D2"身体变化即重算"的落点）
//	Match[i]         = Σ_e NatureRatio[i,e] × AffinityEff[i,e]
//	Refine[i]        = clamp((Match[i] - MatchMin) / (MatchMax - MatchMin), 0, 1)
//	MatchMin         = (Σ_e AffinityEff[i,e]) / 5   （均匀构成）
//	MatchMax         = max_e AffinityEff[i,e]        （完全收敛）
//
// 全程定点整数；Refinement=0 ⇒ D4 全部乘子 = 1（无炼化时行为与旧公式一致）。
// 空穴（无气）炼化度为 0；五行亲和无梯度（MatchMax == MatchMin）时
// 没有可收敛的方向，亦为 0。

// DeriveRefinement 返回穴位 i 的炼化度（千分位 0~1000），供感知层与会话层
// 调用。纯函数：不写 World；同一状态任意次重算结果一致（T4）。
func DeriveRefinement(w *World, i int) int64 {
	if i < 0 || i >= len(w.Character.BodyGraph.Acupoints) {
		return 0
	}
	return refineOf(&w.Character.BodyGraph.Acupoints[i], &w.Character)
}

// refineOf 是 DeriveRefinement 的内部形态（Advance 的 Tick 开始快照同用）。
func refineOf(a *Acupoint, c *Character) int64 {
	load := a.load()
	if load <= 0 {
		return 0
	}
	var affEff [ElementCount]int64
	sum, max := int64(0), int64(0)
	for e := 0; e < ElementCount; e++ {
		affEff[e] = fpMul(c.Affinity[e], a.refineAffinity(e))
		sum += affEff[e]
		if affEff[e] > max {
			max = affEff[e]
		}
	}
	// MatchMin = (Σ_e AffinityEff) / 5：对亲和值（千分位）求普通整数均值，
	// 不用 fpDiv（5 是计数，不是千分位标度量）。
	matchMin := (sum + ElementCount/2) / ElementCount
	if max <= matchMin {
		return 0
	}
	match := int64(0)
	for e := 0; e < ElementCount; e++ {
		match += fpMul(fpDiv(a.Qi[e], load), affEff[e])
	}
	return fpClamp(fpDiv(match-matchMin, max-matchMin), 0, One)
}
