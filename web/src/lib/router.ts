import { useEffect, useState } from "react";

export type Route =
  | { page: "workspaces" }
  | { page: "workspace"; name: string; tab: string }
  | { page: "sessions" }
  | { page: "reviews" }
  | { page: "review"; sha: string }
  | { page: "ledger" }
  | { page: "policy" }
  | { page: "agents" };

export function parseRoute(hash: string): Route {
  const parts = hash.replace(/^#\/?/, "").split("/").filter(Boolean).map(decodeURIComponent);
  switch (parts[0]) {
    case "workspaces":
      return parts[1]
        ? { page: "workspace", name: parts[1], tab: parts[2] ?? "terminal" }
        : { page: "workspaces" };
    case "sessions":
      return { page: "sessions" };
    case "reviews":
      return parts[1] ? { page: "review", sha: parts[1] } : { page: "reviews" };
    case "ledger":
      return { page: "ledger" };
    case "policy":
      return { page: "policy" };
    case "agents":
      return { page: "agents" };
    default:
      return { page: "workspaces" };
  }
}

export function href(...parts: string[]): string {
  return "#/" + parts.map(encodeURIComponent).join("/");
}

export function useRoute(): Route {
  const [route, setRoute] = useState(() => parseRoute(location.hash));
  useEffect(() => {
    const onChange = () => setRoute(parseRoute(location.hash));
    window.addEventListener("hashchange", onChange);
    return () => window.removeEventListener("hashchange", onChange);
  }, []);
  return route;
}
