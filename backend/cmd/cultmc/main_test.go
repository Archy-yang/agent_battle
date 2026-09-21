package main

// cultmc 测试：标定台的纯函数逻辑（统计 helpers、判定 helpers）与
// 聚合账本的守恒校验。场景本体由 go run ./cmd/cultmc 手动验证——
// 命题判定依赖长程动力学（数百至上千 tick），单元测试只锁可快速
// 复现的不变量。

import (
	"testing"

	"agentbattle/backend/cult"
)

func TestMedianAndQuantile(t *testing.T) {
	xs := []int64{5, 1, 3, 9, 7} // 升序 1,3,5,7,9
	if got := medianOf(xs); got != 5 {
		t.Errorf("medianOf: got %d, want 5", got)
	}
	q1, med, q3 := iqrOf(xs)
	if q1 != 3 || med != 5 || q3 != 7 {
		t.Errorf("iqrOf: got (%d,%d,%d), want (3,5,7)", q1, med, q3)
	}
	if got := medianOf(nil); got != 0 {
		t.Errorf("medianOf(nil) = %d, want 0", got)
	}
	// 偶数个：中位取上中位。
	if got := medianOf([]int64{1, 2, 3, 4}); got != 3 {
		t.Errorf("medianOf even: got %d, want 3", got)
	}
}

func TestISqrt(t *testing.T) {
	cases := []struct{ n, want int64 }{
		{0, 0}, {1, 1}, {2, 1}, {4, 2}, {8, 2}, {9, 3},
		{2000000, 1414}, // 1414² = 1999396 ≤ 2000000 < 1415²
	}
	for _, c := range cases {
		if got := iSqrt(c.n); got != c.want {
			t.Errorf("iSqrt(%d) = %d, want %d", c.n, got, c.want)
		}
	}
}

func TestEuclidDist(t *testing.T) {
	a := compVec{1000, 0, 0, 0, 0}
	b := compVec{0, 1000, 0, 0, 0}
	// sqrt(1e6 + 1e6) = 1414（floor）
	if got := euclidDist(a, b); got != 1414 {
		t.Errorf("euclidDist = %d, want 1414", got)
	}
	if got := euclidDist(a, a); got != 0 {
		t.Errorf("euclidDist(self) = %d, want 0", got)
	}
}

func TestSpearman(t *testing.T) {
	if r := spearman1000([]int64{1, 2, 3, 4}, []int64{10, 20, 30, 40}); r != 1000 {
		t.Errorf("spearman(单调升) = %d, want 1000", r)
	}
	if r := spearman1000([]int64{1, 2, 3, 4}, []int64{40, 30, 20, 10}); r != -1000 {
		t.Errorf("spearman(单调降) = %d, want -1000", r)
	}
	// 完全并列 → ρ = 0.5（并列平均秩下的解析值）。
	if r := spearman1000([]int64{1, 2, 3, 4}, []int64{7, 7, 7, 7}); r != 500 {
		t.Errorf("spearman(并列) = %d, want 500", r)
	}
}

func TestNonDecreasing(t *testing.T) {
	if !nonDecreasing([]int64{1, 1, 2, 5}) {
		t.Error("nonDecreasing(1,1,2,5) 应为 true")
	}
	if nonDecreasing([]int64{5, 2, 2}) {
		t.Error("nonDecreasing(5,2,2) 应为 false")
	}
	if !nonDecreasing(nil) {
		t.Error("nonDecreasing(nil) 应为 true")
	}
}

// TestP1AConservationAggregation（P4 聚合账本）：跑一小段场景 A，
// 账本必须零违例且逐 tick 计数。
func TestP1AConservationAggregation(t *testing.T) {
	acc := &accounting{}
	res := runP1A(1, 80, acc)
	if acc.violations != 0 {
		t.Errorf("80 tick 内守恒违例 %d 次（最大残差 %d）", acc.violations, acc.maxResidual)
	}
	if acc.ticks != 80 {
		t.Errorf("账本 tick 计数 = %d, want 80", acc.ticks)
	}
	if res.intake0 <= 0 {
		t.Errorf("初始纳气速率应 > 0: got %d", res.intake0)
	}
}

// TestRigRoundAReverseOnlyFromDantian：轨道甲的逆流段必须被引擎接受
// （发起者丹田是唯一纯汇点），且确实产生做功代价——这是全部场景的
// 前提语义。
func TestRigRoundAReverseOnlyFromDantian(t *testing.T) {
	r := newRig(1)
	acc := &accounting{}
	// 丹田预置一点气，保证逆流有源。
	dan := &r.w.Character.BodyGraph.Acupoints[r.dan]
	dan.Qi[cult.Wood] = 5000

	reps := runRound(r, acc, []cult.Ops{r.absorbOps(0, flow(r.ring[5], 500, true))})
	if len(reps[0].Transfers) != 1 {
		t.Fatalf("应有 1 条传输报告: %d", len(reps[0].Transfers))
	}
	tr := reps[0].Transfers[0]
	if !tr.Reverse || tr.Rejected {
		t.Fatal("丹田发起的逆流不应被拒绝")
	}
	if tr.Actual[cult.Wood] == 0 {
		t.Fatal("逆流应有实际流量")
	}
	if tr.WorkCost[cult.Wood] <= 0 {
		t.Errorf("逆流应产生做功代价: got %d", tr.WorkCost[cult.Wood])
	}
}

// TestCyclesDeterministic：同 seed 两套世界跑同样 20 轮轨道甲，
// 终态逐位一致（可重放性）。
func TestCyclesDeterministic(t *testing.T) {
	run := func() [cult.ElementCount]int64 {
		r := newRig(7)
		acc := &accounting{}
		r.prefill(acc, 40, 500)
		for i := 0; i < 20; i++ {
			runRound(r, acc, r.roundAProp(300))
		}
		return r.w.Character.BodyGraph.Acupoints[r.dan].Qi
	}
	a, b := run(), run()
	if a != b {
		t.Errorf("同 seed 重放不一致: %v vs %v", a, b)
	}
}
