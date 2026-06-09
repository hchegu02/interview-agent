import type { QuestionBankImportCommitSummary, QuestionBankImportItem, QuestionBankImportJob } from "./types";

export type ImportReviewFilter =
  | "all"
  | "pending"
  | "complete"
  | "missing_rubric"
  | "missing_expected_points"
  | "agent_rejected"
  | "invalid"
  | "accepted"
  | "rejected";

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

export function hasImportAnswerCompleteness(importItem: QuestionBankImportItem) {
  const item = importItem.item;
  return Boolean(
    nonEmptyText(item.content)
    && nonEmptyList(item.expected_points)
    && nonEmptyRecord(item.rubric)
    && nonEmptyText(item.sample_answer)
    && nonEmptyList(item.follow_up_hints),
  );
}

export function importItemReviewFlags(importItem: QuestionBankImportItem) {
  const valid = importItem.status === "valid";
  const accepted = valid && reviewStatus(importItem) === "accepted";
  const rejected = valid && reviewStatus(importItem) === "rejected";
  const missingRubric = valid && !nonEmptyRecord(importItem.item.rubric);
  const missingExpectedPoints = valid && !nonEmptyList(importItem.item.expected_points);
  const complete = valid && hasImportAnswerCompleteness(importItem);
  return {
    valid,
    invalid: importItem.status === "invalid",
    accepted,
    rejected,
    complete,
    incomplete: valid && !complete,
    missingRubric,
    missingExpectedPoints,
    agentRejected: importItem.agent_review_status === "rejected",
  };
}

export function buildImportReviewMetrics(
  job: QuestionBankImportJob | undefined,
  items: QuestionBankImportItem[],
  selectedIds: Set<string>,
) {
  const flags = items.map(importItemReviewFlags);
  const accepted = flags.filter((flag) => flag.accepted).length;
  return {
    total: job?.total_items ?? items.length,
    valid: job?.valid_items ?? flags.filter((flag) => flag.valid).length,
    invalid: job?.invalid_items ?? flags.filter((flag) => flag.invalid).length,
    accepted,
    rejected: flags.filter((flag) => flag.rejected).length,
    complete: flags.filter((flag) => flag.complete).length,
    incomplete: flags.filter((flag) => flag.incomplete).length,
    missingRubric: flags.filter((flag) => flag.missingRubric).length,
    missingExpectedPoints: flags.filter((flag) => flag.missingExpectedPoints).length,
    agentRejected: flags.filter((flag) => flag.agentRejected).length,
    selected: selectedIds.size,
    commitReady: accepted,
    imported: job?.imported_items ?? 0,
  };
}

export function filterImportItems(items: QuestionBankImportItem[], filter: ImportReviewFilter, query: string) {
  const normalizedQuery = query.trim().toLocaleLowerCase();
  return items.filter((item) => {
    const flags = importItemReviewFlags(item);
    const filterPass = filter === "all"
      || (filter === "pending" && flags.valid && flags.accepted && !flags.complete)
      || (filter === "complete" && flags.complete)
      || (filter === "missing_rubric" && flags.missingRubric)
      || (filter === "missing_expected_points" && flags.missingExpectedPoints)
      || (filter === "agent_rejected" && flags.agentRejected)
      || (filter === "invalid" && flags.invalid)
      || (filter === "accepted" && flags.accepted)
      || (filter === "rejected" && flags.rejected);
    return filterPass && matchesImportQuery(item, normalizedQuery);
  });
}

export function commitSummary(job: QuestionBankImportJob | undefined): QuestionBankImportCommitSummary | undefined {
  return job?.metadata?.commit_summary;
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

function matchesImportQuery(importItem: QuestionBankImportItem, query: string) {
  if (!query) return true;
  const item = importItem.item;
  const haystack = [
    importItem.id,
    importItem.question_id,
    item.id,
    item.content,
    item.skill_category,
    item.scenario,
    item.source,
    ...(item.tags || []),
    ...(item.role_tags || []),
  ].filter(Boolean).join(" ").toLocaleLowerCase();
  return haystack.includes(query);
}

function nonEmptyText(value: unknown) {
  return typeof value === "string" && value.trim().length > 0;
}

function nonEmptyList(value: unknown) {
  return Array.isArray(value) && value.some((item) => nonEmptyText(item));
}

function nonEmptyRecord(value: unknown) {
  return Boolean(value && typeof value === "object" && !Array.isArray(value) && Object.keys(value).length > 0);
}
