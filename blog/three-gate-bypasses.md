---
title: Three ways to pass the gate without meeting policy
date: 2026-11-26
summary: "Before v0.1.0, seralith gate skipped every merge commit, accepted an approval of a different commit, and evaluated policy on the paths an attestation declared. All three were fixed before the first release."
---

Before the first public release, three gaps in `seralith gate` let a commit
pass without meeting policy. All three were closed on 25 November 2026 by
[aea7f2b](https://github.com/seralith-hq/seralith/commit/aea7f2b51d591945a6b7173dd9aafcec21f1c950)
(`fix(verify): close three ways to pass the gate without meeting policy`),
merged to `main` as
[7c48f8f](https://github.com/seralith-hq/seralith/commit/7c48f8f6a41d1ccaf022d405357cc53c4fe6625b).
GitHub lists no pull request on either commit. Both are ancestors of the
`v0.1.0` tag, and that is the only release tag, so no released version was
affected.

```text
$ git log -1 --format='%H%n%ci%n%s' aea7f2b51d591945a6b7173dd9aafcec21f1c950
aea7f2b51d591945a6b7173dd9aafcec21f1c950
2026-11-25 06:51:14 +0000
fix(verify): close three ways to pass the gate without meeting policy
$ git log -1 --format='%H%n%ci%n%s' 7c48f8f6a41d1ccaf022d405357cc53c4fe6625b
7c48f8f6a41d1ccaf022d405357cc53c4fe6625b
2026-11-25 01:54:58 -0500
Merge: close three ways to pass the gate without meeting policy
$ git merge-base --is-ancestor aea7f2b51d591945a6b7173dd9aafcec21f1c950 v0.1.0; echo "aea7f2b ancestor of v0.1.0: $?"
aea7f2b ancestor of v0.1.0: 0
$ git merge-base --is-ancestor 7c48f8f6a41d1ccaf022d405357cc53c4fe6625b v0.1.0; echo "7c48f8f ancestor of v0.1.0: $?"
7c48f8f ancestor of v0.1.0: 0
$ git rev-parse v0.1.0
5d6ee101be5a44b224a90c208298604c5ed57688
$ git tag --contains aea7f2b51d591945a6b7173dd9aafcec21f1c950
v0.1.0
$ git tag -l 'v*'
v0.1.0
$ gh api repos/seralith-hq/seralith/commits/aea7f2b51d591945a6b7173dd9aafcec21f1c950/pulls --jq length
0
$ gh api repos/seralith-hq/seralith/commits/7c48f8f6a41d1ccaf022d405357cc53c4fe6625b/pulls --jq length
0
```

The commands above were run in this repository on 26 November 2026. The
v0.1.0 changelog has no Fixed section and no Security section. Under Added,
the gate is described as skipping a merge commit only when it is exactly the
clean merge of its parents. That sentence matches the code that shipped.

The parent of the fix is
[816a009](https://github.com/seralith-hq/seralith/commit/816a009a08989bb2e6bb4f57bd10dafd58d93247).
The snippets below are taken from that tree and from `v0.1.0`, which still
has the same `internal/verify/verify.go` as the fix.

## 1. A merge commit that carries its own tree

Threat [T2](/docs/security/threat-model#threats-and-mitigations).

The attack needs push access and no signing key. Build a merge commit by
hand, for example with `git commit-tree`, whose tree is not what merging the
parents produces. The tree can add files that appear in neither parent. Put
no attestation on the commit.

The gate accepted it because, with `SkipMerges` set, every commit with more
than one parent was returned as skipped before notes were read. `SkipMerges`
is the default. The flag `--include-merges` is what turns it off.

```go
	if opts.SkipMerges && len(parents) > 1 {
		rep.Status = StatusSkipped
		return rep, nil
	}
```

That is
[verify.go at 816a009, lines 83–86](https://github.com/seralith-hq/seralith/blob/816a009a08989bb2e6bb4f57bd10dafd58d93247/internal/verify/verify.go#L83-L86).
The comment on `SkipMerges` then said merge commits were covered by the
attestations of their parents. A hand-built merge is not.

The fix skips a merge only when `mergeIsDerived` reports that its tree equals
one parent's tree or, for exactly two parents, the conflict-free result of
`git merge-tree --write-tree --no-messages` on those parents. If `merge-tree`
fails, including on conflicts, the merge is not derived. Any other merge
records this failure and then has to pass the normal change-attestation
checks:

```go
	if opts.SkipMerges && len(parents) > 1 {
		if derived, err := mergeIsDerived(ctx, repo, commit, parents); err != nil {
			return nil, err
		} else if derived {
			rep.Status = StatusSkipped
			return rep, nil
		}
		rep.fail("merge commit is not a clean merge of its parents, so it needs a change attestation")
	}
```

[v0.1.0, lines 87–95](https://github.com/seralith-hq/seralith/blob/v0.1.0/internal/verify/verify.go#L87-L95).
If a change attestation is selected, that "needs a change attestation"
problem is cleared and the attestation is checked in the ordinary way: a
merge with content of its own is allowed when it is signed and meets policy,
and rejected when it is not. `--include-merges` still requires an attestation
on every merge, including a clean one
([`SkipMerges: !includeMerges`](https://github.com/seralith-hq/seralith/blob/v0.1.0/internal/cli/verify.go#L84)).

T2 now says: "The gate requires a change attestation for every commit except
a merge whose tree is exactly the clean merge of its parents." The same rule
is in the [attestation spec](/docs/spec/attestation#verification-algorithm)
and under Known limitations, "Merge commits." The regression test is
`TestMergeWithForeignContentIsNotSkipped`. It builds the merge with
`git commit-tree`, runs the gate, and expects the problem text "not a clean
merge".

## 2. An approval copied from another commit

Threat [T9](/docs/security/threat-model#threats-and-mitigations).

The attack starts from a commit that already has a `change/v1` attestation
signed by a key in the trust store. Take a valid `approval/v1` for some other
commit, signed by a human key that holds the required role, and copy that
envelope into the git note of the commit you want the gate to accept. The
signature is not forged. It is reused.

The gate accepted it because a trusted signature was enough. Approvals were
loaded from the commit's own note and, for a range, from the range tip, and
`satisfy` counted every approval marked valid. The pre-fix check did require
the approval's subject to equal its own predicate commit (`subject does not
match predicate`). It did not require that commit to be the commit under
review, or the range tip when the note was the tip's. An approval of
unrelated work could meet a `require-approval` rule on a protected path.

The fix passes a target into `collectApprovals`. For a note on a commit the
target is that commit. For the approvals `Gate` copies from the range tip,
the target is the tip (`ApprovalTip`). An approval whose predicate names a
different commit is not valid:

```go
		switch {
		case a.Statement.Commit() != pred.Commit:
			ap.Problem = "subject does not match predicate"
		case target == "" || pred.Commit != target:
			ap.Problem = "approval is for a different commit"
```

[v0.1.0, lines 306–310](https://github.com/seralith-hq/seralith/blob/v0.1.0/internal/verify/verify.go#L306-L310).
An approval of the tip, stored on the tip, still covers every commit in the
range. That is the pull-request case: review the head, and the commits it
contains are covered. An approval of some other commit, stored on the tip or
on a commit in the range, is not.

T9 now says: "Approvals are bound to a commit subject; only approvals of the
commit itself, or of the range tip, count." `TestApprovalForAnotherCommitIsRejected`
copies a good approval onto a second protected commit and expects status fail
with "approval is for a different commit", both from `verify.Commit` and from
`verify.Gate`. `TestApprovalOnRangeTipCoversRange` expects an approval of the
tip to pass the range.

## 3. A change attestation that leaves paths out

Threat [T4](/docs/security/threat-model#threats-and-mitigations).

The attack needs the committer's signing key, or another trusted key whose
principal is the committer named in the predicate. Sign a `change/v1` whose
segment trees still chain from the first parent to the commit tree, so the
tree-chain check passes, but whose `files` list omits a path or records fewer
added lines than the diff contains. Path rules and line thresholds then never
see the omitted part.

The gate accepted it because policy ran only on the file lists the segments
declared. `checkIntegrity` already required the predicate tree and parents to
match the commit, and the segment trees to chain to the commit tree. It did
not compare the declared paths with git's diff. A segment can name a result
tree that contains a file the `files` list does not mention.

```go
	res := opts.Policy.Evaluate(policy.ChangesFromSegments(pred.Segments))
```

[816a009, line 106](https://github.com/seralith-hq/seralith/blob/816a009a08989bb2e6bb4f57bd10dafd58d93247/internal/verify/verify.go#L106).

The fix diffs the first parent's tree, or the empty tree for a root commit,
against the commit tree. A path git reports that no segment declares fails
the commit. The old path of a rename must be declared too. Added lines beyond
the number the segments declare for that path are attributed to every actor
that declared the path, so a line threshold still sees them:

```go
	for _, f := range actual {
		owners := actors[f.Path]
		if len(owners) == 0 {
			rep.fail("change to %s is not declared by any segment", f.Path)
			continue
		}
		if f.OldPath != "" && len(actors[f.OldPath]) == 0 {
			rep.fail("rename from %s is not declared by any segment", f.OldPath)
		}
		if extra := f.LinesAdded - declared[f.Path]; extra > 0 {
			for _, a := range owners {
				changes = append(changes, policy.Change{Actor: a, Path: f.Path, LinesAdded: extra})
			}
		}
```

[v0.1.0, lines 277–290](https://github.com/seralith-hq/seralith/blob/v0.1.0/internal/verify/verify.go#L277-L290).
`observedChanges` is what policy evaluates. The failure strings are
`change to <path> is not declared by any segment` and
`rename from <path> is not declared by any segment`.

T4 now says: "Segments must chain to the commit tree exactly, and every path
git reports between the first parent and the commit must be declared by a
segment." The [attestation spec](/docs/spec/attestation#verification-algorithm)
states the same check as step 4 of verification. `TestUndeclaredFilesAreDetected`
rewrites a real attestation so its segment names `README.md` instead of
`internal/crypto/evil.go`, reseals it, and expects the hidden path to fail.
`TestUnderstatedLinesStillTriggerThresholds` sets the declared line count to
1 on a 1600-line agent change and expects the same policy decisions as the
honest attestation.

## What the three attacks required

The merge did not need a key. Push access was enough, because the gate never
looked for a signature.

The replayed approval and the understated attestation both need a
`change/v1` signed by a key in the trust store. The replay does not sign
anything new: it reuses an approval that already verifies, and uses it to
satisfy an independent review the protected path requires. The understated
attestation has to be sealed again by the committer's key after the `files`
list is edited. Neither one is available to someone who can only push.

Two other changes landed in the same pre-release window and are not these
bugs. [23a2c1e](https://github.com/seralith-hq/seralith/commit/23a2c1e567769498f5bb4a19494864eba8381c04)
makes `humanLabels: unverified` the default, so agent rules that name paths
also apply to human-labeled changes on those paths.
[01339f0](https://github.com/seralith-hq/seralith/commit/01339f03b33594eb8ef2a4d4041c48961cc8e7b2)
makes Seralith Control check uploads with the gate's identity rules. That is
threat T13.

## Where the rules live

- [internal/verify/verify.go](https://github.com/seralith-hq/seralith/blob/v0.1.0/internal/verify/verify.go) — `mergeIsDerived`, `collectApprovals`, `observedChanges`
- [internal/verify/integrity_test.go](https://github.com/seralith-hq/seralith/blob/v0.1.0/internal/verify/integrity_test.go)
- [Threat model](/docs/security/threat-model)
- [Attestation spec](/docs/spec/attestation)
