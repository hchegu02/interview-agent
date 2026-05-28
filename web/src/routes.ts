export type Route = "/" | "/resume" | "/jd" | "/interview" | "/report" | "/questions";

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

export function questionURL(id: string): string {
  return `${routes.questions}?q=${encodeURIComponent(id)}`;
}
