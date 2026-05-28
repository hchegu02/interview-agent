import type { Draft, ProfileAnalyzeResponse } from "./types";

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
    return {
      resume_text: parsed.resume_text || "",
      jd_text: parsed.jd_text || "",
      analysis: parsed.analysis,
      updated_at: parsed.updated_at || "",
    };
  } catch {
    return { ...emptyDraft };
  }
}

export function saveDraft(patch: Partial<Draft>): Draft {
  const next: Draft = {
    ...loadDraft(),
    ...patch,
    updated_at: new Date().toISOString(),
  };
  window.localStorage.setItem(DRAFT_KEY, JSON.stringify(next));
  return next;
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

export function analysisSummary(analysis?: ProfileAnalyzeResponse): string {
  if (!analysis?.profile_analysis) return "尚未分析";
  return `${analysis.profile_analysis.match_score} 分 · ${analysis.profile_analysis.summary}`;
}

function uniqueValues(items: string[]): string[] {
  return [...new Set(items.filter(Boolean))];
}
