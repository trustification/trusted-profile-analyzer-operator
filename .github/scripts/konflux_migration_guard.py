#!/usr/bin/env python3
"""Detect Tekton task migrations in a Konflux/Renovate pull request body.

Renovate lists every bumped dependency in the "This PR contains the following
updates:" table. When a Tekton task bump ships a pipeline migration, that is
called out in the row's Notes cell. Those updates rewrite the pipelines in
.tekton/ and must be reviewed by a human, so they are never automerged nor auto
approved.

Reads the PR body from $PR_BODY and writes "migration=true|false" to
$GITHUB_OUTPUT; the workflow fails the check when it is true.
"""

import os
import re
import sys

MARKER = "This PR contains the following updates:"
NEEDLE = "migration"
SEPARATOR = re.compile(r":?-+:?")


def table_rows(body):
    """Return the markdown rows of the Renovate updates table."""
    lines = body.splitlines()
    start = next((i for i, line in enumerate(lines) if MARKER in line), None)
    if start is None:
        return []

    rows = []
    for line in lines[start + 1:]:
        stripped = line.strip()
        if stripped.startswith("|"):
            rows.append(stripped)
        elif rows:
            break
    return rows


def cells(row):
    """Split a markdown table row into its cells, keeping escaped pipes."""
    values = [value.strip() for value in re.split(r"(?<!\\)\|", row)]
    while values and not values[0]:
        values.pop(0)
    while values and not values[-1]:
        values.pop()
    return values


def column(header, name):
    lowered = [value.lower() for value in header]
    return lowered.index(name) if name in lowered else None


def find_migrations(body):
    """Return the packages whose update notes mention a migration."""
    rows = table_rows(body)
    if not rows:
        return []

    header = cells(rows[0])
    notes_index = column(header, "notes")
    type_index = column(header, "type")

    hits = []
    for row in rows[1:]:
        values = cells(row)
        if not values or all(SEPARATOR.fullmatch(value) for value in values):
            continue
        # Only Tekton task bumps carry pipeline migrations. Tables without a
        # Type column (Package/Change/Notes) are Konflux reference tables.
        if type_index is not None:
            dep_type = values[type_index] if type_index < len(values) else ""
            if "tekton" not in dep_type.lower():
                continue
        if notes_index is not None:
            notes = values[notes_index] if notes_index < len(values) else ""
        else:
            # No Notes column: fall back to the whole row so a renamed or
            # missing column cannot silently let a migration through.
            notes = " ".join(values)
        if NEEDLE in notes.lower():
            hits.append(values[0])
    return hits


def main():
    hits = find_migrations(os.environ.get("PR_BODY", ""))

    output = os.environ.get("GITHUB_OUTPUT")
    if output:
        with open(output, "a", encoding="utf-8") as handle:
            handle.write(f"migration={'true' if hits else 'false'}\n")

    if hits:
        print("Task migrations found in the update notes:")
        for hit in hits:
            print(f"  - {hit}")
    else:
        print("No task migration found in the update notes.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
