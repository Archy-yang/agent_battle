package ctmatch

// intent.go —— 玩家/Agent 意图与粗档位换算（stage_2 §26-27）。
//
// LLM 提交的是 Intent，Engine 决定实际结果（stage_2 §26）。
// 本包不做任何 LLM 调用：LLM 相关一律走 Pilot 接口，
// 具体实现由上层（ctmatch/llm.go，WP 后续）注入。

import (
	"fmt"

	"agentbattle/backend/cult"
)

// ---------------------------------------------------------------------------
// 粗档位（stage_2 §27）
// ---------------------------------------------------------------------------

// Level 是新手粗档位。角色只能表达"少量/中等/大量"，
// 由 Runtime 依角色当前能力换算成真实参数（stage_2 §27）。
type Level int

const (
	LevelUnset  Level = iota // 未指定（协议中用显式参数）
	LevelTrace               // 微量
	LevelSmall               // 少量
	LevelMedium              // 中等
	LevelLarge               // 大量
)

// ParseLevel 解析档位词（协议 YAML 用英文；中文名仅供调试）。
func ParseLevel(s string) (Level, error) {
	switch s {
	case "":
		return LevelUnset, nil
	case "trace", "微":
		return LevelTrace, nil
	case "small", "少量":
		return LevelSmall, nil
	case "medium", "middle", "中等":
		return LevelMedium, nil
	case "large", "大量":
		return LevelLarge, nil
	}
	return LevelUnset, fmt.Errorf("unknown level %q", s)
}

// levelPercent 是档位 → 千分位比率的换算表。
//
// 这是"控制精度随成长解锁"的集中落点（stage_2 §27）：新手只能给粗档位，
// 精确参数（Intensity/Amount 直填）要等 Control 成长、功法验证后才解锁。
// 改档位手感只改这一张表。
var levelPercent = [5]int64{0, 50, 250, 500, 800} // 0.05 / 0.25 / 0.50 / 0.80

// IntensityForLevel 把档位换算成吸收强度（千分位比率，∈(0,1000]）。
// 吸收的真实效果由引擎在 stage_3 §7 公式内依 openness/亲和/反压体现，
// 所以强度本身是"能力的百分比"，无需再乘身体参数。
func IntensityForLevel(l Level) int64 { return levelPercent[l] }

// FlowForLevel 把档位换算成一条经脉的单元素流量（量，千分位）：
//
//	flow = 档位百分比 × 通道有效通量
//	通道有效通量 = min(源穴位有效吞吐, 目标穴位有效吞吐, 经脉吞吐)
//
// 有效吞吐 = Throughput × (1 - Damage)（stage_3 §20——损伤真正改变运行能力）。
// 同一档位在不同身体状态下给出不同真实流量——这就是"依角色当前能力换算"。
func FlowForLevel(l Level, g *cult.BodyGraph, from, to, channel int) int64 {
	return scaleByPct(levelPercent[l], channelCapacity(g, from, to, channel))
}

// channelCapacity 返回一条经脉路径的有效通量（stage_3 §5/§8.1/§20）。
func channelCapacity(g *cult.BodyGraph, from, to, channel int) int64 {
	if channel < 0 || channel >= len(g.Channels) {
		return 0
	}
	cap := g.Channels[channel].Throughput
	for _, i := range [2]int{from, to} {
		if i < 0 || i >= len(g.Acupoints) {
			return 0
		}
		a := &g.Acupoints[i]
		eff := fpMul(a.Throughput, One-a.Damage)
		if eff < cap {
			cap = eff
		}
	}
	return cap
}

// scaleByPct 返回 x×pct/1000（pct 为千分位比率），四舍五入到千分位。
func scaleByPct(pct, x int64) int64 { return (x*pct + 500) / 1000 }

const One = 1000 // 定点 1.0（千分位，与 cult 包标度一致）

// ---------------------------------------------------------------------------
// 意图（stage_2 §26）
// ---------------------------------------------------------------------------

// IntentKind 是意图种类。
type IntentKind int

const (
	IntentSense         IntentKind = iota // sense() 主动感知环境
	IntentAbsorb                          // absorb(target, level)
	IntentGuide                           // guide(from, to, element, level, amount)
	IntentStop                            // stop() 停止一切操作
	IntentInspect                         // inspect(target) 内视指定目标
	IntentHypothesis                      // record_hypothesis(text) 登记猜想（stage_2 §26 第六接口）
	IntentProtocolDraft                   // give_protocol_draft(text) 提交功法 YAML 草案（M2 WP-M2-2）
	IntentStartProtocol                   // start_protocol(name) 请起功试跑某门已录入的功法（M2 WP-M2-2）
)

func (k IntentKind) String() string {
	switch k {
	case IntentSense:
		return "sense"
	case IntentAbsorb:
		return "absorb"
	case IntentGuide:
		return "guide"
	case IntentStop:
		return "stop"
	case IntentInspect:
		return "inspect"
	case IntentHypothesis:
		return "record_hypothesis"
	case IntentProtocolDraft:
		return "give_protocol_draft"
	case IntentStartProtocol:
		return "start_protocol"
	}
	return "unknown"
}

// Intent 是一条玩家/Agent 意图。字段按意图种类取用：
//
//	Sense             —— 无字段
//	Absorb            —— Acupoint、Level
//	Guide             —— From、To、Element、Level（或显式 Amount）
//	Stop              —— 无字段
//	Inspect           —— Acupoint
//	RecordHypothesis  —— Text（猜想原文）
//	ProtocolDraft     —— Text（功法 YAML 草案全文；M2 WP-M2-2。LLM 只提交
//	                     草案文本，编译/校验/定性回执由会话层裁决——宪法
//	                     §7：LLM 永不直接创造世界事实）
//	StartProtocol     —— Text（功法名；起功试跑，前置条件仍由会话层把关）
type Intent struct {
	Kind     IntentKind
	Acupoint string // Absorb 目标 / Inspect 目标（穴位名）
	From     string // Guide 源穴位名
	To       string // Guide 目标穴位名
	Element  int    // Guide 元素：cult.Wood..cult.Water；-1 = 混合（五均分）
	Level    Level  // 粗档位（stage_2 §27）
	Amount   int64  // Guide 显式流量（量，千分位；0 = 用档位换算）。
	// 显式流量是"精确控制"，属于 Control 成长后解锁的能力（§27）；
	// L2 探索时也可以直接提交——Engine 仍决定实际结果。
	Text string // RecordHypothesis 的猜想原文（stage_2 §26 第六接口）
}

// ---------------------------------------------------------------------------
// 感知观察（WP-2 已合入，别名直连正式类型）
// ---------------------------------------------------------------------------

// 观察类型即 cult/perception.go 的契约实现（M1 计划 §3.1），此处仅取
// 别名。注意感知隔离律：观察载荷只有定性词与档位，永无真值——本包
// 不提供任何"从 World 直接读数值生成 Observation"的路径，观察只能
// 经由 CultSession.Observe 钩子（生产装配即 cult.Observe）注入。
type AcupointSense = cult.AcupointSense
type EnvSense = cult.EnvSense
type Observation = cult.Observation

// ---------------------------------------------------------------------------
// Pilot 接口（L2；本 WP 只提供测试桩）
// ---------------------------------------------------------------------------

// Pilot 是 L2 Agent 的决策接口。Runtime 只在以下时刻唤醒它
// （stage_2 §33-34 行为生命周期的"出现异常→重新唤醒"）：
//
//   - 上一 Tick 产生了升级事件（GuardAdjusted stop / OutOfBoundary /
//     UnknownAnomaly），下一 Tick 开始时带最新观察征询意图；
//   - 上层显式调用（本 WP 不涉及）。
//
// 本包绝不 import 任何 LLM 客户端；真 LLM 实现在 llm.go（后续 WP）。
type Pilot interface {
	// Decide 基于观察返回下一组意图。obs 可能为 nil（Observe 钩子
	// 未接入时，SPIKE 前的桥接场景）；实现须容忍。
	Decide(obs *Observation) []Intent
}

// ScriptedPilot 是 Pilot 的脚本测试桩：按预先编排的脚本逐次弹出一组
// 意图，供测试与 SPIKE 前的桥接使用。
type ScriptedPilot struct {
	Script [][]Intent // 每次 Decide 依次弹出一条；耗尽后返回 nil
	// AfterAdvice 是"听劝"续接脚本（M2 WP-M2-3）：首次收到会话转向
	// 提示（非空 GiveAdvice）即切换到它逐次弹出（弃用 Script 余下部
	// 分，耗尽后同样返回 nil）；为空/nil 则对提示置之不理（"固执"），
	// 继续按 Script 弹出。
	AfterAdvice [][]Intent
	Calls       int // 已被征询次数（测试断言用）
	Last        *Observation

	// 会话层经可选接口给到的回执、账本视图与转向提示（测试断言用）。
	LastReceipts []IntentReceipt
	LastLedger   []string
	LastAdvice   []string

	afterAdvice bool // 收到提示后是否已切换到 AfterAdvice
	afterCalls  int  // 切换后的征询次数（AfterAdvice 的弹出下标）
}

func (p *ScriptedPilot) Decide(obs *Observation) []Intent {
	p.Calls++
	p.Last = obs
	if p.afterAdvice {
		// 听劝路径：弹出下标相对切换时刻计（弃用 Script 余下部分）。
		if p.afterCalls < len(p.AfterAdvice) {
			out := p.AfterAdvice[p.afterCalls]
			p.afterCalls++
			return out
		}
		return nil
	}
	if p.Calls-1 < len(p.Script) {
		return p.Script[p.Calls-1]
	}
	return nil
}
