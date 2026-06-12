# Documentation Index

One row per doc. `Reader` is the addressed reader (`user`, `operator`,
`contributor`, `adopter`, `+`-joined for genuinely sectioned docs, `—` for
point-in-time rows). `Class` is the confirmed evergreen/point-in-time
classification of record (the classify-and-confirm gate's output). `Owns` is
machine-readable: the path globs whose facts this doc owns — `docmaint stale`
diffs them; `—` for point-in-time docs. The fenced table is machine-maintained;
edit rows, never the sentinels.

<!-- doc-index:begin -->
| Doc | What | Reader | Class | Owns |
| --- | --- | --- | --- | --- |
| `README.md` | install, setup, every command, config, exit codes, event reference | user + operator | evergreen | `cmd/**`, `install.sh`, `listen/**` |
| `docs/BROCHURE.md` | adopter-facing positioning: what slackline is, who it's for, getting started | adopter | evergreen | `cmd/**`, `listen/**` |
| `CLAUDE.md` | architecture/package map + dev workflow for AI coding agents | contributor | evergreen | `cmd/**`, `slack/**`, `listen/**`, `config/**`, `errs/**`, `provision/**`, `main.go`, `Makefile` |
| `AGENTS.md` | symlink → `CLAUDE.md` (same guidance under the AGENTS.md filename) | contributor | symlink | — |
| `skills/using-slack/SKILL.md` | agent-facing how-to for the CLI (send/read/ask/listen/react/download) | user | evergreen | `cmd/**`, `listen/**` |
| `skills/using-slack/provisioning.md` | admin recipe to provision + install a new bot | operator | evergreen | `cmd/provision.go`, `cmd/initcmd.go` |
| `skills/using-slack/copy-buttons.md` | Slack admin-UI selector reference (external ground truth) | operator | evergreen | — |
| `docs/DICTIONARY.md` | project dictionary (normative terminology) + voice | contributor | evergreen | — |
| `CHANGELOG.md` | release history (Keep a Changelog) | — | point-in-time | — |
| `docs/specs/2026-03-16-slackline-design.md` | original design spec | — | point-in-time | — |
| `docs/specs/2026-03-17-distribution-design.md` | distribution/install design | — | point-in-time | — |
| `docs/specs/2026-04-25-slackline-threading-reactions-attachments-design.md` | threading/reactions/attachments/provisioning design | — | point-in-time | — |
| `docs/specs/2026-06-01-cut-jq-friction-and-consolidate-skills-design.md` | jq-friction + skill-consolidation design (PRI-2017) | — | point-in-time | — |
| `docs/plans/2026-04-25-slackline-threading-reactions-attachments-plan.md` | implementation plan for the 2026-04-25 design | — | point-in-time | — |
| `docs/plans/2026-06-01-cut-jq-friction-and-consolidate-skills.md` | implementation plan for the 2026-06-01 design | — | point-in-time | — |
| `ABOUT.md` | project-map summary card | — | generated | — |
<!-- doc-index:end -->

<!-- `ABOUT.md` is foreign-owned: it carries a `maintaining-project-map …
do not hand-edit; regenerated` sentinel, so it is excluded from audit, edits,
and stamping (the next regeneration would wipe any change). Listed here only so
the set is complete and a future study does not mistake it for a missed doc. -->

<!-- `AGENTS.md` is a symlink to `CLAUDE.md` (the only delta was the heading and
intro, so CLAUDE.md was made tool-neutral and AGENTS.md now points at it). Its
Class is `symlink`, not `evergreen`, so `docmaint stale` skips it and CLAUDE.md
is stamped once for both filenames; `scan` follows the link harmlessly. Resolves
finding B (the former byte-for-byte CLAUDE.md/AGENTS.md duplication). -->

<!-- Owns `—` = the doc owns no code surface, so `stale` never flags it. Right
for point-in-time rows, the dictionary (freshness from `scan` + full audits),
and `copy-buttons.md` (ground truth is Slack's external admin UI, not repo code). -->

<!-- Decided gaps (confirmed "we don't write that here", 2026-06-11):
- No standalone getting-started tutorial: README "Setup" + the using-slack
  skill already carry first-success for both the user and the operator.
- No separate architecture/reference doc: CLAUDE.md + AGENTS.md serve
  contributors; splitting would duplicate the package map.
- No in-repo brochure render (docs/index.html): the prime-radiant website repo
  (prime-radiant-inc.github.io) renders brochure pages mechanically from each
  repo's BROCHURE.md at build time. BROCHURE.md plus its stamp is the render
  contract. (Human-authorized deviation from the marketing flow's step 4.)
-->
