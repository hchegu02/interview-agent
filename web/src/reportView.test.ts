import { describe, expect, it } from "vitest";
import { drillPlanSummary, drillQuestionIds, retrievalTraceSummary } from "./reportView";
import type { DrillPlanItem, RetrievalTrace } from "./types";

describe("report drill view helpers", () => {
  const plan: DrillPlanItem[] = [
    { practice_order: 1, skill: "redis", reason: "缓存一致性薄弱", target_score: 80, recommended_question_ids: ["redis-001", "redis-001"] },
    { practice_order: 2, skill: "go", reason: "并发细节不足", target_score: 75, recommended_question_ids: ["go-003"] },
  ];

  it("summarizes the next drill plan for the report page", () => {
    expect(drillPlanSummary(plan)).toBe("2 个弱项 · 2 道题 · redis / go");
  });

  it("deduplicates recommended question ids in display order", () => {
    expect(drillQuestionIds(plan)).toEqual(["redis-001", "go-003"]);
  });

  it("summarizes retrieval trace for the report page", () => {
    const trace: RetrievalTrace = {
      query: "redis aof",
      fallback_reasons: ["rerank fallback"],
      stages: [
        { stage: "vector", count: 2, duration_ms: 1.2, items: [{ id: "redis-001", rank: 1, score: 0.8 }] },
        { stage: "rerank", count: 1, duration_ms: 2.3, items: [{ id: "redis-002", rank: 1, score: 0.9 }] },
      ],
      final: [{ id: "redis-002", rank: 1, score: 0.9 }],
    };

    expect(retrievalTraceSummary(trace)).toBe("2 个阶段 · 1 个最终候选 · 1 个降级信号");
  });
});
