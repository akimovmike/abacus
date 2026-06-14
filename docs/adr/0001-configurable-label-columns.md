# Configurable label columns in the tree view

We chose a one-column-per-label model for showing labels in the tree view, configured through a columns overlay rather than config file editing. The `C` key opens an overlay with a master "Show columns" toggle, checkboxes for each column (built-in and label), and an "Add label column..." action that reuses the existing label picker. Each label column row has an inline editable text field for its display name, defaulting to the label's last segment (split on `-`). Label columns show the display name when the label is present, blank when absent, with the column width matching the display name length.

The overlay excludes labels that are already configured as columns from the picker. Label columns can be removed with `d`, but built-in columns (Last Updated, Assignee, Comments) cannot.

## Considered Options

- **Single "labels" column** showing compact indicators for all configured labels. Rejected because it requires learning abbreviations and scales poorly as labels are added or changed.
- **Auto-truncated label names** without user input. Rejected because mechanical truncation produces ambiguous or ugly results, and different users may want different shorthands for the same label.
- **Config-file-only configuration.** Rejected because the label set changes frequently during triage sessions, so runtime configuration via an overlay is faster.

## Consequences

- The `C` key changes from a blanket columns toggle to opening the columns overlay. The master "Show columns" toggle at the top preserves the quick hide/show-all behavior without losing per-column configuration. Individual checkboxes are dimmed when the master toggle is off.
- Label columns are ordered after built-in columns and are the first to be dropped by responsive column hiding.
- Label column config is stored as a list of `{label, displayName}` entries, separate from the per-built-in-column boolean keys.
