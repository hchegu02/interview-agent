import type { QuestionBankImportItem } from "./types";

export function reviewStatus(item: QuestionBankImportItem) {
  return item.review_status || "accepted";
}

export function reviewStatusLabel(status: string) {
  return status === "rejected" ? "已拒绝" : "已接受";
}

export function importSourceLabel(source: string) {
  return source === "document" ? "文档生成" : "本地题库";
}

export function importFieldProvenanceLabel(source: string) {
  const labels: Record<string, string> = {
    uploaded: "上传",
    merged: "合并",
    llm: "LLM",
    default: "默认",
    generated: "生成",
  };
  return labels[source] || source;
}

export function importDiffRows(importItem: QuestionBankImportItem) {
  const original = importItem.original_item;
  const provenance = importItem.field_provenance;
  if (!original && !provenance) return [];
  const fields = [
    ["skill_category", "技能"],
    ["difficulty", "难度"],
    ["tags", "标签"],
    ["expected_points", "要点"],
    ["rubric", "Rubric"],
    ["sample_answer", "参考答案"],
    ["follow_up_hints", "追问"],
  ] as const;
  return fields.map(([key, label]) => {
    const before = original ? formatImportField(original[key]) : "";
    const after = formatImportField(importItem.item[key]);
    const provenanceSource = provenance?.[key];
    return {
      key,
      label,
      before,
      after,
      source: provenanceSource ? importFieldProvenanceLabel(provenanceSource) : original ? inferImportDiffSource(before, after) : "",
    };
  }).filter((row) => (row.before || row.after) && (original || row.source));
}

function inferImportDiffSource(before: string, after: string) {
  return before === after ? "上传" : before ? "合并" : "LLM";
}

function formatImportField(value: unknown): string {
  if (value == null || value === "") return "";
  if (Array.isArray(value)) return value.join(" / ");
  if (typeof value === "object") {
    return Object.entries(value as Record<string, string>)
      .map(([key, val]) => `${key}: ${val}`)
      .join(" / ");
  }
  return String(value);
}
