import { describe, expect, it } from "vitest";
import { drillPlanSummary, drillQuestionIds } from "./reportView";
import type { DrillPlanItem } from "./types";

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
});
