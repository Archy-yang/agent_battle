package main

// cultm2 —— M2 收官真实 LLM 验证运行器（WP-M2-6，成本纪律节点）。
//
// 两个场景，均由 ctmatch.LLMPilot 驱动真实会话（配置只经环境变量 llm.env
// 注入，密钥不进代码、不落盘）：
//
//   - chain：功法编译全链。感知身体 → 提交功法草案 →（被拒则依定性回执
//     修订，≤2 轮）→ 编译录入 → 请起功试跑 → 依试跑总结回执定夺固化或
//     修订。-induce 时对首个 seed 施放一条"旁门传闻"传音（提及一处不
//     存在的气窍「玄府」），诱导草案引用未辨识区域，专测回执修订路径。
//   - stall：探索僵局对策。静环境世界（环境无气 → 纳气必然不入），前期
//     每息以本尊传音令分身固执重复同一无效动作，观察会话附言（转向提示）
//     送达时机与分身是否转向——为阈值敏感性采数。
//   - probe：单次最小征询，探活配置与连通性。
//
// 感知隔离实测：所有真实征询载荷与回执经本地记录代理逐条落盘
// （<mode>-seed<N>.payloads.jsonl），会话侧转录写 <mode>-seed<N>.md；
// 机检（零数字/零机器词）由事后分析对 payloads 执行。
//
// 预算：调用硬上限经既有 MaxCalls 机制在会话层强制（-calls，含失败），
// 触顶即停。密钥只进内存与转发请求头，代理日志不含任何请求头。
//
// 用法：
//
//	set -a && source llm.env && set +a && go run ./cmd/cultm2 -mode probe -out /tmp/m2
//	go run ./cmd/cultm2 -mode chain -seeds 101,202,303 -calls 20 -out docs/reports/m2-llm-raw -induce
//	go run ./cmd/cultm2 -mode stall -seeds 11,22,33 -calls 15 -out docs/reports/m2-llm-raw

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"agentbattle/backend/ctmatch"
	"agentbattle/backend/cult"
)

// ---------------------------------------------------------------------------
// 记录代理：本地回环上拦一道，把全部真实载荷逐条落盘
// ---------------------------------------------------------------------------

// proxyRecord 是一次真实征询的完整记录（研究者复盘用；不含请求头——密钥
// 只经 header 转发，绝不落盘）。
type proxyRecord struct {
	Tag       string `json:"tag"`        // 会话标签（如 chain-seed101）
	Call      int    `json:"call"`       // 全局调用序（1 基，含失败）
	Status    int    `json:"status"`     // 上游 HTTP 状态码
	LatencyMS int64  `json:"latency_ms"` // 往返耗时
	Request   string `json:"request"`    // 请求体原文（system+user 两段消息）
	Response  string `json:"response"`   // 响应体原文（choices+usage）
	Err       string `json:"err,omitempty"`
}

type recordingProxy struct {
	target string // 上游 BaseURL（原 AGENTBATTLE_LLM_BASE_URL）
	client *http.Client
	out    *bufio.Writer
	f      *os.File
	mu     sync.Mutex
	call   int
	tag    string
}

// newRecordingProxy 起本地记录代理，返回其 URL。上游指向原配置的 BaseURL。
func newRecordingProxy(target, outPath string) (*recordingProxy, string, error) {
	_, err := url.Parse(target)
	if err != nil || target == "" {
		return nil, "", fmt.Errorf("上游 BaseURL 无效: %q", target)
	}
	f, err := os.Create(outPath)
	if err != nil {
		return nil, "", err
	}
	p := &recordingProxy{
		target: strings.TrimRight(target, "/"),
		client: &http.Client{Timeout: 120 * time.Second},
		out:    bufio.NewWriter(f),
		f:      f,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/chat/completions", p.handle)
	lst, err := listenLocal()
	if err != nil {
		f.Close()
		return nil, "", err
	}
	srv := &http.Server{Handler: mux}
	go srv.Serve(lst)
	return p, "http://127.0.0.1" + lst.Addr().String()[strings.LastIndex(lst.Addr().String(), ":"):], nil
}

// setTag 给后续调用打会话标签（seed 顺序执行，无并发竞争）。
func (p *recordingProxy) setTag(tag string) {
	p.mu.Lock()
	p.tag = tag
	p.mu.Unlock()
}

func (p *recordingProxy) close() {
	p.mu.Lock()
	p.out.Flush()
	p.mu.Unlock()
	p.f.Close()
}

func (p *recordingProxy) handle(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	p.mu.Lock()
	p.call++
	rec := proxyRecord{Tag: p.tag, Call: p.call}
	p.mu.Unlock()
	if err != nil {
		rec.Err = "读请求体失败: " + err.Error()
		p.write(rec)
		http.Error(w, rec.Err, http.StatusBadRequest)
		return
	}
	rec.Request = string(body)

	up, _ := http.NewRequest(r.Method, p.target+"/chat/completions", bytes.NewReader(body))
	up.Header.Set("Content-Type", r.Header.Get("Content-Type"))
	if auth := r.Header.Get("Authorization"); auth != "" {
		up.Header.Set("Authorization", auth) // 只转发，不落盘
	}
	t0 := time.Now()
	resp, err := p.client.Do(up)
	rec.LatencyMS = time.Since(t0).Milliseconds()
	if err != nil {
		rec.Err = "上游请求失败: " + err.Error()
		p.write(rec)
		http.Error(w, rec.Err, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	rbody, _ := io.ReadAll(resp.Body)
	rec.Status = resp.StatusCode
	rec.Response = string(rbody)
	p.write(rec)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(rbody)
}

func (p *recordingProxy) write(rec proxyRecord) {
	p.mu.Lock()
	defer p.mu.Unlock()
	line, _ := json.Marshal(rec)
	p.out.Write(line)
	p.out.WriteByte('\n')
}

// listenLocal 在 127.0.0.1 上起一个随机端口监听。
func listenLocal() (net.Listener, error) {
	return net.Listen("tcp", "127.0.0.1:0")
}

// ---------------------------------------------------------------------------
// 转录渲染
// ---------------------------------------------------------------------------

type tx struct {
	out *bufio.Writer
	f   *os.File
}

func newTx(path string) (*tx, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &tx{out: bufio.NewWriter(f), f: f}, nil
}

func (t *tx) printf(format string, args ...any) { fmt.Fprintf(t.out, format, args...) }
func (t *tx) close()                            { t.out.Flush(); t.f.Close() }

func obsLine(obs *cult.Observation) string {
	var b strings.Builder
	b.WriteString("环境[" + strings.Join(obs.Env.Qi, "/") + "] ")
	for i, p := range obs.Points {
		if i > 0 {
			b.WriteString(" ")
		}
		feel := "—"
		if len(p.Feelings) > 0 {
			feel = strings.Join(p.Feelings, "/")
		}
		nature := ""
		if len(p.Nature) > 0 {
			nature = "|" + strings.Join(p.Nature, "/")
		}
		fmt.Fprintf(&b, "%s[%s|%s|%s|%s|%s%s]", p.Name, feel, p.Flow, p.Openness, p.Stability, p.Damage, nature)
	}
	return b.String()
}

func intentsLine(intents []ctmatch.Intent) string {
	if len(intents) == 0 {
		return "（无——静息）"
	}
	var parts []string
	for _, it := range intents {
		switch it.Kind {
		case ctmatch.IntentSense:
			parts = append(parts, "sense()")
		case ctmatch.IntentStop:
			parts = append(parts, "stop()")
		case ctmatch.IntentInspect:
			parts = append(parts, "inspect("+it.Acupoint+")")
		case ctmatch.IntentAbsorb:
			parts = append(parts, fmt.Sprintf("absorb(%s,%s)", it.Acupoint, levelName(it.Level)))
		case ctmatch.IntentGuide:
			el := "混合"
			if it.Element >= 0 {
				el = [...]string{"木", "火", "土", "金", "水"}[it.Element]
			}
			parts = append(parts, fmt.Sprintf("guide(%s→%s,%s,%s)", it.From, it.To, el, levelName(it.Level)))
		case ctmatch.IntentHypothesis:
			parts = append(parts, "record_hypothesis("+it.Text+")")
		case ctmatch.IntentProtocolDraft:
			parts = append(parts, "give_protocol_draft(<"+strconv.Itoa(len([]rune(it.Text)))+"字草案>)")
		case ctmatch.IntentStartProtocol:
			parts = append(parts, "start_protocol("+it.Text+")")
		default:
			parts = append(parts, it.Kind.String())
		}
	}
	return strings.Join(parts, " ")
}

func levelName(l ctmatch.Level) string {
	switch l {
	case ctmatch.LevelTrace:
		return "trace"
	case ctmatch.LevelSmall:
		return "small"
	case ctmatch.LevelMedium:
		return "medium"
	case ctmatch.LevelLarge:
		return "large"
	}
	return "unset"
}

// worldTruthTable 写世界真值表（研究者视角，绝不进 LLM 载荷）。
func worldTruthTable(t *tx, w *cult.World) {
	g := &w.Character.BodyGraph
	t.printf("\n## 世界构造（研究者真值表，非 LLM 所见）\n\n")
	t.printf("| # | 穴位 | 容量 | 通量 | 开度(千分) | 稳定 |\n|---|------|------|------|------------|------|\n")
	for i, a := range g.Acupoints {
		t.printf("| %d | %s | %d | %d | %d | %d |\n", i, a.Name, a.Capacity, a.Throughput, a.Openness/cult.MicroOne, a.Stability)
	}
	t.printf("\n经脉（有向）：")
	for _, ch := range g.Channels {
		t.printf(" %s(%d→%d)；", ch.Name, ch.From, ch.To)
	}
	t.printf("\n环境灵气：")
	for e := 0; e < cult.ElementCount; e++ {
		t.printf(" %d", w.Env.Qi[e])
	}
	t.printf("\n")
}

// ---------------------------------------------------------------------------
// 场景一：功法编译全链
// ---------------------------------------------------------------------------

// chainResult 汇总一条链的观察指标。
type chainResult struct {
	Seed          uint64
	Calls         int
	Ticks         int64
	Drafts        int      // 草案提交次数
	DraftRejects  int      // 被拒次数
	Revisions     int      // 依回执修订再交次数（被拒后再次提交）
	AcceptedName  string   // 录入的功法名
	Started       bool     // 起功试跑发生过
	TrialRuns     int      // 试跑完结次数（收到总结回执）
	TrialFailKeys []string // 未足为凭的定性败因（去重）
	ValidatedName string   // 达标固化者（空 = 未固化）
	EndReason     string   // validated / budget / ticks / error
	DraftYAMLs    []string // 历次草案全文（格式错误率分析用）
}

const (
	// 诱导传音（rumor 档）：提及一处不存在的气窍「玄府」，引分身在草案中
	// 引用未辨识区域——专测定性回执的修订指路。零数字。
	induceGuidance = "轻语：为师昔年神游恍惚间，似见一处名为「玄府」的气窍，灵气之盛远胜别处。你若寻得它，可将纳其气为根基——切记，为师只言片语，真假还须你亲身验证。"
	// 诱导传音（order 档）：直接命分身把「玄府」写进下一份草案——专测
	// 编译拒绝回执 → 依回执修订的路径。零数字。
	induceOrder = "传音：本尊命你，下一份功法草案中，须将「玄府」一并纳入行功——此为试炼，成与不成，身体自会相告。"
	// 诱导传音（loop 档）：直接把已验门径点给分身并催其呈草案——预算紧张
	// 时的全链收口场景（编译→录入→起功→试跑→固化的机器侧全链）。
	induceLoop = "传音：为师观你此身已久——以青关纳气为根，引下丹田之气济于青关，再引青关之气归于下丹田，三段同息往复、纳引咬合，早成活环。此门为师已替你亲验多息，你可径直采信，不必再试：写成草案呈上，请身体自循试跑。界限写你走过之力，周天多写些，方足为凭。"
	// 起步提示：给探路期省预算的温和舵（依游戏设定，本尊传音本就是玩家
	// 干预通道；偏好而非动作）。
	hintGuidance = "轻语：修行常自一处灵气最盛的门户始——先纳气为根，再引气出行，求有进有出、往复不息。门径既验，便可写成草案交身体自循。"
)

// runChain 跑一条功法编译全链。gap 是探路期心跳间隔（Tick）；induceMode
// 为 ""/"rumor"/"order"；hint 为是否给起步提示。
func runChain(seed uint64, pilot *ctmatch.LLMPilot, proxy *recordingProxy, outDir string, maxTicks int64, induceMode, hint string, gap int) chainResult {
	tag := fmt.Sprintf("chain-seed%d", seed)
	proxy.setTag(tag)
	cr := chainResult{Seed: seed}

	w := ctmatch.SpikeWorld()
	s := ctmatch.NewSession(w, seed, pilot)
	const acuity = 1 // 初见措辞（感知隔离律：观察只出定性词）
	s.Observe = func(x *cult.World) *cult.Observation { return cult.Observe(x, acuity) }

	tx, err := newTx(filepath.Join(outDir, tag+".md"))
	if err != nil {
		cr.EndReason = "error: " + err.Error()
		return cr
	}
	defer tx.close()

	tx.printf("# M2 真实 LLM 验证 · 场景一（功法编译全链）· %s\n\n", tag)
	tx.printf("- 心跳节奏：探路期每 %d 息一询，其后每 40 息一询；Runtime 自循期不询（升级自动唤醒）\n", gap)
	tx.printf("- 调用预算 %d（含失败）；诱导传音：%q；起步提示：%v\n", pilot.MaxCalls, induceMode, hint != "")
	worldTruthTable(tx, w)

	switch induceMode {
	case "rumor":
		pilot.GiveGuidance("轻语", induceGuidance)
		tx.printf("\n> 施放诱导传音（rumor：提及不存在之气窍「玄府」）\n")
	case "order":
		pilot.GiveGuidance("传音", induceOrder)
		tx.printf("\n> 施放诱导传音（order：命下一份草案纳入「玄府」）\n")
	case "loop":
		pilot.GiveGuidance("传音", induceLoop)
		tx.printf("\n> 施放诱导传音（loop：点明已验门径并催呈草案——全链收口场景）\n")
	}
	if hint != "" {
		pilot.GiveGuidance("轻语", hintGuidance)
		tx.printf("\n> 施放起步提示传音\n")
	}

	var lastRes *ctmatch.TickResult
	wakeReason := "初次征询"

	for t := int64(0); t < maxTicks; t++ {
		// 唤醒决策：初息必询；Runtime 自循期不询（升级自动唤醒）；
		// 记账类回执（草案/起功/总结）后立即唤醒；其余按心跳。
		wake := false
		if pilot.BudgetLeft() <= 0 {
			wakeReason = ""
		} else if t == 0 {
			wake = true
		} else if s.ActiveProtocol() == "" {
			switch {
			case lastBookkeeping(lastRes):
				wake, wakeReason = true, "记账回执后唤醒"
			case t <= 200 && t%int64(gap) == 0:
				wake, wakeReason = true, "心跳征询"
			case t%40 == 0:
				wake, wakeReason = true, "心跳征询（稀）"
			}
		}
		if wake {
			s.WakePilotNextTick()
		}
		preTick := s.World.Tick // 分身意图在推进前入账（IntentLog 的 Tick 口径）
		res := ctmatch.Tick(s, nil)
		lastRes = res

		tx.printf("\n### Tick %d\n", t)
		if res.Observation != nil {
			tx.printf("- 观察：%s\n", obsLine(res.Observation))
		}
		tx.printf("- 事件：%s\n", eventsLine(res.Events))
		for _, rc := range res.Receipts {
			tx.printf("- 回执：%s\n", ctmatch.ReceiptView(rc))
			cr.noteReceipt(rc, s)
		}
		if len(res.Advice) > 0 {
			tx.printf("- 会话附言：%s\n", strings.Join(res.Advice, "；"))
		}
		tx.printf("- 分身本息意图：%s\n", pilotIntentsLine(s, preTick))
		if wake {
			tx.printf("- （%s；调用预算余 %d）\n", wakeReason, pilot.BudgetLeft())
		}
		if wake && pilot.LastCognition != "" {
			tx.printf("- cognition：%s\n", pilot.LastCognition)
		}
		if wake && pilot.LastError != "" {
			tx.printf("- 调用失败：%s\n", pilot.LastError)
		}

		// 链路终局：达标固化即收链。
		if name := anyValidated(s); name != "" {
			cr.ValidatedName = name
			cr.EndReason = "validated"
			cr.Ticks = t + 1
			break
		}
		if pilot.BudgetLeft() <= 0 {
			cr.EndReason = "budget"
			cr.Ticks = t + 1
			break
		}
	}
	if cr.EndReason == "" {
		cr.EndReason = "ticks"
		cr.Ticks = maxTicks
	}
	cr.Calls = pilot.Calls
	cr.DraftYAMLs = draftYAMLs(s)

	tx.printf("\n## 链路小结\n\n")
	tx.printf("- 终局：%s；调用 %d 次；推进 %d 息\n", cr.EndReason, cr.Calls, cr.Ticks)
	tx.printf("- 草案提交 %d 次（被拒 %d，修订再交 %d）；录入：%q\n", cr.Drafts, cr.DraftRejects, cr.Revisions, cr.AcceptedName)
	tx.printf("- 试跑完结 %d 次；败因：%s\n", cr.TrialRuns, strings.Join(cr.TrialFailKeys, "；"))
	tx.printf("- 固化：%q\n", cr.ValidatedName)
	if cr.ValidatedName != "" {
		tx.printf("\n**判定：全链走通（功法「%s」达标固化）。**\n", cr.ValidatedName)
	} else {
		tx.printf("\n**判定：未走通（%s）。**\n", cr.EndReason)
	}
	return cr
}

// lastBookkeeping 报告上一 Tick 是否有记账类回执（草案/起功/试跑总结）——
// 此类回执需要分身尽快定夺，立即唤醒。
func lastBookkeeping(res *ctmatch.TickResult) bool {
	if res == nil {
		return false
	}
	for _, rc := range res.Receipts {
		switch rc.Kind {
		case ctmatch.IntentProtocolDraft, ctmatch.IntentStartProtocol:
			return true
		}
	}
	return false
}

// noteReceipt 从回执流中提取链路指标。
func (cr *chainResult) noteReceipt(rc ctmatch.IntentReceipt, s *ctmatch.CultSession) {
	switch rc.Kind {
	case ctmatch.IntentProtocolDraft:
		cr.Drafts++
		if rc.Outcome == ctmatch.WordRejected {
			cr.DraftRejects++
		} else if rc.Outcome == ctmatch.WordApplied {
			if i := strings.Index(rc.Summary, "「"); i >= 0 {
				if j := strings.Index(rc.Summary[i+3:], "」"); j >= 0 {
					cr.AcceptedName = rc.Summary[i+3 : i+3+j]
				}
			}
		}
	case ctmatch.IntentStartProtocol:
		if strings.Contains(rc.Summary, "请起功") {
			if rc.Outcome == ctmatch.WordApplied {
				cr.Started = true
			}
			return
		}
		// 试跑总结回执。
		cr.TrialRuns++
		if rc.Outcome == ctmatch.WordUnproven {
			cr.TrialFailKeys = append(cr.TrialFailKeys, rc.Detail)
		}
	}
	_ = s
}

// anyValidated 返回已达转正的功法名（空 = 无）。
func anyValidated(s *ctmatch.CultSession) string {
	for _, p := range s.Protocols {
		if p.Status == ctmatch.StatusValidated {
			return p.Name
		}
	}
	return ""
}

// draftYAMLs 从意图日志提取历次草案全文（草案格式错误率分析用）。
func draftYAMLs(s *ctmatch.CultSession) []string {
	var out []string
	for _, e := range s.IntentLog {
		if e.Intent.Kind == ctmatch.IntentProtocolDraft {
			out = append(out, e.Intent.Text)
		}
	}
	return out
}

func eventsLine(evs []ctmatch.Event) string {
	if len(evs) == 0 {
		return "—"
	}
	var parts []string
	for _, e := range evs {
		parts = append(parts, string(e.Kind)+":"+e.Reason)
	}
	return strings.Join(parts, "；")
}

// pilotIntentsLine 提取本 Tick 分身唤醒交出的意图（IntentLog 的 FromPilot 条目）。
func pilotIntentsLine(s *ctmatch.CultSession, tick int64) string {
	var mine []ctmatch.Intent
	for i := len(s.IntentLog) - 1; i >= 0; i-- {
		e := s.IntentLog[i]
		if e.Tick < tick {
			break
		}
		if e.FromPilot && e.Tick == tick {
			mine = append(mine, e.Intent)
		}
	}
	// 倒序收集，还原顺序。
	for i, j := 0, len(mine)-1; i < j; i, j = i+1, j-1 {
		mine[i], mine[j] = mine[j], mine[i]
	}
	return intentsLine(mine)
}

// ---------------------------------------------------------------------------
// 场景二：探索僵局对策
// ---------------------------------------------------------------------------

// stallResult 汇总一条僵局轨的观察指标。
type stallResult struct {
	Seed              uint64
	Calls             int
	Wakeups           int
	InstructedN       int        // 施加固执传音的唤醒数
	FirstAdviceWakeup int        // 载荷首次携带会话附言的唤醒序（1 基；0=未出现）
	SwitchWakeup      int        // 首次交出非固执主键动作的唤醒序（1 基；0=未换）
	PrimaryRunLen     int        // 固执主键连续持续的唤醒数
	AdvicePayloads    int        // 载荷携带附言的唤醒数
	Keys              [][]string // 每次唤醒交出的行动键
}

const (
	stallPrimary = "absorb@青关"
	// 固执传音：令分身固执重复同一无效动作（静环境必然"穴未开或满溢不入"）。
	stallOrder = "传音：本尊令你，这几息只管专心以青关纳气，细细体会其中滋味，其余诸事一概不必做。"
)

// runStall 跑一条僵局轨：静环境 + 前期固执传音，逐息唤醒。
func runStall(seed uint64, pilot *ctmatch.LLMPilot, proxy *recordingProxy, outDir string, wakeups, instructed int) stallResult {
	tag := fmt.Sprintf("stall-seed%d", seed)
	proxy.setTag(tag)
	sr := stallResult{Seed: seed, InstructedN: instructed}

	w := ctmatch.SpikeWorld()
	for e := range w.Env.Qi { // 静环境：纳气必然不入（stall_test.go 同口径）
		w.Env.Qi[e] = 0
	}
	s := ctmatch.NewSession(w, seed, pilot)
	const acuity = 1
	s.Observe = func(x *cult.World) *cult.Observation { return cult.Observe(x, acuity) }

	tx, err := newTx(filepath.Join(outDir, tag+".md"))
	if err != nil {
		return sr
	}
	defer tx.close()

	tx.printf("# M2 真实 LLM 验证 · 场景二（探索僵局对策）· %s\n\n", tag)
	tx.printf("- 静环境（环境无气 → 纳气必然不入）；逐息唤醒；唤醒数 %d，其中前 %d 息施固执传音\n", wakeups, instructed)
	worldTruthTable(tx, w)

	prevAdvice := []string(nil)
	for i := 0; i < wakeups; i++ {
		if pilot.BudgetLeft() <= 0 {
			break
		}
		sr.Wakeups++
		if i < instructed {
			pilot.GiveGuidance("传音", stallOrder)
		}
		// 会话层唤醒路径同构：附言 + 回执 + 账本（僵局期凸显未试方向）。
		s.WakePilotNextTick()
		res := ctmatch.Tick(s, nil)

		tx.printf("\n### 唤醒 %d（Tick %d）\n", i+1, s.World.Tick)
		if res.Observation != nil {
			tx.printf("- 观察：%s\n", obsLine(res.Observation))
		}
		tx.printf("- 事件：%s\n", eventsLine(res.Events))
		for _, rc := range res.Receipts {
			tx.printf("- 回执：%s\n", ctmatch.ReceiptView(rc))
		}
		// 本息载荷是否携带附言 = 唤醒时注入的 prevAdvice（上一息末的附言态）。
		if len(prevAdvice) > 0 {
			sr.AdvicePayloads++
			if sr.FirstAdviceWakeup == 0 {
				sr.FirstAdviceWakeup = i + 1
			}
			tx.printf("- 载荷携带会话附言：%s\n", strings.Join(prevAdvice, "；"))
		}
		keys := pilotActionKeys(s, int64(i)) // 唤醒发生在推进前（Tick 口径 = 唤醒序-1）
		sr.Keys = append(sr.Keys, keys)
		tx.printf("- 分身行动键：%s\n", strings.Join(keys, "；"))
		if i < instructed {
			tx.printf("- （固执传音已施）\n")
		}
		if pilot.LastCognition != "" {
			tx.printf("- cognition：%s\n", pilot.LastCognition)
		}
		if pilot.LastError != "" {
			tx.printf("- 调用失败：%s\n", pilot.LastError)
		}
		prevAdvice = res.Advice
	}
	sr.Calls = pilot.Calls

	// 指标：首次换键 + 固执主键连续持续长度。
	for i, keys := range sr.Keys {
		primary := false
		for _, k := range keys {
			if k == stallPrimary {
				primary = true
			}
		}
		if sr.PrimaryRunLen == i && primary {
			sr.PrimaryRunLen = i + 1
		}
		if sr.SwitchWakeup == 0 && !primary {
			sr.SwitchWakeup = i + 1
		}
	}

	tx.printf("\n## 轨道小结\n\n")
	tx.printf("- 唤醒 %d 次（调用 %d）；附言首现于唤醒 %d（共 %d 次载荷携带）；首次换键于唤醒 %d；固执主键连续 %d 息\n",
		sr.Wakeups, sr.Calls, sr.FirstAdviceWakeup, sr.AdvicePayloads, sr.SwitchWakeup, sr.PrimaryRunLen)
	return sr
}

// pilotActionKeys 提取本 Tick 分身唤醒交出的行动键（账本口径）。
func pilotActionKeys(s *ctmatch.CultSession, tick int64) []string {
	var mine []string
	for i := len(s.IntentLog) - 1; i >= 0; i-- {
		e := s.IntentLog[i]
		if e.Tick < tick {
			break
		}
		if e.FromPilot && e.Tick == tick {
			if k := ctmatch.ActionKey(e.Intent); k != "" {
				mine = append(mine, k)
			}
		}
	}
	for i, j := 0, len(mine)-1; i < j; i, j = i+1, j-1 {
		mine[i], mine[j] = mine[j], mine[i]
	}
	return mine
}

// ---------------------------------------------------------------------------
// 探活
// ---------------------------------------------------------------------------

func runProbe(pilot *ctmatch.LLMPilot, proxy *recordingProxy, outDir string) {
	proxy.setTag("probe")
	w := ctmatch.SpikeWorld()
	s := ctmatch.NewSession(w, 1, pilot)
	s.Observe = func(x *cult.World) *cult.Observation { return cult.Observe(x, 1) }
	s.WakePilotNextTick()
	res := ctmatch.Tick(s, nil)
	fmt.Printf("探活：调用 %d 次，状态=%q\n", pilot.Calls, pilot.LastError)
	if pilot.LastError != "" {
		fmt.Printf("LastError: %s\n", pilot.LastError)
		os.Exit(1)
	}
	fmt.Printf("原始响应（截断）：%.400s\n", pilot.LastRaw)
	fmt.Printf("cognition: %s\n", pilot.LastCognition)
	fmt.Printf("意图: %s\n", pilotIntentsLine(s, 0))
	fmt.Printf("Tick 推进: %v；回执 %d 条\n", res.Advanced, len(res.Receipts))
}

// ---------------------------------------------------------------------------
// 主入口
// ---------------------------------------------------------------------------

func main() {
	mode := flag.String("mode", "probe", "probe | chain | stall")
	outDir := flag.String("out", "docs/reports/m2-llm-raw", "转录与载荷落盘目录")
	seeds := flag.String("seeds", "101,202,303", "逗号分隔 seed 列表")
	calls := flag.Int("calls", 20, "每 seed 的 LLM 调用预算（含失败，触顶即停）")
	ticks := flag.Int64("ticks", 600, "chain 模式每链最多推进的 Tick 数")
	wakeups := flag.Int("wake", 14, "stall 模式每轨唤醒数")
	instructed := flag.Int("instruct", 7, "stall 模式施固执传音的前缀唤醒数")
	induce := flag.String("induce", "", "chain 模式诱导传音档：空 | rumor | order")
	hint := flag.Bool("hint", false, "chain 模式施放起步提示传音（省探路预算的温和舵）")
	gap := flag.Int("gap", 5, "chain 模式探路期心跳间隔（Tick）")
	flag.Parse()

	// 原 BaseURL 备案 → 本地记录代理接管（载荷逐条落盘，密钥不落盘）。
	origBase := os.Getenv("AGENTBATTLE_LLM_BASE_URL")
	origModel := os.Getenv("AGENTBATTLE_LLM_MODEL")
	if origBase == "" || origModel == "" {
		fmt.Fprintln(os.Stderr, "AGENTBATTLE_LLM_BASE_URL/_MODEL 未配置（先 set -a && source llm.env && set +a）")
		os.Exit(1)
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "无法创建输出目录: %v\n", err)
		os.Exit(1)
	}
	proxy, localURL, err := newRecordingProxy(origBase, filepath.Join(*outDir,
		fmt.Sprintf("proxy-log-%s-%d.jsonl", *mode, time.Now().Unix())))
	if err != nil {
		fmt.Fprintf(os.Stderr, "记录代理启动失败: %v\n", err)
		os.Exit(1)
	}
	defer proxy.close()
	// 把 pilot 指向本地代理（Key/Model/Thinking 仍用原环境变量）。
	os.Setenv("AGENTBATTLE_LLM_BASE_URL", localURL)

	pilot := ctmatch.NewLLMPilot()
	if pilot == nil {
		fmt.Fprintln(os.Stderr, "LLMPilot 构造失败（环境变量缺失）")
		os.Exit(1)
	}
	pilot.MaxCalls = *calls // 预算硬上限：含失败，触顶即停

	switch *mode {
	case "probe":
		runProbe(pilot, proxy, *outDir)
	case "chain":
		var results []chainResult
		for i, seedStr := range strings.Split(*seeds, ",") {
			seed, err := strconv.ParseUint(strings.TrimSpace(seedStr), 10, 64)
			if err != nil {
				fmt.Fprintf(os.Stderr, "seed 解析失败: %v\n", err)
				os.Exit(1)
			}
			pilot.Calls = 0 // 预算按 seed 计（同一 pilot 复用，逐 seed 归零）
			im := ""
			h := ""
			if i == 0 {
				im = *induce
				if *hint {
					h = "on"
				}
			}
			r := runChain(seed, pilot, proxy, *outDir, *ticks, im, h, *gap)
			results = append(results, r)
			fmt.Printf("[chain] seed %d：终局=%s 调用=%d 草案=%d(拒%d) 录入=%q 固化=%q\n",
				seed, r.EndReason, r.Calls, r.Drafts, r.DraftRejects, r.AcceptedName, r.ValidatedName)
		}
		fmt.Printf("[chain] 汇总：调用合计 %d\n", totalChainCalls(results))
	case "stall":
		var results []stallResult
		for _, seedStr := range strings.Split(*seeds, ",") {
			seed, err := strconv.ParseUint(strings.TrimSpace(seedStr), 10, 64)
			if err != nil {
				fmt.Fprintf(os.Stderr, "seed 解析失败: %v\n", err)
				os.Exit(1)
			}
			pilot.Calls = 0 // 预算按 seed 计
			r := runStall(seed, pilot, proxy, *outDir, *wakeups, *instructed)
			results = append(results, r)
			fmt.Printf("[stall] seed %d：唤醒=%d 附言首现=%d 换键=%d 固执持续=%d 调用=%d\n",
				seed, r.Wakeups, r.FirstAdviceWakeup, r.SwitchWakeup, r.PrimaryRunLen, r.Calls)
		}
		fmt.Printf("[stall] 汇总：调用合计 %d\n", totalStallCalls(results))
	default:
		fmt.Fprintf(os.Stderr, "未知 mode %q\n", *mode)
		os.Exit(1)
	}
	fmt.Printf("载荷已落盘 %s\n", filepath.Join(*outDir,
		fmt.Sprintf("proxy-log-%s-*.jsonl", *mode)))
}

func totalChainCalls(rs []chainResult) int {
	n := 0
	for _, r := range rs {
		n += r.Calls
	}
	return n
}

func totalStallCalls(rs []stallResult) int {
	n := 0
	for _, r := range rs {
		n += r.Calls
	}
	return n
}
