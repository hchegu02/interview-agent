import { describe, expect, it } from "vitest";
import { navItemsForWorkspace } from "./routes";

describe("workspace navigation", () => {
  it("keeps candidate interview navigation separate from admin navigation", () => {
    expect(navItemsForWorkspace("candidate").map((item) => item.label)).toEqual(["简历", "JD 分析", "面试", "报告"]);
    expect(navItemsForWorkspace("admin").map((item) => item.label)).toEqual(["题库"]);
  });
});
