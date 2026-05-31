import type { DrillPlanItem } from "./types";

export function drillQuestionIds(plan: DrillPlanItem[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const item of plan) {
    for (const id of item.recommended_question_ids || []) {
      const value = id.trim();
      if (!value || seen.has(value)) continue;
      seen.add(value);
      out.push(value);
    }
  }
  return out;
}

export function drillPlanSummary(plan: DrillPlanItem[]): string {
  const skills = plan.map((item) => item.skill?.trim()).filter((skill): skill is string => Boolean(skill));
  const ids = drillQuestionIds(plan);
  return `${plan.length} 个弱项 · ${ids.length} 道题 · ${skills.slice(0, 3).join(" / ") || "综合表达"}`;
}
