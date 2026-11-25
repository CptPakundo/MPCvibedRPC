# CLAUDE.md

@AGENTS.md
@CONTRIBUTING.md

Claude Code specifics (everything else is in the two files above and in `docs/`):
- Commit as `git -c user.name="Claude" -c user.email="noreply@anthropic.com"`, never with anyone's personal email.
- Skills (`.claude/skills/`: `release`, `throwaway-copy`, `new-player`, `new-setting`, `new-catalog`, `new-host`) and
  rules (`.claude/rules/`: `engine`, `capture`, `tests`, `ci`; loaded when a matching file is read or edited) are
  only pointers to sections of `docs/development.md` and `docs/maintaining.md`. Keep the procedures there, not in the
  skills, and fix a pointer when you rename a section.
- Steps 3-4 of a release (merge to verify) go to the `releaser` agent, with the pull request number, the version and
  the owner's go-ahead in their words. Merging, tagging and publishing need that go-ahead.
