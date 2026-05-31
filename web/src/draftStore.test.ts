import { describe, expect, it } from "vitest";
import { analysisSummary, buildDraft, draftScopeSummary, drillJDText, resumeTextFromSections } from "./draftStore";

describe("draftStore helpers", () => {
  it("builds drill JD text from weak-area plan and question ids", () => {
    const got = drillJDText("原始 JD", [
      { skill: "redis", reason: "缓存一致性薄弱", recommended_question_ids: ["redis-001", "redis-001"] },
      { skill: "go", reason: "并发细节不足", recommended_question_ids: ["go-003"] },
    ]);

    expect(got).toContain("原始 JD");
    expect(got).toContain("本轮专项训练重点");
    expect(got).toContain("缓存一致性薄弱");
    expect(got).toContain("redis-001");
    expect(got).toContain("go-003");
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

  it("summarizes question bank scope for the JD page", () => {
    const got = draftScopeSummary({
      skill_categories: ["redis", "go"],
      scenarios: ["troubleshooting"],
      difficulty_min: 2,
      difficulty_max: 4,
      tags: ["cache"],
    });

    expect(got).toBe("技能 redis / go · 场景 troubleshooting · 难度 2-4 · 标签 cache");
  });

  it("merges scope changes with the current in-memory draft", () => {
    const got = buildDraft({
      resume_text: "默认简历",
      jd_text: "默认 JD",
      updated_at: "old",
    }, {
      question_bank_filter: { skill_categories: ["redis"] },
    }, "2026-05-28T00:00:00.000Z");

    expect(got.resume_text).toBe("默认简历");
    expect(got.jd_text).toBe("默认 JD");
    expect(got.question_bank_filter?.skill_categories).toEqual(["redis"]);
    expect(got.updated_at).toBe("2026-05-28T00:00:00.000Z");
  });

  it("keeps resume text compatible while editing structured sections", () => {
    const got = buildDraft({
      resume_text: "三年 Go 后端经验，做过 Redis 和 PostgreSQL 优化。",
      jd_text: "默认 JD",
      updated_at: "old",
    }, {}, "2026-05-28T00:00:00.000Z");

    expect(got.resume_sections?.summary).toContain("三年 Go 后端经验");

    const text = resumeTextFromSections({
      summary: "三年 Go 后端经验",
      skills: "Go, Redis, PostgreSQL",
      projects: "秒杀系统：负责缓存和队列削峰",
      highlights: "接口耗时降低 40%",
      raw_notes: "原文补充",
    });

    expect(text).toContain("【概况】\n三年 Go 后端经验");
    expect(text).toContain("【技能】\nGo, Redis, PostgreSQL");
    expect(text).toContain("【项目】\n秒杀系统：负责缓存和队列削峰");
    expect(text).toContain("【亮点】\n接口耗时降低 40%");
  });
});
