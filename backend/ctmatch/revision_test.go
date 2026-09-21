package ctmatch

// revision_test.go —— stage_3 修订包 Part C 会话层验收
// （docs/specs/2026-09-21-stage3-revision-circulation-refinement.md）：
//
//	1. 丹田逆流意图 → 引擎收到 Reverse 传输（含做功代价入守恒账），回执已生效且零数字
//	2. 非丹田发起逆流 → "此路不通"回执（定性补语），引擎未收到任何逆流请求
//	3. 含逆流边的协议可编译并试运行：Runtime 产出 Reverse 请求，TrialMonitor 沿路径记账
//	4. 非法边序列（无经脉可达 / 非汇点逆流）编译拒绝
//	5. rule_version 上视图；非 debug 视图无新增真值键

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"agentbattle/backend/cult"
)

// revWorld 建一个丹田预载木气的 SPIKE 世界（下丹田是唯一纯汇点，
// 逆流外推只能由它发起）。
func revWorld(seed uint64) *CultSession {
	w := SpikeWorld()
	w.Character.BodyGraph.Acupoints[0].Qi[cult.Wood] = 50000
	s := NewSession(w, seed, nil)
	Tick(s, nil) // 静息，取观测基准
	return s
}

// 1. 丹田逆流意图 → 引擎收到 Reverse 传输。
func TestReverseIntentFromDantian(t *testing.T) {
	s := revWorld(101)
	res := Tick(s, []Intent{{
		Kind: IntentGuide, From: "下丹田", To: "青关",
		Element: cult.Wood, Level: LevelMedium,
	}})
	if !res.Advanced || res.Report == nil {
		t.Fatalf("tick did not advance: %+v", res.Events)
	}
	// 引擎收到逆流传输：经脉天然向为 青关(1)→下丹田(0)，
	// 逆流则气从 To 端（下丹田）流向 From 端（青关）。
	var rev *cult.TransferReport
	for i := range res.Report.Transfers {
		tr := &res.Report.Transfers[i]
		if tr.Reverse {
			rev = tr
		}
	}
	if rev == nil {
		t.Fatalf("引擎未收到任何逆流传输: %+v", res.Report.Transfers)
	}
	if rev.From != 1 || rev.To != 0 {
		t.Errorf("逆流传输端点错误: from=%d to=%d, want 1/0", rev.From, rev.To)
	}
	if rev.Actual[cult.Wood] <= 0 {
		t.Errorf("逆流传输无实际流量: %+v", rev)
	}
	// A2：逆流做功代价入守恒账（去向=热）。
	if res.Report.Conservation.ReverseWork <= 0 {
		t.Errorf("逆流做功代价未入守恒账: %+v", res.Report.Conservation)
	}
	// 回执正常（铁律 6）且零数字。
	if len(res.Receipts) != 1 || res.Receipts[0].Outcome != WordApplied {
		t.Fatalf("逆流意图回执错误: %+v", res.Receipts)
	}
	if bad := receiptDigits(ReceiptViews(res.Receipts)); bad != "" {
		t.Errorf("回执载荷含数字 %q", bad)
	}
}

// 2. 非丹田发起逆流 → "此路不通"回执，引擎未收到逆流请求。
func TestReverseIntentFromNonSinkRejected(t *testing.T) {
	s := revWorld(103)
	// 青关→神庭：天然经脉为 神庭(6)→青关(1)，逆流发起端青关有出向经脉——拒绝。
	res := Tick(s, []Intent{{
		Kind: IntentGuide, From: "青关", To: "神庭",
		Element: cult.Wood, Level: LevelSmall,
	}})
	if !res.Advanced {
		t.Fatalf("被拒意图不应阻断引擎推进")
	}
	if len(res.Receipts) != 1 || res.Receipts[0].Outcome != WordRejected {
		t.Fatalf("应得此路不通回执: %+v", res.Receipts)
	}
	if res.Receipts[0].Detail != WhyNotBackflow {
		t.Errorf("拒绝补语 = %q, want %q", res.Receipts[0].Detail, WhyNotBackflow)
	}
	for i := range res.Report.Transfers {
		if res.Report.Transfers[i].Reverse {
			t.Error("引擎不应收到任何逆流请求")
		}
	}
	if bad := receiptDigits(ReceiptViews(res.Receipts)); bad != "" {
		t.Errorf("回执载荷含数字 %q", bad)
	}
	// 证据账本：该路记为"试过被拒"（定性）。
	if st := s.Ledger.StateOf("guide@青关→神庭"); st != TriedRejected {
		t.Errorf("被拒逆流应入账试过被拒, got %d", st)
	}
}

// 3. 含逆流边的协议可编译并试运行。
const reverseLoopYAML = `
protocol: nixiu_zhoutian_v1
actions:
  - guide: {from: 下丹田, to: 青关, element: wood, amount: 2000, duration: 1}
  - guide: {from: 青关, to: 下丹田, element: wood, amount: 2000, duration: 1}
cadence: 1
cycles: 6
validated_range:
  - channel: [下丹田, 青关]
    element: wood
    min: 0
    max: 2000
  - channel: [青关, 下丹田]
    element: wood
    min: 0
    max: 2000
`

func TestReverseEdgeProtocolCompilesAndTrials(t *testing.T) {
	s := revWorld(107)
	p, err := Compile([]byte(reverseLoopYAML), &s.World.Character.BodyGraph)
	if err != nil {
		t.Fatalf("含逆流边的协议应可编译: %v", err)
	}
	if len(p.Steps) != 2 {
		t.Fatalf("steps = %d, want 2", len(p.Steps))
	}
	if !p.Steps[0].Reverse {
		t.Errorf("第一步（下丹田→青关）应为逆流边: %+v", p.Steps[0])
	}
	if p.Steps[1].Reverse {
		t.Errorf("第二步（青关→下丹田）应为顺流边: %+v", p.Steps[1])
	}
	if p.Steps[0].Channel != p.Steps[1].Channel {
		t.Errorf("两条边应解析为同一经脉: %d vs %d", p.Steps[0].Channel, p.Steps[1].Channel)
	}
	s.LoadProtocol(p)
	if err := s.StartProtocol(p.Name); err != nil {
		t.Fatalf("start: %v", err)
	}
	sawReverse := false
	for i := 0; i < 2*p.Cycles; i++ { // 周天边界 = 两槽各一息
		res := Tick(s, nil)
		if !res.Advanced {
			t.Fatalf("tick %d 未推进: %+v", i, res.Events)
		}
		for ti := range res.Report.Transfers {
			tr := &res.Report.Transfers[ti]
			if tr.Reverse && tr.Actual[cult.Wood] > 0 {
				sawReverse = true
			}
		}
	}
	if !sawReverse {
		t.Error("试运行未出现逆流传输")
	}
	// TrialMonitor 沿路径记账：6 周天逐条可查，热账字段在（调试视图）。
	if s.Trial == nil || len(s.Trial.Record.Cycles) != 6 {
		t.Fatalf("试跑未按周天记账: %+v", s.Trial)
	}
	for i, c := range s.Trial.Record.Cycles {
		if c.Efficiency < 0 || c.MinStability < 0 {
			t.Errorf("周天 %d 评估量异常: %+v", i, c)
		}
	}
}

// 4. 非法边序列编译拒绝。
func TestCompileRejectsIllegalEdges(t *testing.T) {
	g := &SpikeWorld().Character.BodyGraph

	// 非汇点发起逆流：青关→神庭（天然向 神庭→青关），青关有出向经脉。
	badRev := `
protocol: bad_reverse_v1
actions:
  - guide: {from: 青关, to: 神庭, element: wood, level: small}
`
	_, err := Compile([]byte(badRev), g)
	if err == nil {
		t.Fatal("非汇点逆流边应被拒绝编译")
	}
	if !strings.Contains(err.Error(), "pure sink") {
		t.Errorf("错误信息应说明逆流资格: %v", err)
	}

	// 两向皆无经脉：青关→金门。
	badEdge := `
protocol: no_edge_v1
actions:
  - guide: {from: 青关, to: 金门, element: wood, level: small}
`
	_, err = Compile([]byte(badEdge), g)
	if err == nil {
		t.Fatal("无经脉的边应被拒绝编译")
	}
	if !strings.Contains(err.Error(), "path not connected") {
		t.Errorf("错误信息应说明路径不连通: %v", err)
	}
}

// 5. rule_version 上视图；非 debug 视图无新增真值键。
func TestSessionViewRuleVersion(t *testing.T) {
	reg := NewCultRegistry()
	mux := newTestMux(reg)
	id, v0 := createSession(t, mux, "script")
	if v0.RuleVersion != cult.RuleVersion {
		t.Fatalf("rule_version = %q, want %q", v0.RuleVersion, cult.RuleVersion)
	}
	tick(t, mux, id, 3, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/cult/session/"+id, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode view: %v", err)
	}
	if raw["rule_version"] != cult.RuleVersion {
		t.Fatalf("view rule_version = %v, want %q", raw["rule_version"], cult.RuleVersion)
	}
	// 非 debug 视图仍无真值键；路径评估量（heat/efficiency/stability 数值）不得出现。
	body := rec.Body.String()
	for _, banned := range []string{
		`"metrics"`, `"flow_scale"`, `"debug"`, `"capacity"`, `"throughput"`, `"intake"`,
		`"heat"`, `"thermal_load"`, `"efficiency"`, `"min_stability"`,
	} {
		if strings.Contains(body, banned) {
			t.Fatalf("view leaks truth key %s", banned)
		}
	}
}
