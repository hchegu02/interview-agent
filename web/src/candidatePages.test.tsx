import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { AgentToolTracePanel, InterviewPage, JDPage, ReportPage, UserMemoryPanel } from "./candidatePages";
import type { Session, UserMemory } from "./types";

function markerIndex(html: string, marker: string): number {
  const index = html.indexOf(marker);
  expect(index).toBeGreaterThanOrEqual(0);
  return index;
}

function sliceFrom(html: string, marker: string): string {
  return html.slice(markerIndex(html, marker));
}

describe("user memory panel", () => {
  it("renders long-term user memory evidence", () => {
    const memory: UserMemory = {
      strengths: ["Go 并发", "项目复盘"],
      weaknesses: [
        { topic: "Redis", evidence: "缓存击穿回答缺少互斥锁方案", severity: 3, updated_at: "2026-06-01T12:30:00Z" },
        { topic: "系统设计", severity: 2 },
      ],
      skill_scores: { Go: 86, Redis: 61 },
      last_advice: ["下一轮先练 Redis 高并发缓存场景。"],
      updated_at: "2026-06-01T12:30:00Z",
    };

    const html = renderToStaticMarkup(<UserMemoryPanel memory={memory} />);

    expect(html).toContain("长期用户画像");
    expect(html).toContain("Go 并发");
    expect(html).toContain("Redis");
    expect(html).toContain("缓存击穿回答缺少互斥锁方案");
    expect(html).toContain("Go");
    expect(html).toContain("86");
    expect(html).toContain("下一轮先练 Redis 高并发缓存场景。");
    expect(html).toContain("2026-06-01T12:30:00Z");
  });

  it("renders a readable empty state when memory is missing", () => {
    const html = renderToStaticMarkup(<UserMemoryPanel memory={null} />);

    expect(html).toContain("长期用户画像");
    expect(html).toContain("暂无长期画像数据");
    expect(html).toContain("完成更多面试后再展示稳定弱项和建议");
  });
});

describe("candidate jd page", () => {
  const draft = {
    resume_text: "Go 后端候选人",
    jd_text: "Go 后端岗位",
    updated_at: "2026-01-01T00:00:00Z",
  };

  it("shows question bank scope controls in practice mode", () => {
    const html = renderToStaticMarkup(
      <JDPage draft={draft} mode="practice" busy={false} updateDraft={() => undefined} setMode={() => undefined} analyze={() => Promise.resolve()} startInterview={() => Promise.resolve()} />,
    );

    expect(html).toContain("题库范围");
    expect(html).toContain("全部技能");
  });

  it("hides question bank scope controls in exam mode", () => {
    const html = renderToStaticMarkup(
      <JDPage draft={draft} mode="exam" busy={false} updateDraft={() => undefined} setMode={() => undefined} analyze={() => Promise.resolve()} startInterview={() => Promise.resolve()} />,
    );

    expect(html).toContain("岗位 JD");
    expect(html).not.toContain("题库范围");
    expect(html).not.toContain("全部技能");
  });

  it("shows interview mode controls in the preparation workflow", () => {
    const html = renderToStaticMarkup(
      <JDPage
        draft={draft}
        mode="practice"
        busy={false}
        updateDraft={() => undefined}
        setMode={() => undefined}
        analyze={() => Promise.resolve()}
        startInterview={() => Promise.resolve()}
      />,
    );

    expect(html).toContain("面试模式");
    expect(html).toContain("模拟");
    expect(html).toContain("考试");
    expect(html).toContain("开始面试");
    expect(markerIndex(html, "准备检查")).toBeLessThan(markerIndex(html, "面试模式"));
    expect(markerIndex(html, "面试模式")).toBeLessThan(markerIndex(html, "开始面试"));
  });
});

describe("candidate report page", () => {
  it("renders report-owned round reviews with original answers and follow-ups", () => {
    const session: Session = {
      session_id: "s-round-review",
      mode: "practice",
      status: "completed",
      phase: "completed",
      report: {
        session_id: "s-round-review",
        overall_score: 82,
        skill_breakdown: { Go: 82 },
        round_reviews: [{
          round_id: "r1",
          number: 1,
          type: "main",
          question_id: "go-001",
          question: "讲一下 Go 的 GMP 调度模型。",
          answer: "原始回答：G/M/P 和 work stealing。",
          score: 82,
          hit_points: ["覆盖 G/M/P"],
          missed_points: ["缺少排障案例"],
          suggestion: "补充线上调度延迟案例",
          expected_points: ["G/M/P 定义", "本地队列"],
          counts_toward_overall: true,
          follow_ups: [{
            question: "work stealing 什么时候发生？",
            answer: "追问原始回答：本地队列为空时。",
            score: 80,
            hit_points: ["回答了触发条件"],
            missed_points: ["缺少边界条件"],
            suggestion: "补充全局队列场景",
          }],
        }],
        highlights: [],
        improvements: [],
        next_steps: [],
      },
      rounds: [{
        round_id: "r1",
        number: 1,
        question: { id: "go-001", content: "旧 rounds 问题" },
        answer: "旧 rounds 回答不应作为优先来源",
        completed: true,
      }],
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
    };

    const html = renderToStaticMarkup(<ReportPage session={session} startDrill={() => undefined} jumpQuestion={() => undefined} />);

    expect(html).toContain("逐题评分");
    expect(html).toContain("讲一下 Go 的 GMP 调度模型。");
    expect(html).toContain("原始回答：G/M/P 和 work stealing。");
    expect(html).toContain("82 分");
    expect(html).toContain("覆盖 G/M/P");
    expect(html).toContain("缺少排障案例");
    expect(html).toContain("补充线上调度延迟案例");
    expect(html).toContain("work stealing 什么时候发生？");
    expect(html).toContain("追问原始回答：本地队列为空时。");
    expect(html).not.toContain("旧 rounds 回答不应作为优先来源");
  });

  it("falls back to session rounds when report round reviews are missing", () => {
    const session: Session = {
      session_id: "s-round-fallback",
      mode: "practice",
      status: "completed",
      phase: "completed",
      report: { session_id: "s-round-fallback", overall_score: 70, skill_breakdown: {}, highlights: [], improvements: [], next_steps: [] },
      rounds: [{
        round_id: "r1",
        number: 1,
        question: { id: "redis-001", content: "Redis AOF 和 RDB 怎么取舍？" },
        answer: "AOF 更偏实时恢复，RDB 更适合快照备份。",
        feedback: { score: 70, hit_points: ["区分恢复和快照"], missed_points: ["缺少 fsync 策略"], suggestion: "补充 appendfsync 策略" },
        follow_ups: [{
          question: "AOF everysec 最坏丢多久数据？",
          answer: "最多大约 1 秒。",
          feedback: { score: 80, hit_points: ["回答了时间窗口"], missed_points: ["缺少 fsync 机制"], suggestion: "补充后台刷盘细节" },
        }],
        completed: true,
      }],
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
    };

    const html = renderToStaticMarkup(<ReportPage session={session} startDrill={() => undefined} jumpQuestion={() => undefined} />);

    expect(html).toContain("Redis AOF 和 RDB 怎么取舍？");
    expect(html).toContain("AOF 更偏实时恢复，RDB 更适合快照备份。");
    expect(html).toContain("70 分");
    expect(html).toContain("AOF everysec 最坏丢多久数据？");
    expect(html).toContain("最多大约 1 秒。");
    expect(html).toContain("80 分");
    expect(html).toContain("回答了时间窗口");
  });

  it("hides internal diagnostics on exam reports", () => {
    const session: Session = {
      session_id: "s-exam-report",
      mode: "exam",
      status: "completed",
      phase: "completed",
      report: {
        session_id: "s-exam-report",
        overall_score: 82,
        skill_breakdown: {},
        round_reviews: [{ round_id: "r1", number: 1, question: "Go 问题", answer: "Go 回答", score: 82 }],
        highlights: [],
        improvements: [],
        next_steps: [],
      },
      working_memory: { weak_skills: ["redis"], difficulty: { current: 3 } },
      retrieval_trace: { query: "internal query", final: [{ id: "q-1", rank: 1, score: 0.9 }] },
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
    };

    const html = renderToStaticMarkup(<ReportPage session={session} startDrill={() => undefined} jumpQuestion={() => undefined} />);

    expect(html).toContain("逐题评分");
    expect(html).toContain("Go 回答");
    expect(html).not.toContain("Agent 状态");
    expect(html).not.toContain("检索链路");
    expect(html).not.toContain("internal query");
  });

  it("renders retrieval trace evidence on the report page", () => {
    const session: Session = {
      session_id: "s-1",
      mode: "practice",
      status: "completed",
      phase: "completed",
      report: { session_id: "s-1", overall_score: 82, skill_breakdown: {}, highlights: [], improvements: [], next_steps: [] },
      retrieval_trace: {
        query: "go redis interview",
        stages: [{ stage: "bm25", count: 2, duration_ms: 11 }],
        final: [{ id: "q-redis", rank: 1, score: 0.87, stage: "rrf", sources: { bm25: 1, vector: 2 } }],
        fallback_reasons: ["rerank_timeout"],
      },
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
    };

    const html = renderToStaticMarkup(<ReportPage session={session} startDrill={() => undefined} jumpQuestion={() => undefined} />);

    expect(html).toContain("检索链路");
    expect(html).toContain("go redis interview");
    expect(html).toContain("q-redis");
    expect(html).toContain("rerank_timeout");
  });

  it("renders working memory state on the report page", () => {
    const session: Session = {
      session_id: "s-memory",
      mode: "practice",
      status: "completed",
      phase: "completed",
      report: { session_id: "s-memory", overall_score: 82, skill_breakdown: {}, highlights: [], improvements: [], next_steps: [] },
      working_memory: {
        weak_skills: ["redis"],
        confirmed_skills: ["go"],
        skill_coverage: { go: 1.5, redis: 0.4 },
        difficulty: { current: 3, correct_streak: 2, wrong_streak: 0 },
        avg_score: 82,
        rounds_asked: 3,
        max_rounds: 8,
        scored_rounds: 2,
        degraded_reasons: { rag: "fallback used" },
        probes_used: 1,
        max_probes: 4,
        reflections_used: 0,
        max_reflections: 1,
      },
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
    };

    const html = renderToStaticMarkup(<ReportPage session={session} startDrill={() => undefined} jumpQuestion={() => undefined} />);

    expect(html).toContain("Agent 状态");
    expect(html).toContain("深入难度");
    expect(html).toContain("redis");
    expect(html).toContain("fallback used");
  });

  it("renders training plan before retrieval diagnostics on the report page", () => {
    const session: Session = {
      session_id: "s-report-layout",
      mode: "practice",
      status: "completed",
      phase: "completed",
      report: {
        session_id: "s-report-layout",
        overall_score: 78,
        skill_breakdown: { Go: 78 },
        highlights: ["表达清楚"],
        improvements: ["补充线上排障案例"],
        next_steps: [],
        drill_plan: [{
          practice_order: 1,
          skill: "Go 并发",
          reason: "GMP 调度细节薄弱",
          target_score: 85,
          recommended_question_ids: ["go-001"],
        }],
      },
      working_memory: {
        weak_skills: ["Go 并发"],
        difficulty: { current: 2 },
        rounds_asked: 2,
        max_rounds: 8,
      },
      retrieval_trace: {
        query: "go scheduler",
        final: [{ id: "go-001", rank: 1, score: 0.91 }],
      },
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
    };

    const html = renderToStaticMarkup(<ReportPage session={session} startDrill={() => undefined} jumpQuestion={() => undefined} />);

    expect(html).toContain("训练计划");
    expect(html).toContain("检索链路");
    expect(html).toContain("辅助诊断");
    const auxIndex = markerIndex(html, "辅助诊断");
    expect(markerIndex(html, "训练计划")).toBeLessThan(auxIndex);
    expect(markerIndex(html, "Agent 状态")).toBeGreaterThan(auxIndex);
    expect(markerIndex(html, "检索链路")).toBeGreaterThan(auxIndex);
    const diagnostics = sliceFrom(html, "辅助诊断");
    expect(diagnostics).toContain("Agent 状态");
    expect(diagnostics).toContain("检索链路");
  });
});

describe("candidate interview page", () => {
  it("renders working memory state during an interview", () => {
    const session: Session = {
      session_id: "s-interview-memory",
      mode: "practice",
      status: "running",
      phase: "answering",
      working_memory: {
        difficulty: { current: 1 },
        rounds_asked: 1,
        max_rounds: 8,
        probes_used: 0,
        max_probes: 4,
        reflections_used: 0,
        max_reflections: 1,
      },
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
    };

    const html = renderToStaticMarkup(
      <InterviewPage session={session} events={[]} busy={false} pendingAnswer="" submitAnswer={() => Promise.resolve()} goJD={() => undefined} />,
    );

    expect(html).toContain("Agent 状态");
    expect(html).toContain("基础难度");
    expect(html).toContain("暂无降级记录");
  });

  it("hides internal diagnostics during an exam interview", () => {
    const session: Session = {
      session_id: "s-exam-interview",
      mode: "exam",
      status: "running",
      phase: "answering",
      question: { id: "go-001", content: "讲一下 Go 的 GMP 调度模型。" },
      working_memory: {
        difficulty: { current: 3 },
        rounds_asked: 1,
        max_rounds: 8,
      },
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
    };

    const html = renderToStaticMarkup(
      <InterviewPage
        session={session}
        events={[{ type: "graph.node", label: "Graph 节点", detail: "score_answer", at: "2026-01-01T00:00:00Z" }]}
        busy={false}
        pendingAnswer=""
        submitAnswer={() => Promise.resolve()}
        goJD={() => undefined}
      />,
    );

    expect(html).toContain("正式考试");
    expect(html).toContain("讲一下 Go 的 GMP 调度模型。");
    expect(html).not.toContain("Agent 状态");
    expect(html).not.toContain("Graph 节点");
    expect(html).not.toContain("score_answer");
  });

  it("keeps practice diagnostics available as auxiliary interview information", () => {
    const session: Session = {
      session_id: "s-interview-aux",
      mode: "practice",
      status: "running",
      phase: "answering",
      question: { id: "go-001", content: "讲一下 Go GMP。" },
      working_memory: {
        difficulty: { current: 1 },
        rounds_asked: 1,
        max_rounds: 8,
      },
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
    };

    const html = renderToStaticMarkup(
      <InterviewPage
        session={session}
        events={[{ type: "graph.node", label: "Graph 节点", detail: "score_answer", at: "12:00:00" }]}
        busy={false}
        pendingAnswer=""
        submitAnswer={() => Promise.resolve()}
        goJD={() => undefined}
      />,
    );

    expect(html).toContain("讲一下 Go GMP。");
    expect(html).toContain("辅助诊断");
    const auxIndex = markerIndex(html, "辅助诊断");
    expect(markerIndex(html, "Agent 状态")).toBeGreaterThan(auxIndex);
    expect(markerIndex(html, "Graph 节点")).toBeGreaterThan(auxIndex);
    const diagnostics = sliceFrom(html, "辅助诊断");
    expect(diagnostics).toContain("Agent 状态");
    expect(diagnostics).toContain("Graph 节点");
  });
});

describe("agent tool trace panel", () => {
  it("renders read-only tool trace details", () => {
    const html = renderToStaticMarkup(<AgentToolTracePanel traces={[{
      name: "github.project_analyze",
      permission: "read_only",
      status: "success",
      elapsed_ms: 12,
      summary: "repo metadata loaded",
    }, {
      name: "github.project_analyze",
      permission: "read_only",
      status: "failed",
      error_class: "invalid_github_url",
      elapsed_ms: 2,
      summary: "generic advice returned",
    }]} />);

    expect(html).toContain("工具调用 Trace");
    expect(html).toContain("2 次调用");
    expect(html).toContain("github.project_analyze");
    expect(html).toContain("read_only");
    expect(html).toContain("success");
    expect(html).toContain("failed");
    expect(html).toContain("invalid_github_url");
    expect(html).toContain("12ms");
    expect(html).toContain("repo metadata loaded");
  });

  it("omits the panel when trace is missing", () => {
    const html = renderToStaticMarkup(<AgentToolTracePanel />);

    expect(html).not.toContain("工具调用 Trace");
  });
});
