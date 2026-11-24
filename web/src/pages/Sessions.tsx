import { useResource } from "../lib/hooks";
import type { SessionInfo } from "../lib/types";
import { SessionTerminal } from "../components/Terminal";
import { Empty, ErrorBox } from "../components/ui";
import { href } from "../lib/router";

export function SessionsPage({ version }: { version: number }) {
  const { data, error, reload } = useResource<SessionInfo[]>("/sessions", [version]);
  const live = data?.filter((s) => s.state === "running") ?? [];
  const finished = data?.filter((s) => s.state !== "running").slice(0, 4) ?? [];

  return (
    <div className="page">
      <div className="page-header">
        <div>
          <h1 className="page-title">Live sessions</h1>
          <p className="page-subtitle">Every agent and shell running across your workspaces, side by side.</p>
        </div>
      </div>
      <ErrorBox error={error} />
      {live.length === 0 && (
        <div className="card" style={{ marginBottom: 18 }}>
          <Empty title="Nothing running">
            Start sessions from a{" "}
            <a href={href("workspaces")} style={{ color: "var(--signal)" }}>
              workspace
            </a>
            . Run as many in parallel as you have workspaces.
          </Empty>
        </div>
      )}
      <div className="session-grid">
        {live.map((s) => (
          <div key={s.id} className="grid" style={{ gap: 6 }}>
            <a className="small muted" href={href("workspaces", s.workspace)}>
              {s.workspace} →
            </a>
            <SessionTerminal session={s} onExit={reload} />
          </div>
        ))}
      </div>
      {finished.length > 0 && (
        <>
          <h3 style={{ margin: "26px 0 12px" }}>Recently finished</h3>
          <div className="session-grid">
            {finished.map((s) => (
              <div key={s.id} className="grid" style={{ gap: 6 }}>
                <a className="small muted" href={href("workspaces", s.workspace)}>
                  {s.workspace} →
                </a>
                <SessionTerminal session={s} />
              </div>
            ))}
          </div>
        </>
      )}
    </div>
  );
}
