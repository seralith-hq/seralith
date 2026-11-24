export type ActorKind = "human" | "agent";

export interface Actor {
  kind: ActorKind;
  id: string;
  tool?: string;
  toolVersion?: string;
  model?: string;
  operator?: string;
  host?: string;
}

export interface Hunk {
  oldStart: number;
  oldLines: number;
  newStart: number;
  newLines: number;
}

export interface FileChange {
  path: string;
  oldPath?: string;
  status: "added" | "modified" | "deleted" | "renamed";
  linesAdded: number;
  linesDeleted: number;
  binary?: boolean;
  hunks?: Hunk[];
}

export interface Segment {
  id: string;
  kind: "session" | "manual";
  actor: Actor;
  command?: string[];
  startedAt: string;
  endedAt: string;
  baseTree: string;
  resultTree: string;
  exitCode?: number;
  files: FileChange[] | null;
}

export type Effect = "allow" | "warn" | "require-approval" | "deny";

export interface Decision {
  rule: string;
  effect: Effect;
  message?: string;
  paths?: string[];
  approvals?: number;
  roles?: string[];
}

export interface Summary {
  filesChanged: number;
  linesByKind: Partial<Record<ActorKind, number>>;
  segments: number;
}

export interface Workspace {
  name: string;
  path: string;
  branch: string;
  baseCommit: string;
  owner: string;
  createdAt: string;
  updatedAt: string;
  status: string;
  head: string;
  checkpoint: string;
  segments: Segment[];
  running?: boolean;
}

export interface Preview {
  Workspace: Workspace;
  Segments: Segment[] | null;
  Summary?: Summary;
  Policy?: { decisions: Decision[] };
  Effect?: Effect;
  Running?: boolean;
}

export interface SessionInfo {
  id: string;
  workspace: string;
  state: "running" | "exited" | "failed";
  actor: Actor;
  command: string[];
  startedAt: string;
  endedAt?: string;
  exitCode?: number;
  segment?: string;
  filesChanged: number;
  linesAdded: number;
  linesDeleted: number;
  error?: string;
}

export interface AgentDetection {
  profile: { id: string; name: string; kind: ActorKind; command: string[]; custom?: boolean };
  installed: boolean;
  path?: string;
}

export type VerifyStatus = "pass" | "warn" | "fail" | "skipped";

export interface CommitRow {
  commit: string;
  subject: string;
  author: string;
  date: string;
  branches: string[];
  status: VerifyStatus;
  lines?: Partial<Record<ActorKind, number>>;
  problems: number;
}

export interface Approval {
  approver: string;
  role?: string;
  rules?: string[];
  decision: "approve" | "reject";
  commit: string;
  valid: boolean;
  problem?: string;
}

export interface DecisionStatus extends Decision {
  satisfied: boolean;
  approvers?: string[];
}

export interface CommitReport {
  commit: string;
  subject?: string;
  status: VerifyStatus;
  attested: boolean;
  signers?: string[];
  summary?: Summary;
  decisions?: DecisionStatus[];
  approvals?: Approval[];
  problems?: string[];
  warnings?: string[];
  attestation?: string;
}

export interface AttributedRange {
  start: number;
  end: number;
  segment: string;
}

export interface ChangePredicate {
  commit: string;
  tree: string;
  parents: string[];
  workspace?: string;
  branch?: string;
  committer: Actor;
  segments: Segment[];
  attribution: { path: string; ranges: AttributedRange[] }[] | null;
  summary: Summary;
  createdAt: string;
}

export interface LedgerEntry {
  seq: number;
  time: string;
  type: string;
  actor?: string;
  subject?: string;
  data?: unknown;
  prev: string;
  hash: string;
}

export interface LedgerReport {
  entries: number;
  head: string;
  checkpoints: number;
  verifiedCheckpoints: number;
  issues?: { seq: number; problem: string }[];
}

export interface PolicyRule {
  id: string;
  description?: string;
  effect: Effect;
  message?: string;
  approvals?: number;
  roles?: string[];
  when: {
    actors?: ActorKind[];
    tools?: string[];
    toolsNotIn?: string[];
    models?: string[];
    paths?: string[];
    exclude?: string[];
    files?: { gt?: number };
    linesAdded?: { gt?: number };
  };
}

export interface ProjectInfo {
  name: string;
  root: string;
  principal: string;
  key: string;
  trusted: boolean;
  roles: string[];
  policyDigest: string;
  version: string;
}
