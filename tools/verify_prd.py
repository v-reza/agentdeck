#!/usr/bin/env python3
"""Check a PRD suite for the defects a read-through does not catch.

Usage:  python verify_prd.py <prd-directory>

Checks per file:
  * story and acceptance-criteria counts
  * duplicate story IDs
  * gaps in the story ID sequence
  * stories that carry no acceptance criteria
  * broken relative markdown links between PRD files
  * section cross-references (\u00a7N) pointing at a non-existent section

Exit code 0 when clean, 1 when any check fails. Prints one line per finding.
ponytail: regex-based, no markdown parser dependency; good enough for the
conventions in SKILL.md (## N. headings, **US-XNN** story markers).
"""

import os
import re
import sys

STORY = re.compile(r"(?m)^\*\*(US-([A-Z]+)(\d+))\*\*")
AC = re.compile(r"(?m)^- \[ \] AC")
AC_NUM = re.compile(r"(?m)^- \[ \] (AC(\d+))")
HEADING = re.compile(r"(?m)^## (\d+)\.")
SECTION_REF = re.compile(r"\u00a7(\d+)")
MD_LINK = re.compile(r"\]\(([^)#]+\.md)\)")
# Section that owns the story catalogue. ACs found outside it are orphaned —
# they still count in a naive `- [ ] AC` tally, so the tally looks right while
# the story itself has none. That is the failure this check exists to catch.
STORY_SECTION = 6


def check(path, root):
    name = os.path.basename(path)
    text = open(path, encoding="utf-8").read()
    problems = []

    # --- stories and ACs ------------------------------------------------
    chunks = re.split(r"(?m)^\*\*(US-[A-Z]+\d+)\*\*", text)
    ids, ac_counts = [], []
    for i in range(1, len(chunks), 2):
        ids.append(chunks[i])
        body = chunks[i + 1] if i + 1 < len(chunks) else ""
        ac_counts.append(len(AC.findall(body)))

    dupes = sorted({s for s in ids if ids.count(s) > 1})
    if dupes:
        problems.append(f"duplicate story IDs: {', '.join(dupes)}")

    bare = [ids[i] for i, c in enumerate(ac_counts) if c == 0]
    if bare:
        problems.append(f"stories with no acceptance criteria: {', '.join(bare)}")

    # --- AC placement and numbering -------------------------------------
    # Split the file by section heading so we can tell whether each AC block
    # actually sits under its story, inside the story catalogue section.
    parts = re.split(r"(?m)^## (\d+)\.", text)
    body_by_section = {}
    for i in range(1, len(parts), 2):
        body_by_section[int(parts[i])] = parts[i + 1]

    stray = []
    for num, body in body_by_section.items():
        if num == STORY_SECTION:
            continue
        for m in AC_NUM.finditer(body):
            stray.append(f"{m.group(1)} in \u00a7{num}")
    if stray:
        problems.append(
            "acceptance criteria outside the story section "
            f"(\u00a7{STORY_SECTION}): " + ", ".join(stray[:6])
            + (" ..." if len(stray) > 6 else "")
        )

    # AC numbers inside the catalogue must run AC1..ACn without gaps or
    # restarts. A stray renumber shows up here even when the total is right.
    catalogue = body_by_section.get(STORY_SECTION, "")
    bad_seq = []
    for chunk in re.split(r"(?m)^\*\*US-[A-Z]+\d+\*\*", catalogue)[1:]:
        got = [int(m.group(2)) for m in AC_NUM.finditer(chunk)]
        if got and got != list(range(1, len(got) + 1)):
            bad_seq.append(got)
    if bad_seq:
        problems.append(f"AC numbering not sequential in story section: {bad_seq[:4]}")

    # --- ID sequence per (prefix) ---------------------------------------
    by_prefix = {}
    for full, prefix, num in STORY.findall(text):
        by_prefix.setdefault(prefix, []).append(int(num))
    for prefix, nums in sorted(by_prefix.items()):
        expected = set(range(1, max(nums) + 1))
        missing = sorted(expected - set(nums))
        if missing:
            problems.append(
                f"gaps in US-{prefix} sequence: "
                + ", ".join(f"US-{prefix}{n:02d}" for n in missing)
            )

    # --- cross-references -----------------------------------------------
    sections = set(HEADING.findall(text))
    dangling = sorted(set(SECTION_REF.findall(text)) - sections, key=int)
    if dangling:
        S = "\u00a7"
        joined = ", ".join(S + d for d in dangling)
        problems.append(f"{S} refs to missing sections: {joined}")

    for link in MD_LINK.findall(text):
        if not os.path.exists(os.path.join(root, link)):
            problems.append(f"broken link: {link}")

    return name, len(ids), sum(ac_counts), problems


def main(argv):
    root = argv[1] if len(argv) > 1 else "."
    files = sorted(f for f in os.listdir(root) if f.endswith(".md"))
    if not files:
        print(f"no .md files in {root}", file=sys.stderr)
        return 1

    failed = False
    total_s = total_a = 0
    for f in files:
        name, stories, acs, problems = check(os.path.join(root, f), root)
        total_s += stories
        total_a += acs
        flag = "FAIL" if problems else "ok"
        print(f"{flag:4} {name:28} stories={stories:3}  AC={acs:3}")
        for p in problems:
            print(f"       - {p}")
            failed = True

    print(f"\ntotal: {total_s} stories, {total_a} acceptance criteria")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
