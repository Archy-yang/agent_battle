package ctmatch

// stall_test.go —— M2 WP-M2-3 验收测试（探索僵局对策，B2 模式：探索固执）：
//
//	1. 固执场景：同意图连续多次同质回执 → 转向提示触发（提示文本零数字、
//	   零机器词、含未试方向凸显；账本视图同息凸显未试方向）
//	2. 听劝场景：转向后提示撤下、回执变化（从撞墙到走得通）
//	3. 边界口径：成功节律（生效且有方向回应）不触发；幻觉穴名不入未试方向
//	4. 确定性：同 seed 同脚本（含提示路径）逐位一致
//	5. MC 批测：固执高发场景，开启/关闭检测的固执持续时长对比（可复现）

import (
	"encoding/json"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"agentbattle/backend/cult"
)

// ---------------------------------------------------------------------------
// 场景装配：静环境（环境无气 → 纳气必然"穴未开或满溢不入"）+ 脚本分身
// ---------------------------------------------------------------------------

// stallWorld：无环境之气的测试世界——于青关纳气必然被拒纳（撞墙），
// 而体内引气（丹田⇄青关）不受影响，供"转向后走得通"使用。
func stallWorld() *cult.World {
	w := newTestWorld()
	for e := range w.Env.Qi {
		w.Env.Qi[e] = 0
	}
	return w
}

// runStallScript 按脚本推进会话：每息先唤醒分身再推进，逐息记录
// 回执组与转向提示（确定性：唤醒序即脚本序）。
func runStallScript(t *testing.T, p Pilot, ticks int, w *cult.World) (*CultSession, [][]IntentReceipt, [][]string) {
	t.Helper()
	s := NewSession(w, 97, p)
	s.Observe = func(x *cult.World) *Observation { return cult.Observe(x, 1) }
	receipts := make([][]IntentReceipt, 0, ticks)
	advices := make([][]string, 0, ticks)
	for i := 0; i < ticks; i++ {
		s.WakePilotNextTick()
		res := Tick(s, nil)
		receipts = append(receipts, res.Receipts)
		advices = append(advices, res.Advice)
	}
	return s, receipts, advices
}

// zeroASCII 报告文本中的 ASCII 字母/数字（分身可见载荷零数字零机器词）。
func zeroASCII(s string) string {
	var bad []rune
	for _, r := range s {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			bad = append(bad, r)
		}
	}
	return string(bad)
}

// ---------------------------------------------------------------------------
// 1. 固执场景：转向提示触发
// ---------------------------------------------------------------------------

func TestStallAdviceFiresOnStubbornRepetition(t *testing.T) {
	pilot := &ScriptedPilot{Script: [][]Intent{
		{{Kind: IntentGuide, From: "丹田", To: "青关", Element: cult.Wood, Level: LevelSmall}}, // 先探明一条路（且成）
		{{Kind: IntentAbsorb, Acupoint: "青关", Level: LevelSmall}},                          // 撞墙 ①
		{{Kind: IntentAbsorb, Acupoint: "青关", Level: LevelSmall}},                          // 撞墙 ②
		{{Kind: IntentAbsorb, Acupoint: "青关", Level: LevelSmall}},                          // 撞墙 ③ → 应触发
		{{Kind: IntentAbsorb, Acupoint: "青关", Level: LevelSmall}},                          // 撞墙 ④（提示持续）
	}}
	s, _, advices := runStallScript(t, pilot, 5, stallWorld())

	// 前两息撞墙不满阈值（阈值为连续第 3 条同质回执）。
	if len(advices[1]) != 0 || len(advices[2]) != 0 {
		t.Fatalf("阈值未到不应提示: %v / %v", advices[1], advices[2])
	}
	if len(advices[3]) == 0 {
		t.Fatalf("连续三次同质回执应触发转向提示")
	}
	if len(advices[4]) == 0 {
		t.Fatalf("僵局持续期间提示应保持活跃")
	}
	adv := strings.Join(advices[3], "\n")

	// 零数字、零机器词。
	if bad := receiptDigits(advices[3]); bad != "" {
		t.Errorf("转向提示含数字 %q（分身侧只见定性词）", bad)
	}
	if bad := zeroASCII(adv); bad != "" {
		t.Errorf("转向提示含 ASCII（机器词）%q", bad)
	}
	// 提示文本要素：自述式转向建议 + 未试方向凸显。
	if !strings.Contains(adv, "这条路你已试过几次") || !strings.Contains(adv, "换一个方向") {
		t.Errorf("转向提示缺自述与建议转向: %q", adv)
	}
	if !strings.Contains(adv, "未试方向") {
		t.Errorf("转向提示缺未试方向凸显: %q", adv)
	}
	// 未试方向基于分身自己的尝试记录：已试 丹田→青关 的反向（青关→丹田）
	// 与已知未纳气的 丹田；而刚撞墙的 青关纳气 不算"未试"。
	if !strings.Contains(adv, "引气自青关至丹田") {
		t.Errorf("未试方向缺已试通路的反向: %q", adv)
	}
	if !strings.Contains(adv, "于丹田纳气") {
		t.Errorf("未试方向缺已知未纳气处: %q", adv)
	}
	// 未试方向清单恰为两条：撞墙过的青关纳气不是"未试"（提示首句的
	// 「于青关纳气（少量）」是分身自己交出的意图摘要，属自述回声）。
	if !strings.Contains(adv, "未试方向：引气自青关至丹田；于丹田纳气。") {
		t.Errorf("未试方向清单不符口径: %q", adv)
	}

	// 僵局期间的下一息唤醒：账本视图凸显未试方向，提示注入征询载荷。
	s.WakePilotNextTick()
	Tick(s, nil)
	if len(pilot.LastAdvice) == 0 {
		t.Fatalf("僵局持续期间唤醒应重给提示")
	}
	joined := strings.Join(pilot.LastLedger, "\n")
	if !strings.Contains(joined, "未试方向：引气自青关至丹田") {
		t.Errorf("账本视图应在僵局期间凸显未试方向: %v", pilot.LastLedger)
	}
	if bad := receiptDigits(pilot.LastLedger); bad != "" {
		t.Errorf("账本视图含数字 %q", bad)
	}
}

// ---------------------------------------------------------------------------
// 2. 听劝场景：转向后提示撤下、回执变化
// ---------------------------------------------------------------------------

func TestStallAdviceClearedWhenListening(t *testing.T) {
	pilot := &ScriptedPilot{
		Script: [][]Intent{
			{{Kind: IntentGuide, From: "丹田", To: "青关", Element: cult.Wood, Level: LevelSmall}},
			{{Kind: IntentAbsorb, Acupoint: "青关", Level: LevelSmall}},
			{{Kind: IntentAbsorb, Acupoint: "青关", Level: LevelSmall}},
			{{Kind: IntentAbsorb, Acupoint: "青关", Level: LevelSmall}},
		},
		// 听劝：改走提示里点名的未试方向（青关→丹田，已试通路的反向）。
		AfterAdvice: [][]Intent{
			{{Kind: IntentGuide, From: "青关", To: "丹田", Element: cult.Wood, Level: LevelSmall}},
			{{Kind: IntentSense}},
		},
	}
	s, receipts, advices := runStallScript(t, pilot, 6, stallWorld())

	if len(advices[3]) == 0 {
		t.Fatalf("场景前提：第 4 息应已触发转向提示")
	}
	// 第 5 息：分身听劝转向 → 回执从"不入"变"已生效"。
	var back *IntentReceipt
	for i := range receipts[4] {
		if receipts[4][i].Kind == IntentGuide && strings.Contains(receipts[4][i].Summary, "青关") &&
			strings.Contains(receipts[4][i].Summary, "丹田") {
			back = &receipts[4][i]
		}
	}
	if back == nil {
		t.Fatalf("听劝后应交出引气回执: %+v", receipts[4])
	}
	if back.Outcome != WordApplied {
		t.Fatalf("转向后回执应变化（已生效）: %+v", back)
	}
	if len(advices[4]) != 0 {
		t.Fatalf("转向成功后提示应撤下: %v", advices[4])
	}
	// 第 6 息唤醒：提示已撤（给空）。
	s.WakePilotNextTick()
	Tick(s, nil)
	if pilot.LastAdvice != nil {
		t.Errorf("僵局解除后唤醒应给空提示, got %v", pilot.LastAdvice)
	}
}

// ---------------------------------------------------------------------------
// 3. 边界口径
// ---------------------------------------------------------------------------

// TestStallAdviceNotOnSuccessRhythm：已生效且压力有方向回应的重复是节律，
// 不是固执——不触发提示。
func TestStallAdviceNotOnSuccessRhythm(t *testing.T) {
	pilot := &ScriptedPilot{Script: [][]Intent{
		{{Kind: IntentAbsorb, Acupoint: "青关", Level: LevelSmall}},
		{{Kind: IntentAbsorb, Acupoint: "青关", Level: LevelSmall}},
		{{Kind: IntentAbsorb, Acupoint: "青关", Level: LevelSmall}},
		{{Kind: IntentAbsorb, Acupoint: "青关", Level: LevelSmall}},
	}}
	_, _, advices := runStallScript(t, pilot, 4, newTestWorld())
	for i, a := range advices {
		if len(a) != 0 {
			t.Fatalf("成功节律不应触发提示（息 %d）: %v", i+1, a)
		}
	}
}

// TestStallAdviceFiltersHallucinatedNames：幻觉穴名（回执说"目标穴位
// 不存在"的名字）不回喂、不入未试方向——僵局照旧提示，但账本推不出
// 新路时给定性换法建议，不拿分身念错的名字指路。
func TestStallAdviceFiltersHallucinatedNames(t *testing.T) {
	pilot := &ScriptedPilot{Script: [][]Intent{
		{{Kind: IntentAbsorb, Acupoint: "玄府", Level: LevelSmall}}, // 幻觉穴名 ×3
		{{Kind: IntentAbsorb, Acupoint: "玄府", Level: LevelSmall}},
		{{Kind: IntentAbsorb, Acupoint: "玄府", Level: LevelSmall}},
	}}
	_, _, advices := runStallScript(t, pilot, 3, stallWorld())
	if len(advices[2]) == 0 {
		t.Fatalf("幻觉穴名连撞三次同样应触发转向提示")
	}
	adv := strings.Join(advices[2], "\n")
	if strings.Contains(adv, "未试方向：") {
		t.Errorf("幻觉穴名不得进未试方向: %q", adv)
	}
	if !strings.Contains(adv, "账本里还没有现成的新路") {
		t.Errorf("推不出新路时应给定性换法建议: %q", adv)
	}
}

// ---------------------------------------------------------------------------
// 4. 确定性：同 seed 同脚本（含提示路径）逐位一致
// ---------------------------------------------------------------------------

func TestStallAdviceDeterministic(t *testing.T) {
	script := [][]Intent{
		{{Kind: IntentGuide, From: "丹田", To: "青关", Element: cult.Wood, Level: LevelSmall}},
		{{Kind: IntentAbsorb, Acupoint: "青关", Level: LevelSmall}},
		{{Kind: IntentAbsorb, Acupoint: "青关", Level: LevelSmall}},
		{{Kind: IntentAbsorb, Acupoint: "青关", Level: LevelSmall}},
		{{Kind: IntentGuide, From: "青关", To: "丹田", Element: cult.Wood, Level: LevelSmall}},
		{{Kind: IntentAbsorb, Acupoint: "青关", Level: LevelSmall}},
	}
	do := func() ([][]IntentReceipt, [][]string, []string) {
		pilot := &ScriptedPilot{Script: script}
		s, receipts, advices := runStallScript(t, pilot, len(script), stallWorld())
		return receipts, advices, s.LedgerView()
	}
	r1, a1, l1 := do()
	r2, a2, l2 := do()
	if !reflect.DeepEqual(r1, r2) {
		t.Fatal("回执序列不一致")
	}
	if !reflect.DeepEqual(a1, a2) {
		t.Fatalf("转向提示序列不一致: %v vs %v", a1, a2)
	}
	if !reflect.DeepEqual(l1, l2) {
		t.Fatal("账本视图不一致")
	}
}

// ---------------------------------------------------------------------------
// 5. MC 批测：固执持续时长对比（开启/关闭检测）
// ---------------------------------------------------------------------------

// TestStallAdviceMCStubbornDuration 固执持续时长批测。口径：
//
//   - 200 个 seed（1..200）× 60 息 horizon × 开/关两臂；世界为静环境
//     （纳气必然不入，固执高发）；每息唤醒一次分身。
//   - "固执持续时长" = 首次交出非惯性动作为止的息数；整个 horizon 未换
//     计为 horizon。分身每息恒掷两签（听劝签 + 噪声签，确定性抽签序），
//     开/关两臂同 seed 的抽签序列同构，唯一差异是分身能否收到转向提示
//     （听劝签只在有提示时生效）。
//   - 断言：开启臂总时长显著小于关闭臂（阈值 2/3——允许噪声带来的
//     自然换路，但检测必须带来显著下降）；并对前 20 个 seed 断言同
//     seed 重放逐位一致（确定性）。全整数抽签（Intn）与整数统计，无浮点。
func TestStallAdviceMCStubbornDuration(t *testing.T) {
	const (
		seeds     = 200
		horizon   = 60
		listenPct = 50 // 收到提示后听劝的概率（%）
		noisePct  = 5  // 自发换路噪声（%，与提示无关）
	)

	primary := Intent{Kind: IntentAbsorb, Acupoint: "青关", Level: LevelSmall}
	alternates := []Intent{
		{Kind: IntentGuide, From: "丹田", To: "青关", Element: cult.Wood, Level: LevelSmall},
		{Kind: IntentAbsorb, Acupoint: "丹田", Level: LevelSmall},
	}

	// runStallMC 跑一条批测轨，返回固执持续时长。
	runStallMC := func(seed uint64, disable bool) int {
		w := stallWorld()
		pilot := &adviceListeningPilot{
			rng:        rand.New(rand.NewSource(int64(seed))),
			primary:    primary,
			alternates: alternates,
			listenPct:  listenPct,
			noisePct:   noisePct,
		}
		s := NewSession(w, seed, pilot)
		s.DisableStallAdvice = disable
		s.Observe = func(x *cult.World) *Observation { return cult.Observe(x, 1) }
		for i := 0; i < horizon; i++ {
			s.WakePilotNextTick()
			Tick(s, nil)
			if pilot.switchedAt > 0 {
				return pilot.switchedAt
			}
		}
		return horizon
	}

	type arm struct {
		disable  bool
		duration []int
	}
	arms := []*arm{{disable: true}, {disable: false}}
	for _, a := range arms {
		a.duration = make([]int, 0, seeds)
		for seed := uint64(1); seed <= seeds; seed++ {
			a.duration = append(a.duration, runStallMC(seed, a.disable))
		}
	}
	offSum, onSum := 0, 0
	for _, d := range arms[0].duration {
		offSum += d
	}
	for _, d := range arms[1].duration {
		onSum += d
	}
	// 定点整数报告（千分位均值），无浮点。
	t.Logf("固执持续时长（息，200 seeds×horizon %d）：关闭检测均值 %d.%03d，开启检测均值 %d.%03d",
		horizon, offSum/seeds, (offSum%seeds)*1000/seeds, onSum/seeds, (onSum%seeds)*1000/seeds)
	if onSum*3 >= offSum*2 {
		t.Fatalf("开启检测应显著降低固执持续时长：off=%d on=%d（要求 on<2/3·off）", offSum, onSum)
	}

	// 确定性抽查：前 20 个 seed 的开启臂重放逐位一致。
	for seed := uint64(1); seed <= 20; seed++ {
		if dur := runStallMC(seed, false); dur != arms[1].duration[seed-1] {
			t.Fatalf("seed %d 重放不一致: %d vs %d", seed, dur, arms[1].duration[seed-1])
		}
	}
}

// adviceListeningPilot 是批测分身：以 primary 动作为惯性，经 AdviceAware
// 记录会话转向提示的有无（只记有无，不记内容），收到提示后以 listenPct%
// 概率听劝（改试 alternates 之一），另有 noisePct% 的自发换路噪声（与
// 提示无关）。两枚签每息都掷（不因提示有无而改变抽签序），保证开启/
// 关闭两臂在同 seed 下噪声序列同构——差异只来自检测。
type adviceListeningPilot struct {
	rng             *rand.Rand
	primary         Intent
	alternates      []Intent
	altIdx          int
	calls           int
	listenPct       int
	noisePct        int
	lastAdviceGiven bool
	switchedAt      int // 首次交出非 primary 意图的征询序（1 基；0=未换）
}

func (p *adviceListeningPilot) GiveAdvice(lines []string) {
	p.lastAdviceGiven = len(lines) > 0
}

func (p *adviceListeningPilot) Decide(obs *Observation) []Intent {
	p.calls++
	listenDraw := p.rng.Intn(100)
	noiseDraw := p.rng.Intn(100)
	listen := p.lastAdviceGiven && listenDraw < p.listenPct
	noise := noiseDraw < p.noisePct
	if p.switchedAt == 0 && (listen || noise) {
		p.switchedAt = p.calls
		p.altIdx++
		return []Intent{p.alternates[p.altIdx%len(p.alternates)]}
	}
	return []Intent{p.primary}
}

// ---------------------------------------------------------------------------
// 6. LLMPilot 同构消费（httptest 假端点，不触网——成本纪律）
// ---------------------------------------------------------------------------

// TestLLMPromptCarriesStallGuideline：行事准则含"多次无果考虑换路"条目
// （R3 五段结构内增补；零数字由 TestLLMPromptIsolation 统一锁死）。
func TestLLMPromptCarriesStallGuideline(t *testing.T) {
	var system string
	srv := newFakeLLM(t, `{"cognition":"","intents":[]}`, func(req map[string]any) {
		msgs := req["messages"].([]any)
		system = msgs[0].(map[string]any)["content"].(string)
	})
	p := fakePilot(t, srv)
	p.Decide(fakeObservation())
	if !strings.Contains(system, "会话附言") || !strings.Contains(system, "未试方向") {
		t.Errorf("行事准则缺'多次无果考虑换路'条目")
	}
}

// TestLLMPayloadCarriesAdvice：会话转向提示注入 LLMPilot 征询载荷
// （【会话附言】段，零数字零机器词）；提示撤下后载荷不再携带。
func TestLLMPayloadCarriesAdvice(t *testing.T) {
	var user string
	srv := newFakeLLM(t, `{"cognition":"","intents":[]}`, func(req map[string]any) {
		msgs := req["messages"].([]any)
		user = msgs[1].(map[string]any)["content"].(string)
	})
	p := fakePilot(t, srv)
	p.GiveAdvice([]string{
		"「于青关纳气（少量）」这条路你已试过几次，气息的回应并无不同。可曾想过换一个方向？",
		"未试方向：引气自青关至丹田；于丹田纳气。",
	})
	p.Decide(fakeObservation())
	if p.LastError != "" {
		t.Fatalf("不应报错: %s", p.LastError)
	}
	if !strings.Contains(user, "【会话附言】") || !strings.Contains(user, "未试方向：引气自青关至丹田") {
		t.Errorf("转向提示未注入征询载荷:\n%s", user)
	}
	if seg := user[strings.Index(user, "【会话附言】"):strings.Index(user, "【你的证据账本】")]; zeroASCII(seg) != "" {
		t.Errorf("附言段含数字/机器词 %q", zeroASCII(seg))
	}
	// 提示撤下（给空）后不再携带。
	p.GiveAdvice(nil)
	p.Decide(fakeObservation())
	if strings.Contains(user, "【会话附言】") {
		t.Errorf("提示撤下后载荷不应再携带附言:\n%s", user)
	}
}

// TestLLMSessionStubbornGetsAdvice：会话 + 假端点全链——LLM 分身固执地
// 重复同一无效动作，会话检测到僵局后把转向提示注入下一息征询载荷。
func TestLLMSessionStubbornGetsAdvice(t *testing.T) {
	var user string
	call := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("请求体不是 JSON: %v", err)
		}
		if msgs, ok := body["messages"].([]any); ok && len(msgs) >= 2 {
			user = msgs[1].(map[string]any)["content"].(string)
		}
		call++
		w.Header().Set("Content-Type", "application/json")
		content := `{"cognition":"再试一次纳气","intents":[{"kind":"absorb","target":"青关","level":"small"}]}`
		if call == 1 {
			// 首息先探明一条路：此后"未试方向"才能由账本推出。
			content = `{"cognition":"先引一路气","intents":[{"kind":"guide","from":"丹田","to":"青关","element":"wood","level":"small"}]}`
		}
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": content}}},
		})
	}))
	t.Cleanup(srv.Close)

	w := stallWorld()
	p := newLLMPilotWithConfig(&LLMConfig{BaseURL: srv.URL, APIKey: "test-key", Model: "fake-model"})
	p.MaxCalls = 8
	s := NewSession(w, 97, p)
	s.Observe = func(x *cult.World) *Observation { return cult.Observe(x, 1) }

	// 连续多息重复同一无效动作：第 3 条同质回执后，第 4 息的征询载荷
	// 应带【会话附言】。
	for i := 0; i < 5; i++ {
		s.WakePilotNextTick()
		Tick(s, nil)
	}
	if call < 5 {
		t.Fatalf("分身应被征询 5 次, got %d", call)
	}
	if !strings.Contains(user, "【会话附言】") || !strings.Contains(user, "未试方向") {
		t.Errorf("固执循环应使会话附言进入征询载荷:\n%s", user)
	}
}
