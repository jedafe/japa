Skills trigger **automatically by description matching** — you don't invoke them with a command. Claude Code reads the `description` frontmatter of every skill in `.claude/skills/` and loads the one(s) matching your request. Your job is just to phrase the request so it clearly signals the **app** and the **task type**.

## How it works for the Control app

| You type in Claude Code | Skill that loads |
|---|---|
| "Build the system settings page for the control app" | `jc-control-develop` |
| "Review this control panel component, the health dashboard isn't updating" | `jc-control-debug` |
| "Refactor apps/control/src/routes/jobs to modern patterns" | `jc-control-refactor` |
| "Run a full audit on the control app before I merge" | `jc-control-audit` |

The two signals that matter in your phrasing:
1. **App identity** — "control app", "control panel", or `apps/control` in a path
2. **Task verb** — build/implement/create → develop · review/debug/fix/broken → debug · refactor/modernize/update/clean up → refactor · audit/verify/check compliance → audit

## Explicit invocation (when you want zero ambiguity)

You can also name the skill directly, which forces it:

```
Use the jc-control-develop skill and build the extension registry page.
```

Or reference the file itself:

```
Read .claude/skills/jc-control-audit/SKILL.md and audit apps/control/src/routes.
```

I'd recommend the explicit form for **audits** specifically — audit runs are deliberate checkpoint events, and naming the skill guarantees the full checklist and report format is loaded rather than the agent partially matching on "check this."

## Two practical notes

**Ambiguous requests load the wrong skill.** "Fix the users table" doesn't say which app — the admin and control panels both have tables. Include the app name or a file path (`apps/control/...`) and matching becomes deterministic, since the paths are in every skill description.

**One skill per session/task is the intended flow.** If you're building a page and hit a bug mid-way, you don't need to switch — the develop skill's gates and conventions cover you. Reach for the debug skill when reviewing *existing* code as its own task. And since the skills cross-reference each other (e.g. `jc-control-debug` points to `jc-control-develop` for conventions), Claude Code will pull the referenced sibling automatically when it needs it — that's why all 20 must be installed together.

In Cursor (your secondary agent) there's no auto-triggering — you reference explicitly: `@.claude/skills/jc-control-develop/SKILL.md` in the chat or a Notepad.
