import type { ReactNode } from "react";
import type { Actor, ActorKind, Decision, DecisionStatus } from "../lib/types";
import { actorLabel, percentages } from "../lib/format";

export function Logo({ size = 28 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 64 64" role="img" aria-label="Sigil">
      <path d="M32 3 57.1 17.5v29L32 61 6.9 46.5v-29Z" fill="#0E1116" stroke="#262C3A" />
      <path
        d="M32 9.5 51.5 20.75v22.5L32 54.5 12.5 43.25v-22.5Z"
        fill="none"
        stroke="#FF5A1F"
        strokeWidth="2.5"
      />
      <path
        d="M41 21H25.5v10.5h13V43H23"
        fill="none"
        stroke="#E9E6DF"
        strokeWidth="5"
        strokeLinecap="square"
      />
      <circle cx="41" cy="21" r="3" fill="#3DDC97" />
    </svg>
  );
}

export function Badge({ value, pulse = false }: { value: string; pulse?: boolean }) {
  return (
    <span className={`badge badge-${value}`}>
      <span className={`dot${pulse ? " pulse" : ""}`} />
      {value}
    </span>
  );
}

export function ActorChip({ actor }: { actor: Actor }) {
  return (
    <span
      className={`chip chip-${actor.kind}`}
      title={actor.operator ? `on behalf of ${actor.operator}` : actor.id}
    >
      {actor.kind === "agent" ? "◆" : "●"} {actorLabel(actor)}
    </span>
  );
}

export function AttributionBar({
  lines,
  showLegend = true,
}: {
  lines?: Partial<Record<ActorKind, number>>;
  showLegend?: boolean;
}) {
  const p = percentages(lines);
  return (
    <div className="grid" style={{ gap: 6 }}>
      <div className="attr-bar" aria-label={`human ${p.human}%, agent ${p.agent}%`}>
        <div className="human" style={{ width: `${p.human}%` }} />
        <div className="agent" style={{ width: `${p.agent}%` }} />
      </div>
      {showLegend && (
        <div className="legend">
          <span>
            <i style={{ background: "var(--human)" }} />
            human {lines?.human ?? 0}
          </span>
          <span>
            <i style={{ background: "var(--agent)" }} />
            agent {lines?.agent ?? 0}
          </span>
        </div>
      )}
    </div>
  );
}

export function Empty({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <div className="empty">
      <h3>{title}</h3>
      {children}
    </div>
  );
}

export function ErrorBox({ error }: { error?: string }) {
  if (!error) return null;
  return <div className="error-box">{error}</div>;
}

export function Modal({
  title,
  children,
  onClose,
}: {
  title: string;
  children: ReactNode;
  onClose: () => void;
}) {
  return (
    <div className="modal-backdrop" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="modal" role="dialog" aria-modal="true" aria-label={title}>
        <h2>{title}</h2>
        {children}
      </div>
    </div>
  );
}

export function Decisions({ decisions }: { decisions?: (Decision | DecisionStatus)[] }) {
  if (!decisions || decisions.length === 0) {
    return (
      <div className="row small">
        <Badge value="pass" /> <span className="muted">No policy rules triggered</span>
      </div>
    );
  }
  return (
    <div className="grid" style={{ gap: 10 }}>
      {decisions.map((d) => {
        const status = "satisfied" in d ? d : undefined;
        return (
          <div key={d.rule} className="card card-pad" style={{ padding: "10px 12px" }}>
            <div className="row spread">
              <span className="mono">{d.rule}</span>
              <div className="row">
                {status && d.effect === "require-approval" && (
                  <Badge value={status.satisfied ? "pass" : "fail"} />
                )}
                <Badge value={d.effect} />
              </div>
            </div>
            {d.message && (
              <div className="small" style={{ marginTop: 4 }}>
                {d.message}
              </div>
            )}
            {d.effect === "require-approval" && (
              <div className="small muted">
                needs {d.approvals} approval{d.approvals === 1 ? "" : "s"}
                {d.roles?.length ? ` from ${d.roles.join(" / ")}` : ""}
                {status?.approvers?.length ? ` · approved by ${status.approvers.join(", ")}` : ""}
              </div>
            )}
            {d.paths && d.paths.length > 0 && (
              <div className="small muted mono" style={{ marginTop: 4 }}>
                {d.paths.join("  ")}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}
