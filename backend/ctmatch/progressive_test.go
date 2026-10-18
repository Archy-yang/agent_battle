package ctmatch

// progressive_test.go —— 渐进感知修订包（specs/2026-10-18-progressive-
// perception-v1.md）在会话/API 层的验收测试：
//
//   - 凝神细察（inspect）回执：无感之穴 →「无从察起」；方位语与真名
//     两套称谓都可落地（§4）；
//   - 零泄名机检（§4）：档位低于显名的穴，其真名不得出现在任何观察/
//     感知流文本中（含事件行与 LLM 分身观察口径——方位语一体适用）；
//   - 渐进显形不阻碍既有闭环：感知流先以方位语行文、显名后以真名行文。

import (
	"strings"
	"testing"

	"agentbattle/backend/cult"
)

// receiptAt 报告某息的回执里是否出现指定补语/措辞。
func receiptAt(v *SessionView, tick int64, text string) bool {
	for _, r := range v.Receipts {
		if r.Tick == tick && strings.Contains(r.Text, text) {
			return true
		}
	}
	return false
}

// TestInspectReceipt_NothingToSense 凝神细察：无感之穴 → 回执
// 「无从察起」；方位语（§4 表）与真名同样可落地；熟识度积累后（有感）
// 再凝神不再是「无从察起」。
func TestInspectReceipt_NothingToSense(t *testing.T) {
	reg := NewCultRegistry()
	mux := newTestMux(reg)
	id, _ := createSession(t, mux, "script", "")

	// 金门静置无感：真名凝神 → 无从察起。
	v := tick(t, mux, id, 1, []map[string]any{{"kind": "inspect", "target": "金门"}})
	if !receiptAt(v, v.Tick, "无从察起") {
		t.Fatalf("无感之穴凝神应回执无从察起: %+v", v.Receipts)
	}
	// 方位语「额上」（神庭）同样可落地 → 无从察起。
	v = tick(t, mux, id, 1, []map[string]any{{"kind": "inspect", "target": "额上"}})
	if !receiptAt(v, v.Tick, "无从察起") {
		t.Fatalf("方位语凝神应可落地并回执无从察起: %+v", v.Receipts)
	}
	// 金门再凝神：熟识度已积（有感），回执不再是「无从察起」。
	v = tick(t, mux, id, 1, []map[string]any{{"kind": "inspect", "target": "金门"}})
	if receiptAt(v, v.Tick, "无从察起") {
		t.Fatalf("有感之穴凝神不应回执无从察起: %+v", v.Receipts)
	}
	if !receiptAt(v, v.Tick, "已生效") {
		t.Fatalf("凝神回执缺失: %+v", v.Receipts)
	}
}

// TestNoNameLeak_SessionStream ④ 零泄名机检（会话口径）：档位低于
// 显名的穴，其真名不得出现在任何观察/感知流文本中——sense 行、事件
// 行、观察序列化全文与 LLM 分身的观察载荷（方位语一体适用）。
// 回执/玩家志里的名字是行动者自己交出的称谓（闭环词汇，不属世界主动
// 叙述），不在本机检口径内。
func TestNoNameLeak_SessionStream(t *testing.T) {
	reg := NewCultRegistry()
	mux := newTestMux(reg)
	id, _ := createSession(t, mux, "script", "tutorial")

	// 行功 40 息：青关（主纳穴）逐息活跃 → 早已显名；其余诸穴静置。
	v := tick(t, mux, id, 40, nil)
	h := reg.get(id)
	obs := h.sess.Observe(h.sess.World)
	if obs == nil {
		t.Fatal("观察钩子缺失")
	}
	js := string(mustJSON(t, obs))

	var stream, events string
	for _, l := range v.SenseStream {
		switch l.Kind {
		case "sense":
			stream += l.Text + "\n"
		case "event":
			events += l.Text + "\n"
		}
	}

	named := 0
	for i, p := range obs.Points {
		name := h.sess.World.Character.BodyGraph.Acupoints[i].Name
		if p.Tier == cult.FameTierWord(cult.FameNamed) {
			named++
			if !strings.Contains(stream, name) {
				t.Fatalf("显名之穴 %s 应以真名入感知流", name)
			}
			continue
		}
		for where, text := range map[string]string{"感知流": stream, "事件流": events, "观察序列化": js} {
			if strings.Contains(text, name) {
				t.Fatalf("零泄名机检违例：未显名之穴 %s 出现在%s", name, where)
			}
		}
	}
	if named == 0 {
		t.Fatal("测试前提失效：行功 40 息主纳穴应已显名")
	}

	// 渐进显形不阻碍行文：显名前的感知流以方位语「左胁」指称主纳穴。
	if !strings.Contains(stream, "左胁") {
		t.Fatal("感知流未见方位语行文（未显名动静应以方位语指称）")
	}

	// LLM 分身观察口径：用户载荷同样不得含未显名真名（方位语一体适用）。
	lp := &LLMPilot{}
	msg := lp.buildUserMessage(obs)
	for i, p := range obs.Points {
		if p.Tier == cult.FameTierWord(cult.FameNamed) {
			continue
		}
		name := h.sess.World.Character.BodyGraph.Acupoints[i].Name
		if strings.Contains(msg, name) {
			t.Fatalf("零泄名机检违例：未显名之穴 %s 出现在分身观察载荷", name)
		}
	}
}

// TestChannelTierInSessionView 经脉档位进 ChannelView；端点未显名者
// 称谓置空（经脉名内嵌端点穴名，零泄名口径）。
func TestChannelTierInSessionView(t *testing.T) {
	reg := NewCultRegistry()
	mux := newTestMux(reg)
	id, _ := createSession(t, mux, "script", "tutorial")
	v := tick(t, mux, id, 40, nil)

	foundTier := false
	for _, c := range v.Body.Channels {
		switch c.Tier {
		case "无迹", "初显", "熟路":
			foundTier = true
		default:
			t.Fatalf("经脉档位词越表: %q", c.Tier)
		}
		if c.Tier == "无迹" && (c.Name != "" || c.From != "" || c.To != "") {
			// 无迹经脉可以仍带称谓（拓扑可见）；但称谓里不得内嵌未显名
			// 端点真名——下述逐端点检查统一把关。
			continue
		}
	}
	if !foundTier {
		t.Fatal("ChannelView 缺档位字段")
	}
	// 逐端点：称谓里的端点真名只许出现在该端点显名的经脉上。
	h := reg.get(id)
	obs := h.sess.Observe(h.sess.World)
	g := &h.sess.World.Character.BodyGraph
	isNamed := map[string]bool{}
	for i, p := range obs.Points {
		if p.Tier == cult.FameTierWord(cult.FameNamed) {
			isNamed[g.Acupoints[i].Name] = true
		}
	}
	for i, c := range v.Body.Channels {
		if i >= len(g.Channels) {
			break
		}
		gc := g.Channels[i]
		for _, end := range []struct {
			label string
			idx   int
		}{{c.From, gc.From}, {c.To, gc.To}} {
			if end.label != "" && !isNamed[end.label] {
				t.Fatalf("经脉 %d 称谓内嵌未显名端点 %q", i, end.label)
			}
		}
	}
}
