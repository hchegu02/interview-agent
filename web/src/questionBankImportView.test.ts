import { describe, expect, it } from "vitest";
import {
  buildImportReviewMetrics,
  commitSummary,
  filterImportItems,
  hasImportAnswerCompleteness,
  importDiffRows,
  importFieldProvenanceLabel,
  importItemReviewFlags,
  importSourceLabel,
  reviewStatus,
  reviewStatusLabel,
} from "./questionBankImportView";
import type { QuestionBankImportItem, QuestionBankImportJob } from "./types";

describe("question bank import view helpers", () => {
  it("labels import source and review status", () => {
    expect(importSourceLabel("document")).toBe("文档生成");
    expect(importSourceLabel("question_bank")).toBe("本地题库");
    expect(reviewStatus({} as QuestionBankImportItem)).toBe("accepted");
    expect(reviewStatusLabel("rejected")).toBe("已拒绝");
    expect(reviewStatusLabel("accepted")).toBe("已接受");
    expect(importFieldProvenanceLabel("llm")).toBe("LLM");
  });

  it("builds diff rows from original item and field provenance", () => {
    const rows = importDiffRows({
      id: "row-1",
      question_id: "q1",
      status: "valid",
      review_status: "accepted",
      original_item: {
        id: "q1",
        content: "Redis 热 key 怎么治理？",
        skill_category: "redis",
        tags: ["cache"],
      },
      item: {
        id: "q1",
        content: "Redis 热 key 怎么治理？",
        skill_category: "redis",
        tags: ["cache", "hot-key"],
        expected_points: ["发现热 key", "隔离缓存"],
      },
      field_provenance: {
        tags: "merged",
        expected_points: "llm",
      },
    });

    expect(rows).toContainEqual({
      key: "tags",
      label: "标签",
      before: "cache",
      after: "cache / hot-key",
      source: "合并",
    });
    expect(rows).toContainEqual({
      key: "expected_points",
      label: "要点",
      before: "",
      after: "发现热 key / 隔离缓存",
      source: "LLM",
    });
  });

  it("detects answer completeness and review flags", () => {
    const complete = importItem("imp-1", {
      expected_points: ["定位热点", "隔离缓存"],
      rubric: { good: "能说明发现和治理闭环" },
      sample_answer: "先通过监控发现热点，再做本地缓存和限流。",
      follow_up_hints: ["怎么验证热点已经消退？"],
    });
    const incomplete = importItem("imp-2", {
      expected_points: [],
      rubric: {},
      sample_answer: "",
      follow_up_hints: [],
    });

    expect(hasImportAnswerCompleteness(complete)).toBe(true);
    expect(hasImportAnswerCompleteness(incomplete)).toBe(false);
    expect(importItemReviewFlags({
      ...complete,
      agent_review_status: "rejected",
      agent_review_reason: "rubric 太空泛",
    })).toMatchObject({
      valid: true,
      complete: true,
      agentRejected: true,
      accepted: true,
      rejected: false,
    });
  });

  it("filters import items by operational review states and query", () => {
    const items = [
      importItem("ready-1", { content: "Redis 热 key 怎么治理？", tags: ["redis"], skill_category: "Redis" }),
      importItem("missing-rubric", { content: "MySQL 索引失效场景？", skill_category: "MySQL", tags: ["mysql"], rubric: {} }),
      importItem("agent-bad", { content: "Go GMP 调度？", skill_category: "Go", tags: ["go"] }, { agent_review_status: "rejected" }),
      importItem("invalid-1", { content: "空题", skill_category: "Unknown", tags: ["invalid"] }, { status: "invalid", review_status: "rejected" }),
    ];

    expect(filterImportItems(items, "complete", "")).toHaveLength(2);
    expect(filterImportItems(items, "missing_rubric", "").map((item) => item.id)).toEqual(["missing-rubric"]);
    expect(filterImportItems(items, "agent_rejected", "").map((item) => item.id)).toEqual(["agent-bad"]);
    expect(filterImportItems(items, "invalid", "").map((item) => item.id)).toEqual(["invalid-1"]);
    expect(filterImportItems(items, "all", "redis").map((item) => item.id)).toEqual(["ready-1"]);
  });

  it("builds review metrics and exposes commit summary", () => {
    const job: QuestionBankImportJob = {
      id: "imp-1",
      source_type: "document",
      filename: "Redis.md",
      status: "ready",
      total_chunks: 3,
      total_items: 4,
      valid_items: 3,
      invalid_items: 1,
      imported_items: 0,
      created_at: "2026-06-09T00:00:00Z",
      updated_at: "2026-06-09T00:00:00Z",
      metadata: {
        commit_summary: {
          imported: 2,
          skipped: 1,
          rejected: 1,
          embedded: 2,
          embedding_failed: 0,
        },
      },
    };
    const items = [
      importItem("accepted-complete"),
      importItem("accepted-incomplete", { rubric: {} }),
      importItem("rejected", {}, { review_status: "rejected" }),
      importItem("invalid", {}, { status: "invalid", review_status: "rejected" }),
    ];

    expect(buildImportReviewMetrics(job, items, new Set(["accepted-complete", "rejected"]))).toMatchObject({
      total: 4,
      valid: 3,
      invalid: 1,
      accepted: 2,
      rejected: 1,
      complete: 2,
      incomplete: 1,
      selected: 2,
      commitReady: 2,
    });
    expect(commitSummary(job)).toEqual({
      imported: 2,
      skipped: 1,
      rejected: 1,
      embedded: 2,
      embedding_failed: 0,
    });
  });
});

function importItem(
  id: string,
  item: Partial<QuestionBankImportItem["item"]> = {},
  overrides: Partial<QuestionBankImportItem> = {},
): QuestionBankImportItem {
  return {
    id,
    question_id: `q-${id}`,
    status: "valid",
    review_status: "accepted",
    item: {
      id: `q-${id}`,
      content: item.content ?? "Redis 热 key 怎么治理？",
      skill_category: item.skill_category ?? "Redis",
      difficulty: item.difficulty ?? 3,
      tags: item.tags ?? ["redis", "cache"],
      expected_points: item.expected_points ?? ["发现热点", "治理热点"],
      rubric: item.rubric ?? { pass: "覆盖发现、隔离、验证" },
      sample_answer: item.sample_answer ?? "先定位热 key，再通过缓存隔离、限流和监控闭环治理。",
      follow_up_hints: item.follow_up_hints ?? ["如何验证治理效果？"],
      ...item,
    },
    ...overrides,
  };
}
