#!/usr/bin/env python3
"""Check snoop's user-facing prose against the project's writing rules.

    .claude/hooks/prose_lint.py FILE...      # report problems, exit 1 if any
    .claude/hooks/prose_lint.py --hook       # PostToolUse hook: reads the tool call on stdin

The rules (see .claude/skills/writing-docs/SKILL.md):

- No sentence starts with a code span: a command, flag, key or path.
- No sentence starts with "snoop".
- American spelling.

It reads the README, the site's content and its landing page. Code blocks,
tables, headings and front matter are skipped; captions and card subtitles
in shortcodes are checked as sentences. Whether a sentence is complete can't
be checked mechanically, so the skill covers that.
"""

import json
import os
import re
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

# Files the rules apply to, relative to the repository root.
def applies(rel):
    return (rel == "README.md"
            or (rel.startswith("site/content/") and rel.endswith(".md"))
            or rel == "site/layouts/home.html")

BRITISH = {
    "colour": "color", "colours": "colors", "coloured": "colored",
    "behaviour": "behavior", "favour": "favor", "favourite": "favorite",
    "recognise": "recognize", "recognised": "recognized", "recognises": "recognizes",
    "summarise": "summarize", "summarised": "summarized", "summarises": "summarizes",
    "organise": "organize", "organised": "organized", "initialise": "initialize",
    "customise": "customize", "customised": "customized", "prioritise": "prioritize",
    "labelled": "labeled", "labelling": "labeling", "cancelled": "canceled",
    "cancelling": "canceling", "travelled": "traveled", "modelled": "modeled",
    "centre": "center", "licence": "license", "catalogue": "catalog",
    "grey": "gray", "whilst": "while", "amongst": "among", "learnt": "learned",
    "analyse": "analyze", "analysed": "analyzed",
}

ATTR_SENTENCES = re.compile(r'\b(?:caption|subtitle)="([^"]*)"')


def prose_blocks(text, html):
    """Yields (line number, paragraph text) for each run of prose."""
    if html:
        # Keep code spans as backticks so a leading command is still seen.
        text = re.sub(r"<code[^>]*>(.*?)</code>", r"`\1`", text, flags=re.S)
        text = re.sub(r"\{\{.*?\}\}", " ", text, flags=re.S)
        text = re.sub(r"<!--.*?-->|<svg.*?</svg>|<script.*?</script>", " ", text, flags=re.S)
        text = re.sub(r"<(p|h[1-6]|li|figcaption|div|section)\b[^>]*>", "\n\n", text)
        text = re.sub(r"<[^>]+>", " ", text)
        text = re.sub(r"&rsquo;|&#39;", "'", text).replace("&amp;", "&")
    lines = text.split("\n")
    block, start, fence, front, raw = [], 0, False, False, False
    for i, line in enumerate(lines, 1):
        s = line.strip()
        if i == 1 and s == "---":
            front = True
            continue
        if front:
            front = s != "---"
            continue
        if s.startswith("```"):
            fence = not fence
            if block:
                yield start, " ".join(block)
            block = []
            continue
        # Markdown passes a block of raw HTML through until a blank line.
        if not html and s.startswith("<") and not raw:
            raw = True
        elif not s:
            raw = False
        skip = (fence or raw or not s or s.startswith(("#", "|", "{{", "<", "![", "-->"))
                or (html and s.startswith(("{{", "$"))))
        if skip:
            if block:
                yield start, " ".join(block)
            block = []
            for m in ATTR_SENTENCES.finditer(line):
                yield i, m.group(1)
            continue
        # A list item starts a new block.
        item = re.match(r"^(?:[-*]|\d+\.)\s+(.*)", s)
        if item:
            if block:
                yield start, " ".join(block)
            block, start = [item.group(1)], i
            continue
        if not block:
            start = i
        block.append(s)
    if block:
        yield start, " ".join(block)


def sentences(paragraph):
    # A bold lead-in such as "**Timeline:**" labels the paragraph; the
    # sentence starts after it.
    paragraph = re.sub(r"^\*\*[^*]+:\*\*\s*", "", paragraph)
    # Split after sentence punctuation, but not inside code spans.
    parts, buf, in_code = [], "", False
    for i, ch in enumerate(paragraph):
        buf += ch
        if ch == "`":
            in_code = not in_code
        elif ch in ".!?" and not in_code and (i + 1 == len(paragraph) or paragraph[i + 1] == " "):
            parts.append(buf.strip())
            buf = ""
    if buf.strip():
        parts.append(buf.strip())
    return parts


def check(rel, text):
    problems = []
    html = rel.endswith(".html")
    for line, para in prose_blocks(text, html):
        for s in sentences(para):
            # A command on its own, such as an install line, isn't a sentence.
            if not re.search(r"[A-Za-z]", re.sub(r"`[^`]*`", "", s)):
                continue
            lead = s.lstrip("*_([\"'")
            if lead.startswith("`"):
                problems.append((line, "starts with a code span; begin with a word", s))
            elif re.match(r"snoop\b", lead):
                problems.append((line, 'starts with "snoop"; begin with another word', s))
        for word in re.findall(r"[A-Za-z]+", re.sub(r"`[^`]*`|\([^)]*\)\]?|https?://\S+", "", para)):
            if word.lower() in BRITISH:
                problems.append((line, f'British spelling "{word}"; use "{BRITISH[word.lower()]}"', ""))
    return problems


def report(rel, problems):
    out = []
    for line, what, s in problems:
        snippet = (s[:90] + "…") if len(s) > 90 else s
        out.append(f"{rel}:{line}: {what}" + (f": {snippet}" if snippet else ""))
    return out


def main():
    if sys.argv[1:] == ["--hook"]:
        call = json.load(sys.stdin)
        path = call.get("tool_input", {}).get("file_path") or call.get("tool_response", {}).get("filePath") or ""
        rel = os.path.relpath(os.path.abspath(path), ROOT)
        if not applies(rel) or not os.path.exists(path):
            return 0
        lines = report(rel, check(rel, open(path, encoding="utf-8").read()))
        if lines:
            print(json.dumps({
                "decision": "block",
                "reason": "The edit breaks snoop's writing rules (.claude/skills/writing-docs/SKILL.md). "
                          "Rewrite these sentences:\n" + "\n".join(lines),
            }))
        return 0
    paths = sys.argv[1:]
    if not paths:
        print(__doc__.strip().split("\n\n")[1], file=sys.stderr)
        return 2
    found = []
    for p in paths:
        rel = os.path.relpath(os.path.abspath(p), ROOT)
        found += report(rel, check(rel, open(p, encoding="utf-8").read()))
    print("\n".join(found) if found else "ok")
    return 1 if found else 0


if __name__ == "__main__":
    sys.exit(main())
