import { useResource } from "../lib/hooks";
import type { PolicyRule, ProjectInfo } from "../lib/types";
import { Badge, Empty, ErrorBox } from "../components/ui";

function describe(rule: PolicyRule): string[] {
  const w = rule.when;
  const out: string[] = [];
  if (w.actors?.length) out.push(`actors: ${w.actors.join(", ")}`);
  if (w.tools?.length) out.push(`tools: ${w.tools.join(", ")}`);
  if (w.toolsNotIn?.length) out.push(`tools not in: ${w.toolsNotIn.join(", ")}`);
  if (w.models?.length) out.push(`models: ${w.models.join(", ")}`);
  if (w.paths?.length) out.push(`paths: ${w.paths.join("  ")}`);
  if (w.exclude?.length) out.push(`except: ${w.exclude.join("  ")}`);
  if (w.files?.gt !== undefined) out.push(`more than ${w.files.gt} files`);
  if (w.linesAdded?.gt !== undefined) out.push(`more than ${w.linesAdded.gt} lines added`);
  return out;
}

export function PolicyPage() {
  const policy = useResource<{ rules: PolicyRule[] }>("/policy");
  const project = useResource<ProjectInfo>("/project");
  return (
    <div className="page">
      <div className="page-header">
        <div>
          <h1 className="page-title">Policy</h1>
          <p className="page-subtitle">
            Rules from <code>.seralith/policy.yaml</code>. The gate re-evaluates them independently on every
            commit.
          </p>
        </div>
        {project.data && (
          <span className="mono small muted">sha256 {project.data.policyDigest.slice(0, 16)}</span>
        )}
      </div>
      <ErrorBox error={policy.error} />
      {policy.data?.rules.length === 0 && (
        <div className="card">
          <Empty title="No rules">
            Every change is allowed. Run `seralith policy default` for a starting point.
          </Empty>
        </div>
      )}
      <div className="grid">
        {policy.data?.rules.map((r) => (
          <div key={r.id} className="card card-pad grid" style={{ gap: 6 }}>
            <div className="row spread">
              <span className="mono" style={{ fontWeight: 600 }}>
                {r.id}
              </span>
              <Badge value={r.effect} />
            </div>
            {r.description && <div>{r.description}</div>}
            <div className="small muted mono">{describe(r).join(" · ")}</div>
            {r.effect === "require-approval" && (
              <div className="small">
                Requires {r.approvals} independent approval{r.approvals === 1 ? "" : "s"}
                {r.roles?.length ? ` from ${r.roles.join(" / ")}` : ""}.
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
