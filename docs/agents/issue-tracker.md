# Issue Tracker: Beads

Issues and PRDs for this repo live in Beads. Use the `br` CLI for all issue operations.

## Conventions

* Create issues with `br create`.
* Read issues with `br show <id>` and comments with `br comments <id>`.
* Update issues with `br update <id>`.
* Add comments with `br comments add <id> "..."`.
* Apply and remove labels with `br label add <id> <label>` and `br label remove <id> <label>`.
* Add dependencies with `br dep add <child> <parent> --type parent-child` or `br dep add <blocked> <blocker> --type blocks`.
* After Beads edits, run `br sync --flush-only`.
* Commit `.beads/issues.jsonl` with the code or planning changes that caused the issue update.

## When a skill says "publish to the issue tracker"

Create a Beads issue with `br create`. Use a quoted heredoc for multiline descriptions, designs, acceptance criteria, or notes.

## When a skill says "fetch the relevant ticket"

Run `br show <id>` and `br comments <id>`.

## Important rules

* Use standard Beads IDs such as `ab-xyz`; do not create dotted child IDs.
* Do not use `br create --parent` unless the user explicitly wants dotted child IDs.
* To create a normal child issue, create the issue first, then link it with `br dep add <child> <parent> --type parent-child`.
* Do not use another tracker for this repo unless the user explicitly asks.
