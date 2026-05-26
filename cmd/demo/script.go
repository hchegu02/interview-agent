// cmd/demo 的 YAML 脚本 schema。
//
// 一个 Script 文件描述一次完整的面试：JD、简历，以及候选人按出题顺序
// 提供的回答列表（包含主题题和追问）。demo CLI 按顺序"喂"这些回答到
// graph，直到 Session 完成或 answers 耗尽。
//
// 设计取舍：
//   - 不把 answers 按题分组（list of {question_hint, answer}）：实际跑下来
//     pick_next 出哪道题受 LLM 决策影响，hint 容易和真实出题对不上；
//     按顺序消费简单 + 可预测，跑完看 run.json 就知道每条 answer 对了哪题。
//   - 没有 max_rounds / max_probes 字段：这两个是 Agent 内置预算，
//     demo 不应该覆盖——脚本的"长度"由 len(answers) 决定。
package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Script 是 demo CLI 读取的 YAML 文件结构。
type Script struct {
	JobProfile ScriptJobProfile `yaml:"job_profile"`
	Candidate  ScriptCandidate  `yaml:"candidate"`
	// Answers 按候选人在面试中作答的时间顺序排列。
	// graph 每次暂停（pick_next / probe_ask）后 CLI 弹出第一个填到 Session。
	Answers []string `yaml:"answers"`
}

// ScriptJobProfile 是 JD 原文（不是 LLM 抽取后的 JobProfile）。
// demo 跑 parse_jd 节点来抽取，这样链路与生产一致。
type ScriptJobProfile struct {
	JDText string `yaml:"jd_text"`
}

// ScriptCandidate 是简历原文。
type ScriptCandidate struct {
	ResumeText string `yaml:"resume_text"`
}

// LoadScript 读取并解析 YAML 文件。
// 校验：JD / Resume 必须非空；answers 至少 1 条（否则 graph 一暂停就跑不动了）。
func LoadScript(path string) (*Script, error) {
	if path == "" {
		return nil, fmt.Errorf("script path is required")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read script %s: %w", path, err)
	}
	var s Script
	if err := yaml.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("parse script %s: %w", path, err)
	}
	if s.JobProfile.JDText == "" {
		return nil, fmt.Errorf("script %s: job_profile.jd_text is empty", path)
	}
	if s.Candidate.ResumeText == "" {
		return nil, fmt.Errorf("script %s: candidate.resume_text is empty", path)
	}
	if len(s.Answers) == 0 {
		return nil, fmt.Errorf("script %s: answers is empty", path)
	}
	return &s, nil
}
