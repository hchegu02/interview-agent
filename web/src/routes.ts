export type Route = "/" | "/resume" | "/jd" | "/interview" | "/report" | "/questions";
export type Workspace = "candidate" | "admin";
export type NavItem = { label: string; route: Route };

export const routes = {
  home: "/" as Route,
  resume: "/resume" as Route,
  jd: "/jd" as Route,
  interview: "/interview" as Route,
  report: "/report" as Route,
  questions: "/questions" as Route,
};

export function normalizeRoute(pathname: string): Route {
  switch (pathname) {
    case "/":
    case "/resume":
    case "/jd":
    case "/interview":
    case "/report":
    case "/questions":
      return pathname;
    default:
      return "/resume";
  }
}

export function navItemsForWorkspace(workspace: Workspace): NavItem[] {
  if (workspace === "admin") {
    return [
      { label: "题库", route: routes.questions },
    ];
  }
  return [
    { label: "简历", route: routes.resume },
    { label: "JD 分析", route: routes.jd },
    { label: "面试", route: routes.interview },
    { label: "报告", route: routes.report },
  ];
}

export function workspaceForRoute(route: Route): Workspace {
  return route === routes.questions ? "admin" : "candidate";
}

export function defaultRouteForWorkspace(workspace: Workspace): Route {
  return workspace === "admin" ? routes.questions : routes.resume;
}

export type NavigationState = {
  route: Route;
  workspace: Workspace;
  questionJump: string;
};

export function resolveNavigationState(pathname: string, search: string): NavigationState {
  const route = normalizeRoute(pathname);
  return {
    route,
    workspace: workspaceForRoute(route),
    questionJump: new URLSearchParams(search).get("q") || "",
  };
}

export function questionURL(id: string): string {
  return `${routes.questions}?q=${encodeURIComponent(id)}`;
}
