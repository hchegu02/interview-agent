import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { InterviewPage, ReportPage, UserMemoryPanel } from "./candidatePages";
import type { Session, UserMemory } from "./types";

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

describe("candidate report page", () => {
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
});
