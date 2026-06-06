import { describe, expect, it } from "vitest";
import { navItemsForWorkspace, questionURL, resolveNavigationState } from "./routes";

describe("workspace navigation", () => {
  it("keeps candidate interview navigation separate from admin navigation", () => {
    expect(navItemsForWorkspace("candidate").map((item) => item.label)).toEqual(["简历", "JD 分析", "面试", "报告", "Agent"]);
    expect(navItemsForWorkspace("admin").map((item) => item.label)).toEqual(["题库"]);
  });

  it("derives workspace and question jump from browser location parts", () => {
    expect(resolveNavigationState("/questions", "?q=redis%20hot%20key")).toEqual({
      route: "/questions",
      workspace: "admin",
      questionJump: "redis hot key",
    });
    expect(resolveNavigationState("/agent", "")).toEqual({
      route: "/agent",
      workspace: "candidate",
      questionJump: "",
    });
    expect(resolveNavigationState("/unknown", "?q=ignored")).toEqual({
      route: "/resume",
      workspace: "candidate",
      questionJump: "ignored",
    });
  });

  it("builds encoded question bank jump URLs", () => {
    expect(questionURL("redis hot/key")).toBe("/questions?q=redis%20hot%2Fkey");
  });
});
