import type { InterviewQuestion, InterviewRound, Mode } from "./types";

export function shouldShowCurrent(question: InterviewQuestion | undefined, rounds: InterviewRound[]) {
  const last = rounds[rounds.length - 1];
  return question && (!last || last.question?.content !== question.content);
}

export function modeLabel(mode: Mode) {
  return mode === "practice" ? "模拟" : "考试";
}

export function formatTime(value: string) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "-";
  return date.toLocaleString();
}

export function formatYearsGap(value: number) {
  if (value > 0) return `+${value} 年`;
  if (value < 0) return `${value} 年`;
  return "0 年";
}

export function clamp(n: number, min: number, max: number) {
  return Math.max(min, Math.min(max, Number(n) || 0));
}
