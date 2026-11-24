package evidence

// Control maps a regulatory requirement to the Sigil evidence that supports it.
type Control struct {
	Framework string   `json:"framework"`
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Evidence  string   `json:"evidence"`
	Artifacts []string `json:"artifacts"`
}

// Controls is the mapping included in every evidence package. It describes what the
// artifacts demonstrate; it is not a legal determination of compliance.
var Controls = []Control{
	{
		Framework: "DoWI 8430.01",
		ID:        "AI-Generated Code: Accountability",
		Title:     "AI-generated code is unverified input; a human remains accountable",
		Evidence:  "Every agent segment records the accountable operator, and every commit is signed by its human committer.",
		Artifacts: []string{"attestations/", "report.md#attribution"},
	},
	{
		Framework: "DoWI 8430.01",
		ID:        "AI-Generated Code: Security Validation",
		Title:     "Human review and approval of AI changes to security- or safety-critical functionality",
		Evidence:  "Policy rules require independent, role-bound, signed approvals before agent changes to designated paths pass the gate.",
		Artifacts: []string{"policy.yaml", "gate-report.json", "attestations/"},
	},
	{
		Framework: "DoWI 8430.01",
		ID:        "Verifiable Build Integrity, Provenance, and Pedigree",
		Title:     "Record of tools, versions, and models used to produce software",
		Evidence:  "Segments record tool, tool version, declared model, command, host, and transcript digest for every change.",
		Artifacts: []string{"ai-usage.json", "attestations/"},
	},
	{
		Framework: "NIST SP 800-218",
		ID:        "PO.3.2",
		Title:     "Follow recommended security practices to deploy, operate, and maintain tools and toolchains",
		Evidence:  "Tool allowlist rules constrain which coding agents may contribute; violations fail the gate.",
		Artifacts: []string{"policy.yaml", "gate-report.json"},
	},
	{
		Framework: "NIST SP 800-218",
		ID:        "PS.1.1",
		Title:     "Store all forms of code based on the principle of least privilege",
		Evidence:  "Agents operate in isolated worktrees; pipeline definitions and secrets are protected by deny rules.",
		Artifacts: []string{"policy.yaml"},
	},
	{
		Framework: "NIST SP 800-218",
		ID:        "PS.2.1",
		Title:     "Make software integrity verification information available",
		Evidence:  "In-toto attestations in DSSE envelopes are attached to each commit and verifiable with the published trust store.",
		Artifacts: []string{"attestations/", "trust.yaml", "manifest.json"},
	},
	{
		Framework: "NIST SP 800-218",
		ID:        "PS.3.1",
		Title:     "Securely archive the necessary files and supporting data for each release",
		Evidence:  "This package is a signed, hash-indexed archive of the provenance for the release range.",
		Artifacts: []string{"manifest.json", "manifest.sig.json"},
	},
	{
		Framework: "NIST SP 800-218",
		ID:        "PW.7.1",
		Title:     "Determine whether code review should be used and what type",
		Evidence:  "Policy declares which changes require review, by whom, and how many independent approvals.",
		Artifacts: []string{"policy.yaml"},
	},
	{
		Framework: "NIST SP 800-218",
		ID:        "PW.7.2",
		Title:     "Perform the code review and record results",
		Evidence:  "Approvals and rejections are signed attestations bound to specific commits.",
		Artifacts: []string{"attestations/", "gate-report.json"},
	},
	{
		Framework: "EU CRA",
		ID:        "Annex I, Part II (1)",
		Title:     "Identify and document components and vulnerabilities",
		Evidence:  "Attribution and dependency-manifest rules identify where third-party and agent-introduced components entered the codebase.",
		Artifacts: []string{"attestations/", "gate-report.json"},
	},
	{
		Framework: "EU CRA",
		ID:        "Article 14",
		Title:     "Reporting of actively exploited vulnerabilities",
		Evidence:  "Line-level provenance answers which change, actor, and tool introduced a vulnerable line within the 24-hour early warning window.",
		Artifacts: []string{"attestations/", "ledger.jsonl"},
	},
	{
		Framework: "NIST SP 800-171",
		ID:        "3.3.1 / 3.3.8",
		Title:     "Create and retain audit records; protect audit information",
		Evidence:  "A hash-chained, checkpoint-signed ledger records sessions, commits, approvals, and overrides.",
		Artifacts: []string{"ledger.jsonl", "ledger-verify.json"},
	},
}
