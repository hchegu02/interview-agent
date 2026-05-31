import { describe, expect, it } from "vitest";
import { clamp, formatTime, formatYearsGap, modeLabel, shouldShowCurrent } from "./interviewView";
import type { InterviewQuestion, InterviewRound } from "./types";

describe("interview view helpers", () => {
  it("formats labels and numeric display values", () => {
    expect(modeLabel("practice")).toBe("模拟");
    expect(modeLabel("exam")).toBe("考试");
    expect(formatYearsGap(2)).toBe("+2 年");
    expect(formatYearsGap(-1)).toBe("-1 年");
    expect(formatYearsGap(0)).toBe("0 年");
    expect(clamp(120, 0, 100)).toBe(100);
    expect(clamp(-10, 0, 100)).toBe(0);
    expect(clamp(64, 0, 100)).toBe(64);
  });

  it("formats invalid time as placeholder", () => {
    expect(formatTime("")).toBe("-");
    expect(formatTime("not-a-date")).toBe("-");
  });

  it("shows current question only when it differs from the last round", () => {
    const current: InterviewQuestion = { id: "q2", content: "Redis 热 key 怎么治理？" };
    const rounds: InterviewRound[] = [{
      round_id: "r1",
      number: 1,
      completed: true,
      question: { id: "q1", content: "Go channel 怎么避免泄漏？" },
    }];

    expect(shouldShowCurrent(current, rounds)).toBe(true);
    expect(shouldShowCurrent(rounds[0].question, rounds)).toBe(false);
    expect(shouldShowCurrent(undefined, rounds)).toBeUndefined();
  });
});
