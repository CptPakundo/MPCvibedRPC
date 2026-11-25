# Immutable releases and this repository's release flow

Read 2026-11-11 against GitHub's documentation ("Preventing changes to your releases" and the concept page "Immutable
releases"). The setting is off; this page is the background for the roadmap's
[open decision 2](../roadmap.md#open-decisions-the-maintainers).

## Answer
They fit. The release flow already does what GitHub recommends: `release.yml` uploads every file to a **draft**
(`softprops/action-gh-release`, `draft: true`), the `verify` job checks the draft, and only then runs
`gh release edit "v$VERSION" --draft=false --latest`. GitHub's recommended flow for immutable releases is exactly
"create a draft, attach all assets, publish". The setting applies to releases published after it is on; existing
releases stay as they are.

## What GitHub says
- Once published: the git tag is fixed to its commit and cannot be moved or deleted while the release exists; the
  assets cannot be modified or deleted. Title, notes and the pre-release / latest flag stay editable.
- Whether assets may be ADDED after publishing is not stated; assume not. The flow never does that.
- Deleting an immutable release lets you delete its tag, but the tag NAME can never be reused (also not by a
  recreated repository).
- A signed release attestation (tag, commit, assets) is created automatically, next to the build provenance
  attestation `release.yml` already makes.

## What changes for this repository
1. **A bad published release cannot be redone under the same version.** While a release is a draft, the usual
   remedy stays possible: fix, delete the draft and the tag, release again (the `verify` job only publishes after the
   draft checks and the rebuild comparison pass; see [maintaining.md](../maintaining.md#releasing)). Once public, a
   defect means a new version (0.9.N+1). The public-stage checks in `verify-release.ps1` (latest, a real self-update)
   run after publishing, so a failure there costs a version number, not a retag.
2. The workflow's guard "a re-run after the release went public would turn it back into a draft" is already in
   `release.yml`; with immutability a re-run of the `release` job on a public release would fail anyway.
3. `tools/ci/dev-download.sh`, the verify job's downloads and the stats ledger are unaffected (they only read).
4. A re-run of a release job that already created the draft replaces the draft's files: still fine, drafts are not
   locked.
5. Releases published before the setting is on are not made immutable.

## The switch
The repository belongs to a personal account, not an organization. Either:
- in the web page: repository **Settings**, the **Releases** section, tick **Enable release immutability**; or
- from a terminal, as the repository admin:

```bash
gh api -X PUT repos/OWNER/REPO/immutable-releases
```

Check the state (it answered `{"enabled":false,"enforced_by_owner":false}` on 2026-11-11):

```bash
gh api repos/OWNER/REPO/immutable-releases
```

Undo (affects releases published from then on): `gh api -X DELETE repos/OWNER/REPO/immutable-releases`.

## Suggested order
Turn it on right after a release is public and verified, so the first release under the new rule is a deliberate one
and the next release is the first immutable one. Then add a line to [decisions.md](../decisions.md#releases) and to
the release steps in [maintaining.md](../maintaining.md#releasing) saying that a public release can no longer be
deleted and retagged. The decision is the maintainer's.
