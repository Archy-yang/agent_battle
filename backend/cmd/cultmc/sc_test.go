package main

// sc_test.go —— SC 角色差异性场景（WP-M2-4）的批测：轨道功法的静态
// 合法性（编译成环 + 逐穴进出平衡）、同 seed 逐位重放、守恒归账，以及
// 全矩阵单 seed 批测的测量健康性（全格可筑基、零损伤）。命题级结论
// （最优轨道是否随画像变化）由 go run ./cmd/cultmc -only sc 的报告承载，
// 不在本测试里复判——它是测量结果，不是不变量。

import (
	"testing"

	"agentbattle/backend/ctmatch"
	"agentbattle/backend/cult"
)

// scHasCycle 判定一份协议的引导边集是否含有向环（ctmatch.topoCycle 的
// 本地镜像：该函数未导出，而 TopologyClosure=1 是四条轨道可被判定链
// 筑基的前提——不成环的功法永远不可筑基，矩阵失去意义）。
func scHasCycle(p *ctmatch.Protocol) bool {
	type edge struct{ from, to int }
	var edges []edge
	for _, st := range p.Steps {
		if st.Kind == ctmatch.StepGuide {
			edges = append(edges, edge{st.From, st.To})
		}
	}
	color := map[int]int{}
	var dfs func(u int) bool
	dfs = func(u int) bool {
		color[u] = 1
		for _, e := range edges {
			if e.from != u {
				continue
			}
			switch color[e.to] {
			case 0:
				if dfs(e.to) {
					return true
				}
			case 1:
				return true
			}
		}
		color[u] = 2
		return false
	}
	for _, e := range edges {
		if color[e.from] == 0 && dfs(e.from) {
			return true
		}
	}
	return false
}

// TestSCOrbitProtocols：四条轨道 × 四画像的功法全部可编译，且引导路径
// 成环（判定链 TopologyClosure 前提）；引导流量逐穴进出平衡（吸收点
// 青关除外）——失衡的轨道会以与画像无关的机械过载失稳，淹没角色对照
// （开发期实测教训：青关旁支全量回灌把中庭灌到 LR>1 失稳损伤）。
func TestSCOrbitProtocols(t *testing.T) {
	w := scWorld(0)
	g := &w.Character.BodyGraph
	for c := range scChars {
		for o := range scOrbits {
			p, err := ctmatch.Compile([]byte(scOrbitYAML(c, o, g, scCycleBudget, 0)), g)
			if err != nil {
				t.Fatalf("c%d o%d 编译失败: %v", c, o, err)
			}
			if !scHasCycle(p) {
				t.Fatalf("c%d o%d 引导路径不成环（TopoClosure 恒 0）", c, o)
			}
			// 进出平衡（按段声明量计；同穴多段累计；逆流段按声明方向计）。
			net := map[int]int64{}
			for _, sg := range scOrbits[o].segs {
				net[int(sg[0])] -= sg[2]
				net[int(sg[1])] += sg[2]
			}
			for pt, v := range net {
				name := g.Acupoints[pt].Name
				if name == "青关" {
					continue // 吸收点：纳气自节流，不要求平衡
				}
				if v != 0 {
					t.Errorf("o%d %s 引导进出不平衡: %+d（过载失稳风险）", o, name, v)
				}
			}
		}
	}
}

// TestScanCDeterministic：同 seed 两次完整判定链逐位一致（全部标量场）。
func TestScanCDeterministic(t *testing.T) {
	acc := &accounting{}
	a := runScanC(0, 0, scSeedBase, acc, 0)
	b := runScanC(0, 0, scSeedBase, acc, 0)
	if a != b {
		t.Errorf("同 seed 重放不一致:\n%+v\n%+v", a, b)
	}
}

// TestScanCConservation：判定链会话 tick 全部归账，零违例。
func TestScanCConservation(t *testing.T) {
	acc := &accounting{}
	runScanC(0, 0, scSeedBase, acc, 0)
	runScanC(3, 2, scSeedBase+320, acc, 0)
	if acc.violations != 0 {
		t.Errorf("守恒违例 %d 次（最大残差 %d）", acc.violations, acc.maxResidual)
	}
	if acc.ticks != 2*scBudgetPerRun {
		t.Errorf("账本 tick 计数 = %d, want %d", acc.ticks, 2*scBudgetPerRun)
	}
}

// TestScanCMatrixBatch：全矩阵单 seed 批测（主工作点）。0.8.0 环经济学
// （B7 注记 2026-10-07）下的预期形态：
//   - 全环 O0 / 捷径轮 O1（教学档热预算内）：可筑基、零损伤、零未知异常
//     ——时标应变量在两轨上健康；
//   - 重逆流轨道 O2 逆推轮 / O3 绕土府轮：教学档即走火（损伤 > 0、
//     青关热越过灼热线）——绕土府短环的热代价正是裁决要的经济结构，
//     不再是测量混淆；其筑基时标按未达/延迟处理；
//   - 全部格子守恒零违例、丹田不枯竭。
//
// 命题判定见报告（-only sc）。
func TestScanCMatrixBatch(t *testing.T) {
	old := scRingCond
	scRingCond = scCondMain
	defer func() { scRingCond = old }()
	acc := &accounting{}
	for c := range scChars {
		for o := range scOrbits {
			r := runScanC(c, o, scSeedBase+uint64(c)*100+uint64(o)*10, acc, 0)
			if r.depleted {
				t.Errorf("c%d o%d 丹田枯竭", c, o)
			}
			switch o {
			case 0, 1:
				if r.estTick < 0 {
					t.Errorf("c%d(%s) o%d 未筑基——热预算内轨道的时标应变量失效", c, scChars[c].name, o)
				}
				if r.damage != 0 {
					t.Errorf("c%d o%d 损伤 %d 次——热预算内轨道不应走火", c, o, r.damage)
				}
				if r.anomalies != 0 {
					t.Errorf("c%d o%d 未知异常升级 %d 次", c, o, r.anomalies)
				}
			case 2, 3:
				if r.damage == 0 {
					t.Errorf("c%d o%d 教学档零损伤——重逆流轨道的热代价（走火）未显现", c, o)
				}
				// 走火伴随未知异常升级（损伤无 Guard 可解释）——0.8.0 下这是
				// 裁决要的世界响应，由会话「观察继续」接管，不视为测量故障。
			}
		}
	}
	if acc.violations != 0 {
		t.Errorf("守恒违例 %d（最大残差 %d）", acc.violations, acc.maxResidual)
	}
	if acc.ticks != int64(len(scChars)*len(scOrbits))*scBudgetPerRun {
		t.Errorf("tick 计数 %d 与矩阵规模不符", acc.ticks)
	}
}

// TestSCCharAffRotation：画像构造的自检——偏性形态跨画像不变、仅方向
// 轮转；均衡恒等 1000。这是「画像间差异可纯归于偏行方向」的归因前提。
func TestSCCharAffRotation(t *testing.T) {
	eq := scCharAff(-1)
	for e := 0; e < cult.ElementCount; e++ {
		if eq[e] != 1000 {
			t.Fatalf("均衡画像含非 1000 项: %v", eq)
		}
	}
	for bias := 0; bias < cult.ElementCount; bias++ {
		a := scCharAff(bias)
		for e := 0; e < cult.ElementCount; e++ {
			if a[e] != scCharTemplate[(e-bias+5)%5] {
				t.Fatalf("偏行 %d 轮转错位: %v", bias, a)
			}
		}
		var sorted []int64
		for _, v := range a {
			sorted = append(sorted, v)
		}
		// 与模板同多重集（形态不变）。
		if !sameMultiset(sorted, scCharTemplate[:]) {
			t.Fatalf("偏行 %d 形态漂移: %v", bias, a)
		}
	}
}

func sameMultiset(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	ca := map[int64]int{}
	cb := map[int64]int{}
	for i := range a {
		ca[a[i]]++
		cb[b[i]]++
	}
	for k, v := range ca {
		if cb[k] != v {
			return false
		}
	}
	return true
}
