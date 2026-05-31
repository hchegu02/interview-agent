import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { DetailList, EmptyPage, PageHeader, Rubric, Select } from "./sharedView";

describe("shared view components", () => {
  it("renders page headers with optional copy", () => {
    const html = renderToStaticMarkup(<PageHeader eyebrow="Step 1" title="候选人资料" copy="补充说明" />);

    expect(html).toContain("Step 1");
    expect(html).toContain("<h1>候选人资料</h1>");
    expect(html).toContain("补充说明");
  });

  it("renders select options sorted with numeric awareness", () => {
    const html = renderToStaticMarkup(
      <Select value="" onChange={() => undefined} label="全部难度" values={{ "10": 1, "2": 3 }} format={(v) => `难度 ${v}`} />,
    );

    expect(html.indexOf("难度 2 (3)")).toBeLessThan(html.indexOf("难度 10 (1)"));
  });

  it("omits empty detail blocks", () => {
    expect(renderToStaticMarkup(<DetailList title="评分要点" items={[]} />)).toBe("");
    expect(renderToStaticMarkup(<Rubric rubric={{}} />)).toBe("");
  });

  it("renders empty page action", () => {
    const html = renderToStaticMarkup(<EmptyPage title="还没有进行中的面试" action="回到 JD 分析" onAction={() => undefined} />);

    expect(html).toContain("还没有进行中的面试");
    expect(html).toContain("回到 JD 分析");
  });
});
