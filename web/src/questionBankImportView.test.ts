import { describe, expect, it } from "vitest";
import { importDiffRows, importFieldProvenanceLabel, importSourceLabel, reviewStatus, reviewStatusLabel } from "./questionBankImportView";
import type { QuestionBankImportItem } from "./types";

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
});
