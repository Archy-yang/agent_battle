package cult

// heat_test.go —— 热管线慢通道法则测试（0.8.0，修订包 B7 注记 2026-10-07
// 环经济学裁决）。热管线：产生（传输损耗/转化损耗/驱动做功/逆流做功）
// → 沉积（heatAccum / reverseHeatAccum）→ 衰减（逐穴散热率画像）→
// ThermalLoad → 应变（HeatLoadStrain）→ 稳定损伤。本文件锁定：
//
//	①无伤死区：热负荷低于 HeatStrainFloor（灼热档）不产生应变——常规
//	  行功（温热/明显发热档）零回归；
//	②灼热走火：长程热积累越过死区后，应变随热负荷单调上升，稳定性被
//	  持续侵蚀（走火风险的慢响应面）；
//	③敛热画像：散热率画像加成使同强度行功的热积累成比例下降（0 = 全局
//	  默认的画像钩子语义，特殊性落在参数画像，底层零专名）。
//
// 全程定点整数；同 seed 重放逐位一致。

import "testing"

// heatRingWorld 构造 N 穴单环行功世界（partjRingWorld 的散热画像参数化
// 变体）：diss[i] 为各穴散热率画像（0 = 未画像）。
func heatRingWorld(diss []int64) *World {
	w := partjRingWorld()
	for i := range w.Character.BodyGraph.Acupoints {
		if i < len(diss) {
			w.Character.BodyGraph.Acupoints[i].HeatDissipation = diss[i]
		}
	}
	return w
}

// TestHeatLoadStrainDeadZone（法则①）：温和行功（热负荷在灼热档以下）
// 长程运行——热负荷应变恒 0，稳定性不因热侵蚀。
func TestHeatLoadStrainDeadZone(t *testing.T) {
	w := heatRingWorld(nil)
	// 300/元素 温和环流：损耗+转化热沉积小，散热 0.3 下稳态 ~250（温热档）。
	for i := 0; i < 200; i++ {
		w.Ops = boundaryOpsSmall()
		rep := Advance(w)
		for pi := range rep.Points {
			if rep.Points[pi].HeatLoadStrain != 0 {
				t.Fatalf("tick %d 穴 %d: 温热档热负荷不应产生应变: %d",
					i, pi, rep.Points[pi].HeatLoadStrain)
			}
			if rep.Points[pi].ThermalLoad >= HeatStrainFloor {
				t.Fatalf("测试前提失效：tick %d 热负荷 %d 已入灼热档", i, rep.Points[pi].ThermalLoad)
			}
		}
	}
}

// boundaryOpsAmt 是三穴环的等量环流请求（每 Tick 每脉 amt/元素）。
func boundaryOpsAmt(amt int64) Ops {
	a := [ElementCount]int64{amt, amt, amt, amt, amt}
	return Ops{Requests: []FlowRequest{
		{Channel: 0, Amount: a}, {Channel: 1, Amount: a}, {Channel: 2, Amount: a},
	}}
}

// TestHeatLoadStrainWalkaway（法则②）：猛行功（热负荷深入灼热档）——
// 热负荷应变 > 0；持续走火下稳定性被侵蚀直至损伤（走火风险的慢响应
// 面，B7「可持续强度受限」）。注意慢通道一拍滞后：本 Tick 应变读数
// 对应上一 Tick 结算的热负荷（§17 实现口径）。
func TestHeatLoadStrainWalkaway(t *testing.T) {
	w := heatRingWorld(nil)
	sawStrain := false
	var prevTL int64
	minStab := int64(One)
	for i := 0; i < 150; i++ {
		w.Ops = boundaryOpsAmt(2000) // 2000/元素 猛行功：热沉积深入灼热档
		rep := Advance(w)
		p := &rep.Points[0]
		if p.HeatLoadStrain > 0 {
			sawStrain = true
		}
		if prevTL >= HeatStrainFloor && prevTL < HeatStrainFloor+2000 && p.HeatLoadStrain == 0 &&
			p.ThermalLoad >= prevTL {
			t.Fatalf("tick %d: 灼热档热负荷 %d 且未回落，应变不应为零", i, prevTL)
		}
		prevTL = p.ThermalLoad
		if p.Stability < minStab {
			minStab = p.Stability
		}
	}
	if !sawStrain {
		t.Fatal("测试前提失效：猛行功未进入灼热档（热负荷应变从未触发）")
	}
	if minStab > 400 {
		t.Fatalf("走火未把稳定性压到损伤带: 最低稳定 %d", minStab)
	}
}

// TestHeatDissipationProfile（法则③）：同强度行功、同 seed，高散热画像
// 穴的热积累严格低于默认画像穴；0 画像与全局默认逐位一致（画像钩子的
// 缺省语义）。敛热之穴的世界语义（B7）落在参数画像上。
func TestHeatDissipationProfile(t *testing.T) {
	// 同一世界两个穴（B/C 画像相同、在环上对称位置），只给 B 加散热画像。
	w := heatRingWorld([]int64{0, 0, 0})
	w.Character.BodyGraph.Acupoints[1].HeatDissipation = 600
	var tlB, tlC int64
	for i := 0; i < 80; i++ {
		w.Ops = boundaryOps()
		rep := Advance(w)
		tlB += rep.Points[1].ThermalLoad
		tlC += rep.Points[2].ThermalLoad
	}
	if tlB >= tlC {
		t.Fatalf("高散热画像穴的热积累应低于默认画像穴: B=%d vs C=%d", tlB, tlC)
	}
	// 缺省语义：0 画像 = 全局默认——两世界（全 0 画像 vs 全未画像）逐位一致。
	w1 := heatRingWorld([]int64{0, 0, 0})
	w2 := heatRingWorld(nil)
	for i := 0; i < 50; i++ {
		w1.Ops = boundaryOps()
		w2.Ops = boundaryOps()
		r1, r2 := Advance(w1), Advance(w2)
		for pi := range r1.Points {
			if r1.Points[pi].ThermalLoad != r2.Points[pi].ThermalLoad {
				t.Fatalf("tick %d 穴 %d: 0 画像应与全局默认逐位一致: %d vs %d",
					i, pi, r1.Points[pi].ThermalLoad, r2.Points[pi].ThermalLoad)
			}
		}
	}
}

// boundaryOpsSmall 是三穴环的温和环流请求（每 Tick 每脉 300/元素）。
func boundaryOpsSmall() Ops {
	return Ops{Requests: []FlowRequest{
		{Channel: 0, Amount: [ElementCount]int64{300, 300, 300, 300, 300}},
		{Channel: 1, Amount: [ElementCount]int64{300, 300, 300, 300, 300}},
		{Channel: 2, Amount: [ElementCount]int64{300, 300, 300, 300, 300}},
	}}
}
