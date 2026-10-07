package ctmatch

// draft.go —— 功法编译端到端（M2 WP-M2-2）：分身经 Intent 提交功法 YAML
// 草案 → Compile 校验 → 定性回执；草案录入后可请起功试跑；试跑完毕给
// 总结回执（成则固化在册、不成则交回修订），并唤醒分身定夺。
//
// 宪法落点：LLM 只提交 Intent（这里即 Protocol 草案文本），编译校验、
// 试跑起功、达标转正全部由会话层/引擎裁决——分身永不直接创造世界事实。
//
// 感知隔离律落点：草案校验的拒绝回执走 CompileError.AgentView（零数字、
// 零机器词、可指导修订）；试跑总结回执只给定性成败之词，周天账目数值
// （效率/稳定性）只留调试视图。起功与试跑对既有运行期（Runtime、护卫、
// 出界升级、移交门）零改动——试跑期分身照常受询、回执照常回流。

import (
	"errors"
	"strings"
)

// settleProtocolDraft 处理一份功法草案提交意图（非动作意图，不打断
// Runtime）：编译校验，非法即定性拒绝回执；合法即录入功法册（草创，
// 同名草案覆盖修订——但已固化的功法不可被覆盖）。
func (s *CultSession) settleProtocolDraft(it Intent) IntentReceipt {
	rc := IntentReceipt{Tick: s.World.Tick, Kind: it.Kind, Summary: "呈上一份功法草案"}
	if strings.TrimSpace(it.Text) == "" {
		rc.Outcome, rc.Detail = WordUnclear, "草案空无一字，无从校验。"
		return rc
	}
	p, err := Compile([]byte(it.Text), &s.World.Character.BodyGraph)
	if err != nil {
		var ce *CompileError
		if errors.As(err, &ce) && ce.Name != "" {
			rc.Summary = "呈上一份功法草案「" + ce.Name + "」"
		}
		rc.Outcome = WordRejected
		rc.Detail = "草创未成：" + strings.Join(draftErrLines(err), " ")
		return rc
	}
	if old, ok := s.Protocols[p.Name]; ok && old.Status == StatusValidated {
		rc.Summary = "呈上一份功法草案「" + p.Name + "」"
		rc.Outcome = WordRejected
		rc.Detail = "此功已固化在册，不可用草案覆盖；要改它，另起新名，重新走过试跑。"
		return rc
	}
	s.LoadProtocol(p)
	rc.Summary = "呈上一份功法草案「" + p.Name + "」"
	rc.Outcome = WordApplied
	rc.Detail = "已录入功法册（草创），可请起功试跑。"
	return rc
}

// draftErrLines 从编译错误提取定性错误行；非 CompileError（防御）回落
// 机读文本——正常路径 Compile 只返回 *CompileError。
func draftErrLines(err error) []string {
	var ce *CompileError
	if errors.As(err, &ce) && len(ce.Agent) > 0 {
		return ce.Agent
	}
	return []string{"草创未成，其故难明。"}
}

// settleProtocolStart 处理一份起功试跑意图（非动作意图）：按功法名起功，
// 前置条件仍由 StartProtocol 把关（stage_2 §35）；试跑的运行期与手工
// 起功完全同一条路径（Runtime/护卫/出界/移交门零特判）。
func (s *CultSession) settleProtocolStart(it Intent) IntentReceipt {
	name := strings.TrimSpace(it.Text)
	rc := IntentReceipt{Tick: s.World.Tick, Kind: it.Kind, Summary: "请起功「" + name + "」试跑"}
	if name == "" {
		rc.Outcome, rc.Detail = WordUnclear, "未指明要起功的功法名。"
		return rc
	}
	if _, ok := s.Protocols[name]; !ok {
		rc.Outcome, rc.Detail = WordRejected, "功法册里没有这份功法。"
		return rc
	}
	if err := s.StartProtocol(name); err != nil {
		rc.Outcome, rc.Detail = WordRejected, "起功未成：此刻身势不合其所求，或尚无静观为凭。"
		return rc
	}
	rc.Outcome = WordApplied
	rc.Detail = "已起功，行功由身体自循；有异象必报，试跑完毕另有总结回执。"
	return rc
}

// 试跑总结回执的定性措辞（零数字；周天账目数值只进调试视图）。
const WordTrialQualified = "周天行行稳固、归返充足，亦无未知异象——已固化在册，可为日后复用之基。"

// trialConclusion 在试跑自然行满（跑满周天数）时给出总结回执，并唤醒
// 分身定夺固化或修订（stage_2 §34 行为生命周期的 M2 延伸：试跑完毕
// 交还分身）。达标转正与固化由 finishTrial 依世界法则裁决，回执只是
// 定性转达。返回总结回执，由 Tick 并入本 Tick 回执组。
func (s *CultSession) trialConclusion() IntentReceipt {
	if s.Trial == nil {
		return IntentReceipt{}
	}
	sum := s.Trial.Finish()
	name := s.Trial.Record.Protocol
	rc := IntentReceipt{Tick: s.World.Tick, Kind: IntentStartProtocol, Summary: "「" + name + "」试跑已毕"}
	if sum.Qualified {
		rc.Outcome = WordApplied
		rc.Detail = WordTrialQualified
	} else {
		rc.Outcome = WordUnproven
		rc.Detail = strings.Join(trialFailReasons(s.Trial.Record), "；") + "。可依此修订草案，再行走跑。"
	}
	s.lastEscalated = true // 唤醒分身：固化或修订，由分身定夺
	return rc
}

// trialFailReasons 归纳逐周天失败原因（去重；定性词，零数字）。
func trialFailReasons(rec TrialRecord) []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range rec.Cycles {
		if c.Success || c.FailWhy == "" || seen[c.FailWhy] {
			continue
		}
		seen[c.FailWhy] = true
		out = append(out, c.FailWhy)
	}
	if len(out) == 0 {
		// 无失败周天却未达标：要么周天太少不足为凭，要么行中有过
		// 无法解释的异象（§29：有未知异常者不转正）。
		if rec.UnknownAnomaly {
			out = append(out, "行功中出现过无法解释的异象")
		} else {
			out = append(out, "试跑的周天太少，尚不足为凭")
		}
	}
	return out
}
