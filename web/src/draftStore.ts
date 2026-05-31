import type { Draft, ProfileAnalyzeResponse, QuestionBankFilter, ResumeSections } from "./types";

export const DRAFT_KEY = "interview_agent_draft_v1";

const emptyDraft: Draft = {
  resume_text: "",
  jd_text: "",
  updated_at: "",
};

export function loadDraft(): Draft {
  try {
    const raw = window.localStorage.getItem(DRAFT_KEY);
    if (!raw) return { ...emptyDraft };
    const parsed = JSON.parse(raw) as Partial<Draft>;
    const resumeSections = normalizeResumeSections(parsed.resume_sections, parsed.resume_text || "");
    return {
      resume_text: parsed.resume_text || resumeTextFromSections(resumeSections),
      resume_sections: resumeSections,
      jd_text: parsed.jd_text || "",
      question_bank_filter: normalizeQuestionBankFilter(parsed.question_bank_filter),
      analysis: parsed.analysis,
      updated_at: parsed.updated_at || "",
    };
  } catch {
    return { ...emptyDraft };
  }
}

export function saveDraft(patch: Partial<Draft>): Draft {
  const next = buildDraft(loadDraft(), patch);
  window.localStorage.setItem(DRAFT_KEY, JSON.stringify(next));
  return next;
}

export function buildDraft(current: Draft, patch: Partial<Draft>, now = new Date().toISOString()): Draft {
  const resumeSections = normalizeResumeSections(
    patch.resume_sections ?? current.resume_sections,
    patch.resume_text ?? current.resume_text,
  );
  const resumeText = patch.resume_sections
    ? resumeTextFromSections(resumeSections)
    : patch.resume_text ?? current.resume_text ?? resumeTextFromSections(resumeSections);
  return {
    ...current,
    ...patch,
    resume_text: resumeText,
    resume_sections: resumeSections,
    question_bank_filter: normalizeQuestionBankFilter(patch.question_bank_filter ?? current.question_bank_filter),
    updated_at: now,
  };
}

export function clearDraft(): Draft {
  window.localStorage.removeItem(DRAFT_KEY);
  return { ...emptyDraft };
}

export function drillJDText(rawJD: string, plan: { skill?: string; reason?: string; recommended_question_ids?: string[] }[]): string {
  const focus = plan.map((item) => `${item.skill || "综合表达"}：${item.reason || "继续训练"}`);
  const ids = uniqueValues(plan.flatMap((item) => item.recommended_question_ids || []));
  const lines = [
    rawJD || "技术面试弱项专项训练。",
    "",
    "本轮专项训练重点：",
    ...focus.map((item) => `- ${item}`),
  ];
  if (ids.length) {
    lines.push("", `优先覆盖题库题：${ids.join("、")}`);
  }
  return lines.join("\n");
}

export function resumeTextFromSections(sections: ResumeSections): string {
  return [
    ["概况", sections.summary],
    ["技能", sections.skills],
    ["项目", sections.projects],
    ["亮点", sections.highlights],
    ["原文补充", sections.raw_notes],
  ]
    .filter(([, value]) => value.trim())
    .map(([label, value]) => `【${label}】\n${value.trim()}`)
    .join("\n\n");
}

export function normalizeResumeSections(sections?: Partial<ResumeSections>, fallbackText = ""): ResumeSections {
  if (sections) {
    return {
      summary: sections.summary || "",
      skills: sections.skills || "",
      projects: sections.projects || "",
      highlights: sections.highlights || "",
      raw_notes: sections.raw_notes || "",
    };
  }
  const text = fallbackText.trim();
  return {
    summary: firstNonEmptyLine(text),
    skills: linesMatching(text, ["技能", "技术", "stack", "redis", "go", "postgres", "kafka"]).join("\n"),
    projects: linesMatching(text, ["项目", "负责", "系统", "平台", "服务"]).join("\n"),
    highlights: linesMatching(text, ["优化", "提升", "降低", "增长", "%", "qps", "延迟"]).join("\n"),
    raw_notes: text,
  };
}

export function analysisSummary(analysis?: ProfileAnalyzeResponse): string {
  if (!analysis?.profile_analysis) return "尚未分析";
  return `${analysis.profile_analysis.match_score} 分 · ${analysis.profile_analysis.summary}`;
}

export function draftScopeSummary(filter?: QuestionBankFilter): string {
  const scope = normalizeQuestionBankFilter(filter);
  if (!scope) return "未限制题库范围";
  const parts: string[] = [];
  if (scope.skill_categories?.length) parts.push(`技能 ${scope.skill_categories.join(" / ")}`);
  if (scope.scenarios?.length) parts.push(`场景 ${scope.scenarios.join(" / ")}`);
  if (scope.difficulty_min && scope.difficulty_max) {
    parts.push(scope.difficulty_min === scope.difficulty_max ? `难度 ${scope.difficulty_min}` : `难度 ${scope.difficulty_min}-${scope.difficulty_max}`);
  } else if (scope.difficulty_min) {
    parts.push(`难度 >= ${scope.difficulty_min}`);
  } else if (scope.difficulty_max) {
    parts.push(`难度 <= ${scope.difficulty_max}`);
  }
  if (scope.tags?.length) parts.push(`标签 ${scope.tags.join(" / ")}`);
  return parts.join(" · ") || "未限制题库范围";
}

export function normalizeQuestionBankFilter(filter?: QuestionBankFilter): QuestionBankFilter | undefined {
  if (!filter) return undefined;
  const skill_categories = compact(filter.skill_categories);
  const scenarios = compact(filter.scenarios);
  const tags = compact(filter.tags);
  let difficulty_min = normalizeDifficulty(filter.difficulty_min);
  let difficulty_max = normalizeDifficulty(filter.difficulty_max);
  if (difficulty_min && difficulty_max && difficulty_min > difficulty_max) {
    [difficulty_min, difficulty_max] = [difficulty_max, difficulty_min];
  }
  const next: QuestionBankFilter = {};
  if (skill_categories.length) next.skill_categories = skill_categories;
  if (scenarios.length) next.scenarios = scenarios;
  if (difficulty_min) next.difficulty_min = difficulty_min;
  if (difficulty_max) next.difficulty_max = difficulty_max;
  if (tags.length) next.tags = tags;
  return Object.keys(next).length ? next : undefined;
}

function uniqueValues(items: string[]): string[] {
  return [...new Set(items.filter(Boolean))];
}

function firstNonEmptyLine(text: string): string {
  return text.split(/\r?\n/).map((line) => line.trim()).find(Boolean) || text;
}

function linesMatching(text: string, keywords: string[]): string[] {
  const lowerKeywords = keywords.map((item) => item.toLowerCase());
  return text.split(/\r?\n/)
    .map((line) => line.trim())
    .filter((line) => {
      const lower = line.toLowerCase();
      return lowerKeywords.some((keyword) => lower.includes(keyword));
    });
}

function compact(items?: string[]): string[] {
  return uniqueValues((items || []).map((item) => item.trim()).filter(Boolean));
}

function normalizeDifficulty(value?: number): number | undefined {
  const n = Number(value);
  if (!Number.isInteger(n) || n < 1 || n > 5) return undefined;
  return n;
}
