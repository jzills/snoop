---
name: site
description: Use when changing snoop's website or documentation site (site/: Hugo with Hextra, the landing page, docs pages, styles, the logo or favicons) or its screenshots (demo/shots.sh). Covers the design conventions, how to preview and screenshot the site, and how to re-render the terminal screenshots.
---

# snoop's site

The site is Hugo with the Hextra theme, in `site/` as its own Go module. It
is published to GitHub Pages from `main` by `.github/workflows/pages.yml`,
which also serves `install.sh`. Follow the `writing-docs` skill for every
word on it.

## Layout

| Path | What |
| --- | --- |
| `site/layouts/home.html` | the landing page, as a layout rather than markdown |
| `site/content/docs/` | the docs: overview, install, quickstart, `guides/`, `reference/` |
| `site/assets/css/custom.css` | all custom styling: tokens, navbar logo, landing page, footer, screenshots |
| `site/layouts/_partials/footer.html`, `custom/footer.html` | the footer, on every page |
| `site/assets/snoop-mark.svg` | the logo (two eyes), inlined so it takes the accent color |
| `site/layouts/shortcodes/shot.html` | `{{< shot src="x.png" alt="…" caption="…" >}}` for screenshots |
| `site/static/shots/` | terminal screenshots, rendered by `demo/shots.sh` |
| `site/static/install.sh` | the install script; CI checks the published copy matches |

## Design conventions

- **Plain words, visual depth.** The landing page says what snoop is, shows
  the timeline and links into the docs; detail belongs in the docs. Depth
  comes from a faint hairline grid and accent glow behind the hero,
  screenshots framed as terminal windows, icon cards for the guides, a
  full-width band, a closing panel and sections that ease in on scroll
  (`assets/js/snoop-reveal.js`). It shares the footer and its structure with
  the kx site, but keeps its own look: the grid rather than kx's dots, and
  the blinking eyes.
- **One accent.** The accent (`--snoop-accent`, purple) is the only color:
  the logo, links, buttons, icons, glows and edges. Every motion stops under
  `prefers-reduced-motion`.
- **The footer** (`layouts/_partials/custom/footer.html`) is on every page,
  with the install command, link columns and the author's wordmark. It
  carries no license or trademark notice.
- **System fonts.** No web fonts.
- **The logo is the two eyes** (F4), beside "snoop" in the body font. Don't
  draw the eyes into the word.
- **Hextra does the docs.** Use its sidebar, table of contents, search,
  `cards`, `tabs`, `steps` and `callout` shortcodes rather than custom markup.
- **Real screenshots** from `site/static/shots/`, never mock-ups. Each one
  has alt text and, in the docs, a caption that is a complete sentence.
- Every page works at phone width with no sideways scrolling.

## Preview

With Hugo extended 0.164.0 (`~/.local/bin/hugo` here):

```sh
cd site && hugo server                      # http://localhost:1313/snoop/
cd site && hugo --gc --minify -d /tmp/out   # a build, as CI makes it
```

Check what you changed in a browser before calling it done. Headless
Chromium from vhs works for screenshots. Serve the build under `/snoop/`,
because the base URL has that path:

```sh
mkdir -p /tmp/serve && ln -sfn /tmp/out /tmp/serve/snoop
(cd /tmp/serve && python3 -m http.server 1315 --bind 127.0.0.1 &)
C=$(ls ~/.cache/rod/browser/chromium-*/chrome | head -1)
$C --headless=new --no-sandbox --hide-scrollbars --window-size=1280,2400 \
   --virtual-time-budget=4000 --screenshot=/tmp/home.png http://127.0.0.1:1315/snoop/
```

Add `--force-dark-mode` for the dark theme. Use a window 500 pixels wide to
check the phone layout, since headless Chromium won't go narrower. Look at
light, dark and phone widths for any visual change.

## Screenshots

After any change to how snoop looks, re-render the terminal screenshots:

```sh
demo/shots.sh
```

It builds snoop, seeds fictional sessions with `demo/seed.py`, and records
`demo/shots.tape` and `demo/live.tape` with vhs. It never touches your own
transcripts or cache. Never record against real sessions. The live board's
sessions are seeded just before their shot so their states read correctly.
Look at every image afterwards, since vhs timing occasionally captures the
wrong frame.

## Before committing

- `hugo --gc --minify` builds without warnings.
- The prose linter passes (see the `writing-docs` skill).
- The checks in `pages.yml` would pass: compiled CSS, `install.sh`
  identical to `site/static/install.sh`, `shots/timeline.png` and
  `shots/live.png` present, and `docs/` built.
