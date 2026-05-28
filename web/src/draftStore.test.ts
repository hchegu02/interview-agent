import { describe, expect, it } from "vitest";
import { analysisSummary, drillJDText } from "./draftStore";

describe("draftStore helpers", () => {
  it("builds drill JD text from weak-area plan and question ids", () => {
    const got = drillJDText("原始 JD", [
      { skill: "redis", reason: "缓存一致性薄弱", recommended_question_ids: ["redis-001", "redis-001"] },
      { skill: "go", reason: "并发细节不足", recommended_question_ids: ["go-003"] },
    ]);

    expect(got).toContain("原始 JD");
    expect(got).toContain("本轮专项训练重点");
    expect(got).toContain("- redis：缓存一致性薄弱");
    expect(got).toContain("优先覆盖题库题：redis-001、go-003");
  });

  it("summarizes profile analysis for the JD page", () => {
    const got = analysisSummary({
      profile_analysis: {
        match_score: 72,
        summary: "匹配良好，需要验证项目深度。",
        years_gap: 0,
      },
    });

    expect(got).toBe("72 分 · 匹配良好，需要验证项目深度。");
  });
});
