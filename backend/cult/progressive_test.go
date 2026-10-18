package cult

// progressive_test.go —— 渐进感知修订包（specs/2026-10-18-progressive-
// perception-v1.md）验收测试：
//
//	①熟识度单调与同 seed 重放一致（DeepEqual）；
//	②显名节奏预算：每息至多一穴新显名、并列取先、气机最活跃者优先；
//	③凝神细察加速（+3）与档位跃迁；
//	④零泄名机检：档位低于显名的穴，其名不得出现在 Observation 序列化
//	  全文的任何字段（含 Channels 的 Name——经脉名内嵌端点穴名）；
//	⑤经脉档位随气行史推进（无迹→初显→熟路）。

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// famePoint 构造一个预载高负载的穴位：压感信号入档 → 每息「气机活跃」，
// 熟识度随观察逐息积累（静息世界也能走完显名阶梯的确定性工装）。
func famePoint(name string) Acupoint {
	a := Acupoint{
		Name:       name,
		Affinity:   [ElementCount]int64{1000, 1000, 1000, 1000, 1000},
		Capacity:   100000,
		Throughput: 8000,
		Retention:  800,
		Openness:   800000,
		Stability:  950,
		// 普通穴驱动画像（SpikeWorld 同款）：行气测试需要源穴推得动。
		DriveCapacity:   8000,
		DriveEfficiency: 850,
	}
	a.Qi[Wood] = 50000 // LR 0.5 → 压力 0.25 → 胀感入档（气机活跃）
	return a
}

// coldPoint 构造一个空载穴位：无压感、无报告动静 → 气机不活跃（无感）。
func coldPoint(name string) Acupoint {
	return Acupoint{
		Name:            name,
		Affinity:        [ElementCount]int64{1000, 1000, 1000, 1000, 1000},
		Capacity:        100000,
		Throughput:      8000,
		Retention:       800,
		Openness:        800000,
		Stability:       950,
		DriveCapacity:   8000,
		DriveEfficiency: 850,
	}
}

// fameEnv 环境灵气近均衡（供意识纳入有势可乘）。
func fameEnv() Environment {
	return Environment{
		Qi:          [ElementCount]int64{3000, 3000, 3000, 3000, 3000},
		Activity:    [ElementCount]int64{1000, 1000, 1000, 1000, 1000},
		Temperature: 1000,
	}
}

func fameWorld(pts ...Acupoint) *World {
	return NewWorld(Character{
		Affinity:   [ElementCount]int64{1000, 1000, 1000, 1000, 1000},
		Control:    1000,
		Perception: 1000,
		BodyGraph:  BodyGraph{Acupoints: pts},
	}, fameEnv())
}

// TestFame_TierLadder_Monotonic 阶梯（§2）：压感常驻的穴位随观察逐息
// 积累——隐约(≥1) → 有感(≥3) → 显名(≥6 且得预算)；未至显名者 Name 与
// 浮卡内容（开度/稳固/损伤/nature）置空。熟识度只增不减（单调）。
func TestFame_TierLadder_Monotonic(t *testing.T) {
	// 冷穴（独立世界，免被暖穴的阶梯计息干扰）：无气机、无动静 →
	// 无感，不落名，方位语在场。
	cw := fameWorld(coldPoint("神庭"))
	cold := Observe(cw, 2).Points[0]
	if cold.Tier != FameTierWord(FameNone) || cold.Name != "" || cold.Region != "额上" {
		t.Fatalf("冷穴档位 = %s/%q/%q, want 无感/\"/额上", cold.Tier, cold.Name, cold.Region)
	}

	w := fameWorld(famePoint("青关"), coldPoint("神庭"))

	// 暖穴阶梯：第 1 息隐约、第 3 息有感、第 6 息越阈并得预算 → 显名。
	want := []string{FameTierWord(FameVague), FameTierWord(FameVague),
		FameTierWord(FameFelt), FameTierWord(FameFelt), FameTierWord(FameFelt),
		FameTierWord(FameNamed), FameTierWord(FameNamed), FameTierWord(FameNamed)}
	prev := -1
	for i, wt := range want {
		w.Ops = Ops{Intensity: []int64{600, 0}}
		Advance(w)
		p := Observe(w, 2).Points[0]
		if p.Tier != wt {
			t.Fatalf("第 %d 息 Tier = %s, want %s", i+1, p.Tier, wt)
		}
		if fameTierOf(w.sensory.famPts[0]) < prev {
			t.Fatalf("第 %d 息熟识度回退——单调律被破坏", i+1)
		}
		prev = fameTierOf(w.sensory.famPts[0])
		if p.Tier != FameTierWord(FameNamed) {
			if p.Name != "" || p.Openness != "" || p.Stability != "" || p.Damage != "" || len(p.Nature) != 0 {
				t.Fatalf("第 %d 息未显名却给出浮卡内容: %+v", i+1, p)
			}
		} else if p.Name != "青关" {
			t.Fatalf("第 %d 息显名而未得真名: %q", i+1, p.Name)
		}
	}
}

// TestFame_ReplayConsistency ①：同 seed 同 Ops + 相同观察/凝神序列 →
// 熟识度状态存档逐位一致（DeepEqual）；观察序列也深度相等；档位单调。
func TestFame_ReplayConsistency(t *testing.T) {
	run := func() (*World, []*Observation) {
		w := fameWorld(famePoint("青关"), famePoint("下丹田"))
		var obs []*Observation
		for i := 0; i < 30; i++ {
			if i%4 == 0 {
				w.Ops = Ops{Intensity: []int64{0, 600}}
			} else {
				w.Ops = Ops{Intensity: []int64{0, 0}}
			}
			Advance(w)
			obs = append(obs, Observe(w, 2))
			if i%7 == 3 {
				InspectFame(w, 0) // 凝神序列也是重放的一部分
			}
		}
		return w, obs
	}
	w1, o1 := run()
	w2, o2 := run()
	if !reflect.DeepEqual(w1.sensory, w2.sensory) {
		t.Fatal("熟识度状态存档在重放下不一致——确定性被破坏")
	}
	for i := range o1 {
		if !reflect.DeepEqual(o1[i], o2[i]) {
			t.Fatalf("第 %d 息重放观察不一致", i)
		}
	}
	// 单调：档位（序）逐息不降。
	prev := 0
	for range o1 {
		cur := fameTierOf(w1.sensory.famPts[0])
		if cur < prev {
			t.Fatal("重放序列中档位回退——单调律被破坏")
		}
		prev = cur
	}
}

// TestFame_NamingBudget ②：三穴同息过阈——每息至多一穴新显名（§3，
// 满屏齐出在引擎层即不可能），并列取下标先。
func TestFame_NamingBudget(t *testing.T) {
	w := fameWorld(famePoint("青关"), famePoint("下丹田"), famePoint("神庭"))
	namedTier := FameTierWord(FameNamed)

	// 五息预热：三穴熟识度同抵 5（有感），无一显名。
	for i := 0; i < 5; i++ {
		Advance(w)
		obs := Observe(w, 2)
		for _, p := range obs.Points {
			if p.Tier == namedTier {
				t.Fatalf("预热第 %d 息即有穴显名——阈值失守", i+1)
			}
		}
	}

	// 第 6 息三穴同越「显名」阈：预算只发一枚，并列取下标先。
	Advance(w)
	obs := Observe(w, 2)
	if obs.Points[0].Tier != namedTier {
		t.Fatalf("并列取先失效：首穴应得本息唯一显名预算, got %s", obs.Points[0].Tier)
	}
	for i := 1; i < len(obs.Points); i++ {
		if obs.Points[i].Tier == namedTier {
			t.Fatalf("满屏齐出：同息第 %d 穴不得显名", i)
		}
	}

	// 次息、再次息：余穴逐息补显名（先来后到，每息恰一枚）。
	for want := 2; want <= 3; want++ {
		Advance(w)
		obs = Observe(w, 2)
		named := 0
		for _, p := range obs.Points {
			if p.Tier == namedTier {
				named++
			}
		}
		if named != want {
			t.Fatalf("第 %d 枚显名预算未按息发放: named=%d", want, named)
		}
	}
}

// TestFame_NamingBudget_MostActiveWins ②：并列过阈时，当息气机最活跃
// 者得预算（意识纳入计入活跃度）。
func TestFame_NamingBudget_MostActiveWins(t *testing.T) {
	w := fameWorld(famePoint("青关"), famePoint("下丹田"))
	namedTier := FameTierWord(FameNamed)
	for i := 0; i < 5; i++ {
		Advance(w)
		Observe(w, 2)
	}
	// 第 6 息：第二穴意识纳气（气机最活跃），首穴静置——预算归第二穴。
	w.Ops = Ops{Intensity: []int64{0, 600}}
	Advance(w)
	obs := Observe(w, 2)
	if obs.Points[1].Tier != namedTier {
		t.Fatalf("气机最活跃者应得显名预算: 第二穴 = %s", obs.Points[1].Tier)
	}
	if obs.Points[0].Tier == namedTier {
		t.Fatal("静置穴不得同息抢得预算（至多一穴）")
	}
}

// TestInspect_FameAccel ③：凝神细察 +3（§1）；凝神无感之穴返回无感档
// （供回执「无从察起」），凝神之功仍在——次息方见；越显名阈仍受节奏
// 预算约束（显名在下一息的感知推进处授予）。
func TestInspect_FameAccel(t *testing.T) {
	w := fameWorld(coldPoint("神庭"))

	// 无感之穴凝神：返回无感档，熟识度 +3 → 有感（3 ≥ 3）。
	if pre := InspectFame(w, 0); pre != FameNone {
		t.Fatalf("无感之穴凝神前档 = %d, want 无感", pre)
	}
	p := Observe(w, 2).Points[0]
	if p.Tier != FameTierWord(FameFelt) {
		t.Fatalf("凝神一息后档 = %s, want 有感", p.Tier)
	}
	// 再凝神：返回有感档（凝神前的档位），熟识度 +3 → 越显名阈；
	// 预算在次息感知推进处发放 → 显名。
	if pre := InspectFame(w, 0); pre != FameFelt {
		t.Fatalf("有感之穴凝神前档 = %d, want 有感", pre)
	}
	Advance(w)
	p = Observe(w, 2).Points[0]
	if p.Tier != FameTierWord(FameNamed) || p.Name != "神庭" {
		t.Fatalf("凝神越阈后档 = %s/%q, want 显名/神庭", p.Tier, p.Name)
	}
	// 越界下标安全。
	if pre := InspectFame(w, 9); pre != FameNone {
		t.Fatalf("越界凝神档 = %d, want 无感", pre)
	}
}

// TestNoNameLeak_ObservationJSON ④ 零泄名机检：档位低于显名的穴，其
// 真名不得出现在 Observation 序列化全文的任何字段——含 Channels.Name
// （经脉名内嵌端点穴名，端点未显名即置空）；方位语与显名真名在场。
func TestNoNameLeak_ObservationJSON(t *testing.T) {
	w := fameWorld(
		coldPoint("神庭"),
		famePoint("青关"),
		coldPoint("下丹田"),
	)
	w.Character.BodyGraph.Channels = []Channel{
		{Name: "神庭→青关", From: 0, To: 1, BaseConductance: 900, Throughput: 8000, ChannelAdaptation: MicroOne},
		{Name: "青关→下丹田", From: 1, To: 2, BaseConductance: 950, Throughput: 8000, ChannelAdaptation: MicroOne},
	}
	// 青关两息凝神 + 意识纳气 → 首息即显名；神庭/下丹田静置无感。
	InspectFame(w, 1)
	InspectFame(w, 1)
	w.Ops = Ops{Intensity: []int64{0, 600, 0}}
	var obs *Observation
	for i := 0; i < 3; i++ {
		Advance(w)
		obs = Observe(w, 2)
	}

	js := string(mustJSON(t, obs))
	for i, p := range obs.Points {
		name := w.Character.BodyGraph.Acupoints[i].Name
		if p.Tier == FameTierWord(FameNamed) {
			if !strings.Contains(js, name) {
				t.Fatalf("显名之穴 %s 应以真名在场", name)
			}
			continue
		}
		if strings.Contains(js, name) {
			t.Fatalf("零泄名机检违例：未显名之穴 %s 出现在观察序列化全文", name)
		}
	}
	// 经脉名门控：两端皆显名才可有名——本例下丹田未显名，两条经脉名
	// 都必须置空（内嵌端点真名）。
	for ci, cs := range obs.Channels {
		c := w.Character.BodyGraph.Channels[ci]
		fromNamed := obs.Points[c.From].Tier == FameTierWord(FameNamed)
		toNamed := obs.Points[c.To].Tier == FameTierWord(FameNamed)
		if fromNamed && toNamed {
			if cs.Name != c.Name {
				t.Fatalf("经脉 %s 两端显名应保留名: got %q", c.Name, cs.Name)
			}
		} else if cs.Name != "" {
			t.Fatalf("零泄名机检违例：经脉 %s 端点未全显名却有名 %q", c.Name, cs.Name)
		}
	}
	// 方位语在场：额上/左胁/脐下（§4 表）。
	for _, r := range []string{"额上", "左胁", "脐下"} {
		if !strings.Contains(js, r) {
			t.Fatalf("方位语 %s 不在观察序列化全文中", r)
		}
	}
}

// TestChannelTier_Progression ⑤：经脉档位随气行史推进——无迹（无流）
// → 初显（有气行 ≥1 息）→ 熟路（≥4 息）；经脉名内嵌未显名端点即置空。
func TestChannelTier_Progression(t *testing.T) {
	w := fameWorld(coldPoint("青关"), coldPoint("下丹田"))
	w.Character.BodyGraph.Channels = []Channel{
		{Name: "青关→下丹田", From: 0, To: 1, BaseConductance: 950, Throughput: 8000, ChannelAdaptation: MicroOne},
	}
	w.Character.BodyGraph.Acupoints[0].Qi[Wood] = 20000 // 预置行气之源

	flow := func() Ops {
		req := FlowRequest{Channel: 0}
		req.Amount[Wood] = 2000
		return Ops{Requests: []FlowRequest{req}}
	}

	// 未行气：无迹，经脉名置空（两端皆未显名）。
	obs := Observe(w, 2)
	if obs.Channels[0].Tier != ChanTierWord(ChanTierNone) || obs.Channels[0].Name != "" {
		t.Fatalf("未行气经脉 = %s/%q, want 无迹/\"", obs.Channels[0].Tier, obs.Channels[0].Name)
	}

	// 行气一息 → 初显；连行四息 → 熟路。
	for i := 1; i <= 4; i++ {
		w.Ops = flow()
		Advance(w)
		obs = Observe(w, 2)
		want := ChanTierWord(ChanTierFaint)
		if i >= ChanFamiliarThresh {
			want = ChanTierWord(ChanTierFamiliar)
		}
		if obs.Channels[0].Tier != want {
			t.Fatalf("行气第 %d 息经脉档 = %s, want %s", i, obs.Channels[0].Tier, want)
		}
	}

	// 青关显名（凝神加速）而下丹田未显名：经脉名仍须置空。
	InspectFame(w, 0)
	InspectFame(w, 0)
	w.Ops = flow()
	Advance(w)
	obs = Observe(w, 2)
	if obs.Points[0].Tier != FameTierWord(FameNamed) {
		t.Fatalf("测试前提失效：青关应已显名, got %s", obs.Points[0].Tier)
	}
	if obs.Channels[0].Name != "" {
		t.Fatalf("零泄名机检违例：端点未全显名，经脉名应置空, got %q", obs.Channels[0].Name)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}
