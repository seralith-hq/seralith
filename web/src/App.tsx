import { useCallback, useState } from "react";
import { useEvents, useResource } from "./lib/hooks";
import { href, useRoute, type Route } from "./lib/router";
import type { ProjectInfo, SessionInfo } from "./lib/types";
import { Logo, Wordmark } from "./components/ui";
import { WorkspacesPage } from "./pages/Workspaces";
import { WorkspacePage } from "./pages/Workspace";
import { SessionsPage } from "./pages/Sessions";
import { ReviewPage, ReviewsPage } from "./pages/Reviews";
import { LedgerPage } from "./pages/Ledger";
import { PolicyPage } from "./pages/Policy";
import { AgentsPage } from "./pages/Agents";

const nav: { page: Route["page"]; label: string; match: Route["page"][] }[] = [
  { page: "workspaces", label: "Workspaces", match: ["workspaces", "workspace"] },
  { page: "sessions", label: "Live sessions", match: ["sessions"] },
  { page: "reviews", label: "Reviews", match: ["reviews", "review"] },
  { page: "ledger", label: "Audit ledger", match: ["ledger"] },
  { page: "policy", label: "Policy", match: ["policy"] },
  { page: "agents", label: "Coding tools", match: ["agents"] },
];

export function App() {
  const route = useRoute();
  const [version, setVersion] = useState(0);
  const bump = useCallback(() => setVersion((v) => v + 1), []);
  useEvents(bump);
  const project = useResource<ProjectInfo>("/project");
  const sessions = useResource<SessionInfo[]>("/sessions", [version]);
  const running = sessions.data?.filter((s) => s.state === "running").length ?? 0;

  if (project.error?.includes("token")) {
    return (
      <div className="empty" style={{ paddingTop: "20vh" }}>
        <Logo size={56} />
        <h3 style={{ marginTop: 16 }}>Open the workbench from your terminal</h3>
        <p>
          Run <code>sigil serve</code> and use the link it prints. The link carries a one-time token for this
          session.
        </p>
      </div>
    );
  }

  return (
    <div className="app">
      <nav className="sidebar" aria-label="Primary">
        <div className="brand">
          <Logo />
          <div>
            <Wordmark height={13} />
            <div className="brand-project">{project.data?.name ?? "…"}</div>
          </div>
        </div>
        {nav.map((n) => (
          <a
            key={n.page}
            href={href(n.page)}
            className={`nav-item${n.match.includes(route.page) ? " active" : ""}`}
          >
            {n.label}
            {n.page === "sessions" && running > 0 && <span className="nav-count">{running}</span>}
          </a>
        ))}
        {project.data && (
          <div className="sidebar-footer">
            <span>{project.data.principal}</span>
            <span className="mono" title={project.data.key}>
              {project.data.key.slice(0, 22)}…
            </span>
            <span style={{ color: project.data.trusted ? "var(--verified)" : "var(--breach)" }}>
              {project.data.trusted
                ? `trusted · ${project.data.roles.join(", ") || "no roles"}`
                : "key not trusted"}
            </span>
            <span>{project.data.version === "dev" ? "dev build" : `v${project.data.version}`}</span>
          </div>
        )}
      </nav>
      <main className="main">
        <Page route={route} version={version} />
      </main>
    </div>
  );
}

function Page({ route, version }: { route: Route; version: number }) {
  switch (route.page) {
    case "workspace":
      return <WorkspacePage name={route.name} tab={route.tab} version={version} />;
    case "sessions":
      return <SessionsPage version={version} />;
    case "reviews":
      return <ReviewsPage version={version} />;
    case "review":
      return <ReviewPage sha={route.sha} version={version} />;
    case "ledger":
      return <LedgerPage version={version} />;
    case "policy":
      return <PolicyPage />;
    case "agents":
      return <AgentsPage />;
    default:
      return <WorkspacesPage version={version} />;
  }
}
