package ctmatch

// ctmatch_test.go —— WP-3 验收测试：
//
//	1. 粗档位 → 参数换算（stage_2 §27）
//	2. 协议编译：合法过 / 非法拒（断路径、超域、未知穴位）
//	3. Runtime 全自动跑 N 周天 + guards（reduce_flow 与 stop）
//	4. 出界升级：validated_range 外请求 → Escalation 且引擎未推进
//	5. 确定性：同 seed 同意图两次会话逐位一致
//	6. Pilot 唤醒 / 前置条件 / 未知异常 / 无效意图

import (
	"reflect"
	"testing"

	"agentbattle/backend/cult"
)

// ---------------------------------------------------------------------------
// 测试世界：丹田(0) ⇄ 青关(1)，加一个无连接的 阳枢(2) 供路径校验拒绝
// ---------------------------------------------------------------------------

func testEnv() cult.Environment {
	return cult.Environment{
		Qi:          [cult.ElementCount]int64{10000, 1000, 1000, 500, 4000},
		Activity:    [cult.ElementCount]int64{1000, 1000, 1000, 1000, 1000},
		Temperature: 1000,
	}
}

func testPoint(name string, capacity, throughput int64) cult.Acupoint {
	return cult.Acupoint{
		Name:       name,
		Affinity:   [cult.ElementCount]int64{1200, 800, 900, 750, 1000},
		Capacity:   capacity,
		Throughput: throughput,
		Retention:  400,
		Openness:   800000,
		Stability:  950,
		Activity:   [cult.ElementCount]int64{1000, 1000, 1000, 1000, 1000},
	}
}

func testCharacter() cult.Character {
	return cult.Character{
		Affinity:   [cult.ElementCount]int64{1250, 800, 900, 750, 1100},
		Control:    700,
		Perception: 500,
		BodyGraph: cult.BodyGraph{
			Acupoints: []cult.Acupoint{
				testPoint("丹田", 20000, 4000),
				testPoint("青关", 10000, 3000),
				testPoint("阳枢", 5000, 2000),
			},
			Channels: []cult.Channel{
				{Name: "任脉下行", From: 0, To: 1, BaseConductance: 900, Throughput: 4000},
				{Name: "任脉上行", From: 1, To: 0, BaseConductance: 900, Throughput: 4000},
			},
		},
	}
}

// newTestWorld 建一个预载灵气的世界：丹田有木气可引导。
func newTestWorld() *cult.World {
	w := cult.NewWorld(testCharacter(), testEnv())
	w.Character.BodyGraph.Acupoints[0].Qi[cult.Wood] = 20000
	return w
}

func newTestSession(t *testing.T, seed uint64) *CultSession {
	t.Helper()
	s := NewSession(newTestWorld(), seed, nil)
	// 先推进一 Tick（静息），使 StartProtocol 有观测基准。
	Tick(s, nil)
	return s
}

const validYAML = `
protocol: dantian_to_qingguan_v1
preconditions:
  - point: 青关
    metric: stability
    op: ">="
    value: 900
actions:
  - absorb:
      target: 青关
      level: trace
      duration: 2
  - guide:
      from: 丹田
      to: 青关
      element: wood
      level: medium
      duration: 1
cadence: 1
cycles: 3
guards:
  - when:
      point: 青关
      metric: pressure
      op: ">="
      value: 1200
    do: reduce_flow
validated_range:
  - channel: [丹田, 青关]
    element: wood
    min: 0
    max: 2000
`

// ---------------------------------------------------------------------------
// 1. 档位换算
// ---------------------------------------------------------------------------

func TestLevelConversion(t *testing.T) {
	// 强度：档位 → 千分位比率（stage_2 §27）。
	cases := map[Level]int64{
		LevelTrace: 50, LevelSmall: 250, LevelMedium: 500, LevelLarge: 800,
	}
	for lv, want := range cases {
		if got := IntensityForLevel(lv); got != want {
			t.Errorf("IntensityForLevel(%d) = %d, want %d", lv, got, want)
		}
	}

	// 流量：档位 × 通道有效通量。青关吞吐 3000、无损伤 → 有效通量 3000。
	g := &newTestWorld().Character.BodyGraph
	for _, tc := range []struct {
		lv   Level
		want int64
	}{{LevelTrace, 150}, {LevelSmall, 750}, {LevelMedium, 1500}, {LevelLarge, 2400}} {
		if got := FlowForLevel(tc.lv, g, 0, 1, 0); got != tc.want {
			t.Errorf("FlowForLevel(%d) = %d, want %d", tc.lv, got, tc.want)
		}
	}

	// 同档位依身体状态换算：青关损伤 0.3 → 有效吞吐 2100 → large = 1680。
	g2 := &newTestWorld().Character.BodyGraph
	g2.Acupoints[1].Damage = 300
	if got := FlowForLevel(LevelLarge, g2, 0, 1, 0); got != 1680 {
		t.Errorf("FlowForLevel(large, damaged) = %d, want 1680", got)
	}
}

func TestGuideMixedDistribution(t *testing.T) {
	s := newTestSession(t, 42)
	res := Tick(s, []Intent{{
		Kind: IntentGuide, From: "丹田", To: "青关",
		Element: -1, Level: LevelMedium, // 1500 → 每元素 300
	}})
	if !res.Advanced || res.Report == nil {
		t.Fatalf("tick did not advance")
	}
	req := res.Report.Transfers[0].Requested
	for e := 0; e < cult.ElementCount; e++ {
		if req[e] != 300 {
			t.Errorf("mixed element %d requested %d, want 300", e, req[e])
		}
	}
}

// ---------------------------------------------------------------------------
// 2. 协议编译
// ---------------------------------------------------------------------------

func TestCompileValid(t *testing.T) {
	g := &newTestWorld().Character.BodyGraph
	p, err := Compile([]byte(validYAML), g)
	if err != nil {
		t.Fatalf("valid protocol rejected: %v", err)
	}
	if p.Name != "dantian_to_qingguan_v1" {
		t.Errorf("name = %q", p.Name)
	}
	if len(p.Steps) != 2 || p.Cycles != 3 || p.Cadence != 1 {
		t.Fatalf("steps=%d cycles=%d cadence=%d", len(p.Steps), p.Cycles, p.Cadence)
	}
	if p.Steps[1].Channel != 0 || p.Steps[1].From != 0 || p.Steps[1].To != 1 {
		t.Errorf("guide step not resolved to channel 0 (0->1): %+v", p.Steps[1])
	}
	if p.Steps[0].PointIdx != 1 || p.Steps[0].Level != LevelTrace {
		t.Errorf("absorb step wrong: %+v", p.Steps[0])
	}
	if len(p.Guards) != 1 || p.Guards[0].When.PointIdx != 1 {
		t.Errorf("guards not resolved: %+v", p.Guards)
	}
	if len(p.Ranges) != 1 || p.Ranges[0].Max != 2000 {
		t.Errorf("ranges wrong: %+v", p.Ranges)
	}
}

func TestCompileRejects(t *testing.T) {
	g := &newTestWorld().Character.BodyGraph
	cases := []struct {
		name string
		yaml string
		want string // 错误信息必须包含的片段
	}{
		{"断路径", `
protocol: broken
actions:
  - guide: {from: 丹田, to: 阳枢, element: wood, level: small}
`, "path not connected"},
		{"未知穴位", `
protocol: ghost
actions:
  - absorb: {target: 太乙, level: small}
`, `unknown acupoint "太乙"`},
		{"未知档位", `
protocol: badlevel
actions:
  - absorb: {target: 青关, level: huge}
`, "unknown level"},
		{"档位与参数互斥", `
protocol: dup
actions:
  - absorb: {target: 青关, level: small, intensity: 600}
`, "mutually exclusive"},
		{"超域强度", `
protocol: over
actions:
  - absorb: {target: 青关, intensity: 1500}
`, "intensity in (0,1]"},
		{"无参数引导", `
protocol: empty
actions:
  - guide: {from: 丹田, to: 青关, element: wood}
`, "need level or positive amount"},
		{"未知 guard 指标", `
protocol: badguard
actions:
  - absorb: {target: 青关, level: trace}
guards:
  - when: {point: 青关, metric: luck, op: ">=", value: 100}
    do: stop
`, "unknown metric"},
		{"未知 guard 动作", `
protocol: badaction
actions:
  - absorb: {target: 青关, level: trace}
guards:
  - when: {point: 青关, metric: pressure, op: ">=", value: 100}
    do: explode
`, "unknown action"},
		{"阈值超域", `
protocol: badvalue
actions:
  - absorb: {target: 青关, level: trace}
guards:
  - when: {point: 青关, metric: pressure, op: ">=", value: -5}
    do: stop
`, "out of domain"},
		{"无动作", `
protocol: noactions
actions: []
`, "at least one step"},
		{"区间倒挂", `
protocol: badrange
actions:
  - absorb: {target: 青关, level: trace}
validated_range:
  - channel: [丹田, 青关]
    min: 2000
    max: 1000
`, "0 <= min <= max"},
	}
	for _, tc := range cases {
		_, err := Compile([]byte(tc.yaml), g)
		if err == nil {
			t.Errorf("%s: expected rejection, got nil", tc.name)
			continue
		}
		if !contains(err.Error(), tc.want) {
			t.Errorf("%s: error %q does not mention %q", tc.name, err, tc.want)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

// ---------------------------------------------------------------------------
// 3. Runtime 全自动跑 N 周天
// ---------------------------------------------------------------------------

func TestProtocolAutoRun(t *testing.T) {
	s := newTestSession(t, 7)
	p, err := Compile([]byte(validYAML), &s.World.Character.BodyGraph)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	s.LoadProtocol(p)
	if err := s.StartProtocol(p.Name); err != nil {
		t.Fatalf("start: %v", err)
	}

	// 步骤：absorb(2 ticks) + guide(1 tick)，3 周天 → 9 Tick。
	for i := 0; i < 9; i++ {
		res := Tick(s, nil)
		if !res.Advanced {
			t.Fatalf("tick %d: engine not advanced", i)
		}
	}
	if got := s.World.Tick; got != 10 { // 1 静息 + 9
		t.Fatalf("world tick = %d, want 10", got)
	}
	if s.ActiveProtocol() != "" {
		t.Errorf("protocol still active after completion")
	}
	if e := lastEventOf(s, EventProtocolFinished); e == nil {
		t.Errorf("no protocol_finished event; events=%+v", s.Events)
	} else if e.Detail["cycles"] != 3 {
		t.Errorf("finished detail cycles = %d", e.Detail["cycles"])
	}
}

// lastEventOf 返回最后一个指定种类的事件；无则 nil。
func lastEventOf(s *CultSession, k EventKind) *Event {
	for i := len(s.Events) - 1; i >= 0; i-- {
		if s.Events[i].Kind == k {
			return &s.Events[i]
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// 4. Guards：reduce_flow 与 stop
// ---------------------------------------------------------------------------

const guardReduceYAML = `
protocol: guarded_guide_v1
actions:
  - guide:
      from: 丹田
      to: 青关
      element: wood
      level: medium
      duration: 1
cadence: 1
cycles: 20
guards:
  - when:
      point: 青关
      metric: pressure
      op: ">="
      value: 800
    do: reduce_flow
validated_range:
  - channel: [丹田, 青关]
    element: wood
    min: 0
    max: 3000
`

func TestGuardReduceFlow(t *testing.T) {
	s := newTestSession(t, 11)
	// 预载青关负载 0.9：本 Tick 引导后压力越过 0.8 阈值。
	s.World.Character.BodyGraph.Acupoints[1].Qi[cult.Wood] = 9000
	p, err := Compile([]byte(guardReduceYAML), &s.World.Character.BodyGraph)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	s.LoadProtocol(p)
	if err := s.StartProtocol(p.Name); err != nil {
		t.Fatalf("start: %v", err)
	}

	res1 := Tick(s, nil)
	if res1.Escalated {
		t.Errorf("reduce_flow should not escalate to pilot")
	}
	if lastEventOf(s, EventGuardAdjusted) == nil {
		t.Fatalf("no guard_adjusted event; events=%+v", s.Events)
	}
	if s.FlowScale() >= One {
		t.Errorf("flowScale = %d, want < 1000 after reduce_flow", s.FlowScale())
	}

	scale1 := s.FlowScale() // tick1 后由 guard 决定的缩系数
	res2 := Tick(s, nil)
	// 第二 Tick 的请求流量应已缩减（1500 × scale/1000）。
	got := res2.Report.Transfers[0].Requested[cult.Wood]
	want := fpMul(1500, scale1)
	if got != want {
		t.Errorf("tick2 requested = %d, want %d (scaled)", got, want)
	}
	if s.ActiveProtocol() == "" {
		t.Errorf("protocol should still be running after reduce_flow")
	}
}

func TestGuardStop(t *testing.T) {
	s := newTestSession(t, 13)
	s.World.Character.BodyGraph.Acupoints[1].Qi[cult.Wood] = 9500
	yaml := `
protocol: stop_guard_v1
actions:
  - guide:
      from: 丹田
      to: 青关
      element: wood
      level: medium
cadence: 1
cycles: 10
guards:
  - when:
      point: 青关
      metric: pressure
      op: ">="
      value: 900
    do: stop
validated_range:
  - channel: [丹田, 青关]
    element: wood
    min: 0
    max: 3000
`
	p, err := Compile([]byte(yaml), &s.World.Character.BodyGraph)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	s.LoadProtocol(p)
	if err := s.StartProtocol(p.Name); err != nil {
		t.Fatalf("start: %v", err)
	}

	res := Tick(s, nil)
	if !res.Escalated {
		t.Errorf("guard stop should escalate")
	}
	e := lastEventOf(s, EventGuardAdjusted)
	if e == nil {
		t.Fatalf("no guard_adjusted event")
	}
	if !contains(e.Reason, "stop") {
		t.Errorf("event reason = %q", e.Reason)
	}

	// 下一 Tick：Protocol 已暂停，引擎以零 Ops 推进（无吸收、无引导）。
	res2 := Tick(s, nil)
	if !res2.Advanced {
		t.Fatalf("tick2 not advanced")
	}
	if res2.Report.Points[1].Intake != [cult.ElementCount]int64{} {
		t.Errorf("paused protocol still absorbing: %+v", res2.Report.Points[1].Intake)
	}
	if s.ActiveProtocol() == "" {
		t.Errorf("paused protocol should remain active (resumable)")
	}
}

// ---------------------------------------------------------------------------
// 5. 出界升级（Knowledge Boundary，stage_3 §41）
// ---------------------------------------------------------------------------

func TestOutOfBoundary(t *testing.T) {
	s := newTestSession(t, 17)
	yaml := `
protocol: boundary_v1
actions:
  - guide:
      from: 丹田
      to: 青关
      element: wood
      level: large     # 0.8 × 3000 = 2400 > max 2000
cadence: 1
cycles: 5
validated_range:
  - channel: [丹田, 青关]
    element: wood
    min: 0
    max: 2000
`
	p, err := Compile([]byte(yaml), &s.World.Character.BodyGraph)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	s.LoadProtocol(p)
	if err := s.StartProtocol(p.Name); err != nil {
		t.Fatalf("start: %v", err)
	}

	tickBefore := s.World.Tick
	res := Tick(s, nil)
	if res.Advanced || res.Report != nil {
		t.Fatalf("engine must NOT advance on out-of-boundary request")
	}
	if s.World.Tick != tickBefore {
		t.Errorf("world tick moved: %d -> %d", tickBefore, s.World.Tick)
	}
	if !res.Escalated {
		t.Errorf("out-of-boundary must escalate")
	}
	e := lastEventOf(s, EventOutOfBoundary)
	if e == nil {
		t.Fatalf("no out_of_boundary event")
	}
	if !e.IsEscalation() {
		t.Errorf("out_of_boundary not marked as escalation")
	}
	if s.ActiveProtocol() == "" {
		t.Errorf("protocol should be paused, not discarded")
	}
}

func TestUnvalidatedChannelRejectsAnyFlow(t *testing.T) {
	s := newTestSession(t, 19)
	yaml := `
protocol: no_range_v1
actions:
  - guide:
      from: 丹田
      to: 青关
      element: wood
      level: trace
cadence: 1
cycles: 2
`
	p, err := Compile([]byte(yaml), &s.World.Character.BodyGraph)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	s.LoadProtocol(p)
	if err := s.StartProtocol(p.Name); err != nil {
		t.Fatalf("start: %v", err)
	}
	res := Tick(s, nil)
	if res.Advanced || !res.Escalated || lastEventOf(s, EventOutOfBoundary) == nil {
		t.Fatalf("unvalidated channel must escalate before advancing; res=%+v", res)
	}
}

// ---------------------------------------------------------------------------
// 6. Pilot 唤醒 / 前置条件 / 未知异常 / 无效意图
// ---------------------------------------------------------------------------

func TestEscalationWakesPilot(t *testing.T) {
	w := newTestWorld()
	pilot := &ScriptedPilot{Script: [][]Intent{{{Kind: IntentStop}}}}
	s := NewSession(w, 23, pilot)
	Tick(s, nil)

	yaml := `
protocol: wake_v1
actions:
  - guide:
      from: 丹田
      to: 青关
      element: wood
      level: large
cadence: 1
cycles: 2
validated_range:
  - channel: [丹田, 青关]
    element: wood
    min: 0
    max: 2000
`
	p, err := Compile([]byte(yaml), &w.Character.BodyGraph)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	s.LoadProtocol(p)
	if err := s.StartProtocol(p.Name); err != nil {
		t.Fatalf("start: %v", err)
	}

	if res := Tick(s, nil); !res.Escalated {
		t.Fatalf("expected boundary escalation")
	}
	res := Tick(s, nil) // 升级挂起 → 唤醒 Pilot
	if pilot.Calls != 1 {
		t.Fatalf("pilot calls = %d, want 1", pilot.Calls)
	}
	if !res.Advanced {
		t.Fatalf("pilot stop should let engine advance with zero ops")
	}
	if res.Report.Points[1].Intake != [cult.ElementCount]int64{} {
		t.Errorf("pilot stop not executed: intake %+v", res.Report.Points[1].Intake)
	}
	found := false
	for _, le := range s.IntentLog {
		if le.FromPilot && le.Intent.Kind == IntentStop {
			found = true
		}
	}
	if !found {
		t.Errorf("pilot intent not logged")
	}
}

func TestPreconditions(t *testing.T) {
	s := newTestSession(t, 29)
	yaml := `
protocol: precond_v1
preconditions:
  - point: 青关
    metric: stability
    op: ">="
    value: 990
actions:
  - absorb: {target: 青关, level: trace}
cadence: 1
cycles: 1
`
	p, err := Compile([]byte(yaml), &s.World.Character.BodyGraph)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	s.LoadProtocol(p)
	if err := s.StartProtocol(p.Name); err == nil {
		t.Fatalf("start should fail on unmet precondition (stability 950 < 990)")
	}
	if lastEventOf(s, EventPreconditionFail) == nil {
		t.Errorf("no precondition_failed event")
	}
	if s.ActiveProtocol() != "" {
		t.Errorf("protocol must not start on unmet precondition")
	}
}

func TestUnknownAnomaly(t *testing.T) {
	s := newTestSession(t, 31)
	// 预载青关满载并超压：负载比 2.0 → 应变大幅上升 → 稳定性跌破 0.4 →
	// 出现新损伤，而协议没有任何 guard 覆盖 → 未知异常升级。
	s.World.Character.BodyGraph.Acupoints[1].Qi[cult.Wood] = 20000
	yaml := `
protocol: anomaly_v1
actions:
  - absorb: {target: 青关, level: trace}
cadence: 1
cycles: 4
`
	p, err := Compile([]byte(yaml), &s.World.Character.BodyGraph)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	s.LoadProtocol(p)
	if err := s.StartProtocol(p.Name); err != nil {
		t.Fatalf("start: %v", err)
	}

	res := Tick(s, nil)
	if !res.Escalated {
		t.Fatalf("expected unknown_anomaly escalation")
	}
	e := lastEventOf(s, EventUnknownAnomaly)
	if e == nil {
		t.Fatalf("no unknown_anomaly event; events=%+v", s.Events)
	}
	if res.Report.Points[1].Damage == 0 {
		t.Errorf("test setup: expected new damage, none observed")
	}
}

func TestInvalidIntentDropped(t *testing.T) {
	s := newTestSession(t, 37)
	res := Tick(s, []Intent{{Kind: IntentAbsorb, Acupoint: "不存在", Level: LevelSmall}})
	if !res.Advanced {
		t.Fatalf("tick should still advance")
	}
	if lastEventOf(s, EventInvalidIntent) == nil {
		t.Errorf("no invalid_intent event")
	}
	for _, v := range res.Report.Points[1].Intake {
		if v != 0 {
			t.Errorf("invalid intent must not affect world")
		}
	}
}

// ---------------------------------------------------------------------------
// 7. 确定性：同 seed 同意图流 → 逐位一致
// ---------------------------------------------------------------------------

func TestDeterminism(t *testing.T) {
	script := [][]Intent{
		{{Kind: IntentSense}},
		{{Kind: IntentAbsorb, Acupoint: "青关", Level: LevelSmall}},
		{{Kind: IntentGuide, From: "丹田", To: "青关", Element: cult.Wood, Level: LevelMedium}},
		{{Kind: IntentStop}},
		{{Kind: IntentGuide, From: "丹田", To: "青关", Element: -1, Level: LevelLarge, Amount: 1500}},
		nil, nil, nil, nil, nil,
	}

	run := func() (*cult.World, []*cult.TickReport, []IntentLogEntry) {
		s := NewSession(newTestWorld(), 1234, nil)
		var reps []*cult.TickReport
		for i, intents := range script {
			_ = i
			res := Tick(s, intents)
			reps = append(reps, res.Report)
		}
		return s.World, reps, s.IntentLog
	}

	w1, r1, l1 := run()
	w2, r2, l2 := run()

	if !reflect.DeepEqual(w1, w2) {
		t.Fatalf("worlds diverge (incl. unexported state)")
	}
	b1, err1 := MarshalReports(r1)
	b2, err2 := MarshalReports(r2)
	if err1 != nil || err2 != nil {
		t.Fatalf("marshal: %v %v", err1, err2)
	}
	if string(b1) != string(b2) {
		t.Fatalf("tick reports diverge byte-wise")
	}
	if !reflect.DeepEqual(l1, l2) {
		t.Fatalf("intent logs diverge")
	}

	// 意图日志应覆盖全部提交（4 条非空 + 1 条 pilot 之外的意图计数）。
	n := 0
	for _, group := range script {
		n += len(group)
	}
	if len(l1) != n {
		t.Errorf("intent log entries = %d, want %d", len(l1), n)
	}
}

// ---------------------------------------------------------------------------
// 8. 显式流量（L2 探索不受 validated_range 限制——Runtime 才受限）
// ---------------------------------------------------------------------------

func TestExplicitAmountIntentExplores(t *testing.T) {
	s := newTestSession(t, 41)
	// L2 探索未知区间：直提 5000 流量（远超任何已验证区）——
	// 意图路径不做边界拦截，Engine 决定实际结果。
	res := Tick(s, []Intent{{
		Kind: IntentGuide, From: "丹田", To: "青关",
		Element: cult.Wood, Level: LevelUnset, Amount: 5000,
	}})
	if !res.Advanced {
		t.Fatalf("tick should advance")
	}
	if got := res.Report.Transfers[0].Requested[cult.Wood]; got == 0 {
		t.Errorf("explicit amount intent produced no request")
	}
}
