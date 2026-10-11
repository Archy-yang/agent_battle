package main

// scfeed_test.go —— SF 旁支喂养场景（修订包「保留的非环边」注记 2026-10-11
// 旁支角色裁决验证）的批测：协议静态合法性、同 seed 逐位重放、守恒归账、
// 全矩阵单 seed 批测的测量健康性（零损伤、零未知异常、无过载失控、无
// 枯竭）。命题级结论（旁支裁决成立/作废）由 go run ./cmd/cultmc -only
// scfeed 的报告承载，不在本测试里复判——它是测量结果，不是不变量。

import (
	"reflect"
	"strings"
	"testing"

	"agentbattle/backend/ctmatch"
	"agentbattle/backend/cult"
)

// TestFeedProtoCompiles：喂养/对照两轨功法在两画像下全部可编译；引导路径
// 含主环有向环（判定链 TopologyClosure 语义的前提）；喂养轮的泵段让位与
// 旁支注入段落在声明的穴位上。
func TestFeedProtoCompiles(t *testing.T) {
	for cs, charIdx := range feedCharIdx {
		w := feedWorld(charIdx, 0)
		g := &w.Character.BodyGraph
		for _, feed := range []bool{true, false} {
			p, err := ctmatch.Compile([]byte(feedProtoYAML(charIdx, feed, feedTeachTick, g)), g)
			if err != nil {
				t.Fatalf("cs%d feed=%v 编译失败: %v", cs, feed, err)
			}
			if !scHasCycle(p) {
				t.Fatalf("cs%d feed=%v 引导路径不成环", cs, feed)
			}
			yaml := feedProtoYAML(charIdx, feed, feedTeachTick, g)
			hasJin := strings.Contains(yaml, "absorb: {target: 金门")
			hasSide := strings.Contains(yaml, "guide: {from: 金门, to: 中庭")
			if feed != hasJin || feed != hasSide {
				t.Fatalf("cs%d feed=%v 旁支动作缺席/多余（absorb@金门=%v, guide 金门→中庭=%v）",
					cs, feed, hasJin, hasSide)
			}
		}
	}
}

// TestFeedJinMature：喂养世界的金门修行有成口径到位（容量/通量/开度/稳定），
// 且不触碰真值表世界（SpikeWorld 原样）与环上六穴。
func TestFeedJinMature(t *testing.T) {
	w := feedWorld(0, 0)
	g := &w.Character.BodyGraph
	idx := scIdx(g)
	jin := &g.Acupoints[idx["金门"]]
	if jin.Capacity != 30000 || jin.Throughput != 10000 ||
		jin.Openness != cult.MicroOne || jin.Stability != 950 {
		t.Fatalf("金门修行有成口径缺失: cap=%d tp=%d open=%d stab=%d",
			jin.Capacity, jin.Throughput, jin.Openness, jin.Stability)
	}
	base := ctmatch.SpikeWorld()
	bg := &base.Character.BodyGraph
	for i := range bg.Acupoints {
		a := &bg.Acupoints[i]
		if a.Name == "金门" {
			if a.Capacity == 30000 {
				t.Fatalf("SpikeWorld 真值表被改动（金门容量）")
			}
		}
	}
	if base.Env.Qi != w.Env.Qi {
		// 现行环境变体 = 真值表环境，必须逐位一致。
		t.Fatalf("现行环境变体偏离真值表: %v vs %v", w.Env.Qi, base.Env.Qi)
	}
	// 玄泉不在本批测范围，必须保持真值表画像。
	bidx := scIdx(bg)
	xq := &g.Acupoints[idx["玄泉"]]
	bxq := &bg.Acupoints[bidx["玄泉"]]
	if xq.Capacity != bxq.Capacity || xq.Openness != bxq.Openness {
		t.Fatalf("玄泉画像被误改: cap %d→%d open %d→%d",
			bxq.Capacity, xq.Capacity, bxq.Openness, xq.Openness)
	}
}

// TestFeedDeterministic：同 seed 两次完整格逐位一致（全部标量场）。
func TestFeedDeterministic(t *testing.T) {
	acc := &accounting{}
	a := runFeedCell(3, 0, true, feedSeedBase, acc)
	b := runFeedCell(3, 0, true, feedSeedBase, acc)
	if !reflect.DeepEqual(a, b) {
		t.Errorf("同 seed 重放不一致:\n%+v\n%+v", a, b)
	}
}

// TestFeedConservation：格会话 tick 全部归账，零违例。
func TestFeedConservation(t *testing.T) {
	acc := &accounting{}
	runFeedCell(3, 0, true, feedSeedBase, acc)
	runFeedCell(0, 2, false, feedSeedBase+1052, acc)
	if acc.violations != 0 {
		t.Errorf("守恒违例 %d 次（最大残差 %d）", acc.violations, acc.maxResidual)
	}
	// 观测基准 tick 在 acc 之外直推（与 runScanC 同口径），不计入账本。
	if acc.ticks != 2*feedBudget() {
		t.Errorf("账本 tick 计数 = %d, want %d", acc.ticks, 2*feedBudget())
	}
}

// TestFeedMatrixBatch：全矩阵单 seed 批测（工作点：注入 600/元素 + 泵段
// 让位 700，扫频依据见 scfeed.go 头注）。测量健康性口径：
//   - 全部格子（2 画像 × 2 轨道 × 3 环境）零损伤、零未知异常升级、
//     丹田不枯竭——共模机械/热事件不得淹没画像对照；
//   - 喂养轮的旁支链真实发生（金门金属纳入 > 0、金门→中庭金属输送 > 0、
//     中庭→阳枢金属带走 > 输送——环流确实把注入的气带走了）；
//   - 金门/中庭负载比峰值不失控（< 1.1——短暂越线即应变侵蚀，扫频教训）。
func TestFeedMatrixBatch(t *testing.T) {
	acc := &accounting{}
	for cs, charIdx := range feedCharIdx {
		for trk, feed := range []bool{true, false} {
			for ei := range feedEnvs {
				seed := feedSeedBase + uint64(cs)*1000 + uint64(trk)*100 + uint64(ei)*10
				r := runFeedCell(charIdx, ei, feed, seed, acc)
				if r.depleted {
					t.Errorf("cs%d(%s) e%d feed=%v 丹田枯竭", cs, scChars[charIdx].name, ei, feed)
				}
				if r.damage != 0 {
					t.Errorf("cs%d e%d feed=%v 损伤 %d 次——工作点失稳", cs, ei, feed, r.damage)
				}
				if r.anomalies != 0 {
					t.Errorf("cs%d e%d feed=%v 未知异常升级 %d 次", cs, ei, feed, r.anomalies)
				}
				if r.midPeakRatio >= 1100 || r.jinPeakRatio >= 1100 {
					t.Errorf("cs%d e%d feed=%v 负载比失控（中庭 %d / 金门 %d）",
						cs, ei, feed, r.midPeakRatio, r.jinPeakRatio)
				}
				if feed {
					if r.jinMetalIn <= 0 || r.sideMetal <= 0 {
						t.Errorf("cs%d e%d 喂养链不通：金门纳入 %d、注入 %d",
							cs, ei, r.jinMetalIn, r.sideMetal)
					}
					if r.midOutMetal <= r.sideMetal {
						t.Errorf("cs%d e%d 环流未带走注入（中庭→阳枢 %d ≤ 注入 %d）",
							cs, ei, r.midOutMetal, r.sideMetal)
					}
				}
			}
		}
	}
	if acc.violations != 0 {
		t.Errorf("守恒违例 %d（最大残差 %d）", acc.violations, acc.maxResidual)
	}
	if acc.ticks != int64(len(feedCharIdx)*2*len(feedEnvs))*feedBudget() {
		t.Errorf("tick 计数 %d 与矩阵规模不符", acc.ticks)
	}
}
