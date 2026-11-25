import { useResource } from "../lib/hooks";
import type { AgentDetection } from "../lib/types";
import { Badge, ErrorBox } from "../components/ui";

export function AgentsPage() {
  const { data, error } = useResource<AgentDetection[]>("/agents");
  return (
    <div className="page">
      <div className="page-header">
        <div>
          <h1 className="page-title">Coding tools</h1>
          <p className="page-subtitle">
            Seralith launches these as-is. It never proxies model traffic, never sees your tokens, and works with
            local models.
          </p>
        </div>
      </div>
      <ErrorBox error={error} />
      <div className="card">
        <table className="data">
          <thead>
            <tr>
              <th>Tool</th>
              <th>Kind</th>
              <th>Command</th>
              <th>Status</th>
              <th>Path</th>
            </tr>
          </thead>
          <tbody>
            {data?.map((a) => (
              <tr key={a.profile.id}>
                <td>
                  <strong>{a.profile.name}</strong> <span className="mono muted small">{a.profile.id}</span>
                  {a.profile.custom && (
                    <span className="chip" style={{ marginLeft: 6 }}>
                      custom
                    </span>
                  )}
                </td>
                <td>
                  <span className={`chip chip-${a.profile.kind}`}>{a.profile.kind}</span>
                </td>
                <td className="mono small">{a.profile.command.join(" ")}</td>
                <td>
                  <Badge value={a.installed ? "pass" : "neutral"} />
                </td>
                <td className="mono small muted">{a.path}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
