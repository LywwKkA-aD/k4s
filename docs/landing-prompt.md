# k4scli.io — landing brief

> Paste this entire file into Claude Design (or any code-capable LLM) as
> a single brief. The output should be a complete, deployable
> **5-page static site** at `docs/site/`. All copy, structure and SEO
> requirements below are mandatory.

---

## Role and goal

You are a senior product designer + frontend engineer + SEO specialist.
Build a **5-page marketing site** for **k4s**, an open-source TUI for
Kubernetes, and ship it ready to deploy on **GitHub Pages** or
**Cloudflare Pages**.

**Domain:** `https://k4scli.io`
**GitHub:** `https://github.com/LywwKkA-aD/k4s`
**Demo asset:** `docs/demo.gif` (1280×720, ~2 minutes, already in the repo)

The site must rank in the **top 5–10 of Google** for these queries within
3 months:

- `k4s`, `k4scli`, `k4s cli`
- `k9s alternative`, `alternative to k9s`, `k9s vs k4s`
- `kubernetes tui`, `k8s tui`, `k3s tui`
- `kubectl tui`, `kubectl gui terminal`, `terminal kubernetes manager`
- `kubernetes cli ui`, `k8s terminal ui`

The main competitor is **k9scli.io** (derailed/k9s). We compete *honestly*:
mention them, link them, position k4s as a **kubectl-aware companion**, not
a replacement.

### Why 5 pages and not 1

A short landing + dedicated sub-pages is the right choice here:

- **More indexable URLs** — every sub-page targets its own keyword cluster
  and earns its own backlinks.
- **Cleaner UX** — no infinite-scroll "wall of text".
- **Better Google relevance signals** — `/vs-k9s` is laser-focused on
  `k9s alternative`, `/faq` is laser-focused on long-tail questions.
- **Easier to maintain** — each page is independently editable.

---

## Site map

```
/                hero, features grid, quickstart, CTA
/why-k4s/        the "why I built it" story + the kubectl-by-osmosis idea
/vs-k9s/         comparison table k4s vs k9s vs lazykube vs raw kubectl
/faq/            FAQ — long-tail SEO catcher, schema.org FAQPage
/changelog/      version history + public roadmap
```

The navigation is identical on every page (sticky top):

```
k4s · Why · vs k9s · FAQ · Changelog · ★ GitHub
```

The active link is rendered with `--lavender` and a soft underline.

---

## Product summary

**k4s** is a fast, opinionated TUI for Kubernetes (k8s & k3s) written in
Go on top of Charm's Bubble Tea framework.

The headline differentiator: **every keystroke prints its `kubectl`
equivalent in the footer.** k4s is a TUI *and* a kubectl tutor — users
build CLI muscle memory while they work. We call this **kubectl by
osmosis**.

**MVP feature set (already shipped):**

- Pods, namespaces, deployments, services — list views
- `describe` view in kubectl style with events
- Multi-pod streaming logs with per-pod FNV colour tag, search,
  smart auto-follow, two-step clear
- `exec` into a pod via `kubectl exec`
- `top` for pods and nodes via the metrics.k8s.io REST API
- Live `/` substring filter on every list view
- Kubeconfig context switcher
- 5s auto-refresh (`w` toggle, footer shows `--watch`)
- Centred help popup on `?`
- Footer hints with the `kubectl` equivalent of every shortcut

**Roadmap (in order):**

1. Resource actions — delete pod / scale deployment / rollout restart
2. YAML view with chroma syntax highlighting
3. Events view (cluster-wide and per-resource)
4. "All containers" option in the logs picker
5. ConfigMaps and Secrets list views
6. Homebrew tap (`brew install LywwKkA-aD/tap/k4s`)
7. Linux deb/rpm packages

---

## Visual direction — Charm-style soft dark

This is **non-negotiable**. The site must feel like an extension of the
TUI itself. Reject any temptation to use generic SaaS gradients.

### Palette (Catppuccin Mocha + Charm purple)

```
--bg-base        #1E1E2E   # body background
--bg-mantle      #181825   # cards, code blocks
--bg-crust       #11111B   # deepest layer (footer)
--text           #CDD6F4   # primary text
--text-muted     #A6ADC8   # secondary text
--lavender       #B4BEFE   # primary accent (k4s brand)
--mauve          #CBA6F7   # hover / secondary accent
--blue           #89B4FA   # links
--green          #A6E3A1   # success / "kubectl ✅"
--peach          #FAB387   # warnings, "vs k9s" highlights
--red            #F38BA8   # errors, "❌" cells
--surface-glow   rgba(180, 190, 254, 0.08)  # soft halo behind cards
```

### Typography

- **Headings:** `JetBrains Mono` (700) — monospace, technical, matches the
  TUI. Loaded via Google Fonts with `font-display: swap`.
- **Body:** `Inter` (400/500/600) — clean, neutral, readable.
- **Code:** `JetBrains Mono` (400/500).

### Layout principles

- **Floating terminal mockups** as the visual hero — frame `docs/demo.gif`
  inside a stylised terminal window (red/yellow/green dots top-left,
  rounded corners, soft outer glow in `--surface-glow`).
- **Soft glow** behind feature cards (radial gradient on hover).
- **Generous whitespace** — sections breathe, don't cram.
- **Scroll-driven micro-animations** — fade-up on enter (CSS
  `@scroll-timeline` or simple `IntersectionObserver` with
  `transition: opacity .4s, transform .4s`).
- **No emojis in headings.** Use Lucide icons (inline SVG) instead.
- **No stock photos.** Everything is monospace text, terminal mockups,
  and abstract gradients.

### Responsive breakpoints

- **Mobile** ≤ 640px: single column, compressed terminal mockup, hero GIF
  at 90vw, sticky bottom CTA bar with "GitHub →" and "Install".
- **Tablet** 641–1024px: two-column for features grid, terminal mockup
  scales to 80vw.
- **Desktop** ≥ 1025px: full-width hero (max-w-7xl), three-column features
  grid, side-by-side comparison table on `/vs-k9s/`.

The site **must** pass Lighthouse 100/100/100/100 on
performance / accessibility / best practices / SEO **on every page**.

### Shared components (used across pages)

1. **TopNav** — sticky, blur backdrop. Logo, link list, GitHub button
   with star count from `https://img.shields.io/github/stars/LywwKkA-aD/k4s?style=flat`.
2. **Footer** — three columns (project / docs / author), MIT badge,
   "Built with Bubble Tea + Go" line, links to k9s and lazykube as
   "kindred projects" (good for backlink reciprocity).
3. **TerminalMockup** — wrapper for any image with the three-dot chrome.
4. **CodeBlock** — `<pre><code>` with copy-to-clipboard button.
5. **CTABanner** — bottom-of-page strip with `Try k4s now` + GitHub link
   + install snippet. Reused on every sub-page.

---

## Page 1 — `/` (home)

**`<title>`** — `k4s — kubectl-aware TUI for Kubernetes & k3s | k4scli.io`
**Meta description** — `Open-source terminal UI for Kubernetes. Every keystroke prints its kubectl equivalent in the footer. A k9s alternative that teaches kubectl while you work.`
**Canonical** — `https://k4scli.io/`
**JSON-LD** — `SoftwareApplication` (full payload, see SEO block below)

### Sections

#### 1.1 Hero

**Headline (h1):**

> The kubectl-aware TUI for Kubernetes & k3s.

**Sub-headline (paragraph):**

> k4s is a fast terminal UI for Kubernetes. Every keystroke prints the
> `kubectl` equivalent in the footer — build CLI muscle memory while you
> work. Free, open-source, single binary, MIT-licensed.

**Primary CTA:** `View on GitHub` → `https://github.com/LywwKkA-aD/k4s`
**Secondary CTA:** copy `go install github.com/LywwKkA-aD/k4s/cmd/k4s@latest`

**Visual:** `docs/demo.gif` framed in a terminal mockup, autoplay on
loop, lazy-loaded with a low-quality placeholder. Width responsive,
max 1100px.

Caption beneath: *"Real recording. The footer shows `≈ kubectl logs -f -c
spammer log-spammer-… -n k4s-demo --tail=100` while you stream logs."*

Three small links beneath the visual:
`Why I built it →` `/why-k4s/`
`How does it compare to k9s →` `/vs-k9s/`
`FAQ →` `/faq/`

#### 1.2 Features grid (9 cards, 3×3 desktop, 1-col mobile)

Each card: lucide icon + monospace title + 2-line description.

1. **Multi-pod log streaming** — Stream every replica at once with
   per-pod FNV colour tags, search, smart auto-follow, two-step clear.
2. **Live filter** — `/` substring filter on any list view. Works in
   pods, namespaces, deployments, services, contexts.
3. **kubectl footer hints** — Every shortcut prints its CLI equivalent.
   Learn `kubectl` by osmosis.
4. **Describe view** — kubectl-style sections including events,
   scrollable, syntax-aware.
5. **Exec** — Press `e`, drop into the pod shell via `tea.ExecProcess`.
   No reconnect dance.
6. **Top** — `kubectl top pods/nodes` via the metrics.k8s.io REST API.
   No extra dependency on `k8s.io/metrics`.
7. **Context switcher** — `:ctx` jumps between kubeconfig contexts. Drops
   stale navigation history automatically.
8. **Watch** — 5s auto-refresh on every list view. Footer shows
   `--watch`. Toggle with `w`.
9. **Centred help popup** — `?` lists global, view-specific and command
   bar bindings. No memorising the cheat sheet.

#### 1.3 Quickstart (three CSS-only tabs)

**macOS / Linux (Go installed):**
```bash
go install github.com/LywwKkA-aD/k4s/cmd/k4s@latest
k4s
```

**From source:**
```bash
git clone https://github.com/LywwKkA-aD/k4s
cd k4s
make demo                    # local k3s + seed data
KUBECONFIG=$(pwd)/.kube/config make run
```

**Homebrew (coming soon):**
```bash
# brew install LywwKkA-aD/tap/k4s   # planned for v0.2
```

Each code block has a copy-to-clipboard button (small, top-right corner,
icon-only `lucide-copy`). Hover state shows the lavender accent.

#### 1.4 CTABanner — `Try k4s now`

---

## Page 2 — `/why-k4s/` (story)

**`<title>`** — `Why I built k4s — a kubectl-aware TUI for Kubernetes`
**Meta description** — `The story behind k4s. Why I built another Kubernetes TUI when k9s already existed. The kubectl-by-osmosis design philosophy.`
**Canonical** — `https://k4scli.io/why-k4s/`
**JSON-LD** — `Article` (headline, author, datePublished, image: og.png)

### Sections

#### 2.1 Hero — small, no GIF

Single column, max-w-3xl, centered.

**h1:** `Why I built k4s`
**Lead paragraph (italic, --text-muted):**
> k9s is a great tool. So is lazykube. So is `kubectl` itself. So why
> another Kubernetes TUI?

#### 2.2 The story (long-form, max 800 words)

Voice: first-person, direct, no fluff. Themes:

- I love k9s but it doesn't *teach* kubectl — you stay locked in the TUI
  forever.
- I wanted a hybrid: full TUI for speed, but every shortcut surfaces the
  underlying `kubectl` so I leave each session knowing one more command.
- Bubble Tea is a joy to write in. The Action abstraction makes every
  operation auditable — a future where every Action emits an audit
  event is on the roadmap.
- k4s is opinionated. It picks what to expose, when to flash a
  `--watch`, when to drop you back to home. The opinion *is* the
  product.
- Made for k3s and k8s alike. Footers stay the same.

End with: *"k4s won't replace k9s. It's for the people who use a TUI but
also want to leave with sharper kubectl reflexes."*

#### 2.3 The "kubectl by osmosis" idea — a small terminal mockup

Inside a TerminalMockup, render this snippet:

```
NAMESPACE   NAME                   READY   STATUS    RESTARTS   AGE
k4s-demo    nginx-58bcfc684b-bcqpk 1/1     Running   0          14h
k4s-demo    nginx-58bcfc684b-r56bp 1/1     Running   0          14h

q quit · ^c quit · esc back · : command · ? help · enter describe · l logs
≈ kubectl get pods -n k4s-demo --watch
```

Caption: *"Press `:ns` and the footer flashes `kubectl get namespaces`.
Press `l` and the footer flashes `kubectl logs -f`. After a week, you
just know."*

#### 2.4 CTABanner

---

## Page 3 — `/vs-k9s/` (comparison)

**`<title>`** — `k4s vs k9s — the honest comparison | k4scli.io`
**Meta description** — `An honest comparison of k4s, k9s, lazykube, and raw kubectl. Pick the right Kubernetes TUI for your workflow.`
**Canonical** — `https://k4scli.io/vs-k9s/`
**JSON-LD** — `Article` + `Table` (microdata for the comparison table)

### Sections

#### 3.1 Hero

**h1:** `k4s vs k9s — the honest comparison`
**Lead:** `Both are open-source, both are written in Go, both are excellent. Here's how they actually differ.`

#### 3.2 The comparison table (real semantic HTML)

Use `<table>` with `<thead>` / `<tbody>` and `scope="col"` / `scope="row"`
for accessibility + SEO.

| Capability                            | k4s          | k9s         | lazykube    | kubectl  |
| ------------------------------------- | ------------ | ----------- | ----------- | -------- |
| Single static binary                  | ✅           | ✅          | ✅          | ✅       |
| Multi-pod log streaming               | ✅           | ✅          | ✅          | manual   |
| Live `/` filter on every view         | ✅           | ✅          | partial     | grep     |
| **`kubectl` equivalent in footer**    | **✅ unique**| ❌          | ❌          | n/a      |
| Container picker for exec / logs      | ✅           | ✅          | partial     | manual   |
| Auto-refresh watch                    | ✅           | ✅          | ✅          | `-w`     |
| Built-in `top` (metrics.k8s.io)       | ✅           | ✅          | ❌          | ✅       |
| kubeconfig context switcher           | ✅           | ✅          | ✅          | ✅       |
| Action abstraction (auditable verbs)  | ✅           | ❌          | ❌          | n/a      |
| Maturity                              | early        | mature      | mature      | very mature |
| Resource actions (delete/scale)       | roadmap      | ✅          | ✅          | ✅       |
| Plugin / extension system             | ❌           | ✅          | ❌          | ✅       |
| MIT licensed                          | ✅ MIT       | Apache 2.0  | MIT         | Apache 2.0 |
| Footprint (binary)                    | ~38 MB       | ~50 MB      | ~25 MB      | varies   |

Caption: *"Both k9s and lazykube are excellent — k4s exists for the niche
where 'I want a TUI **and** I want to learn kubectl' overlap."*

#### 3.3 When to pick each

Three side-by-side cards.

- **Pick k9s if** — you want the broadest feature set, plugins, and
  years of battle-testing. Production teams running 50+ clusters.
- **Pick lazykube if** — you want the smallest binary and a "lazy"
  ergonomic style à la `lazygit`. Solo dev workflows.
- **Pick k4s if** — you want a TUI that doubles as a `kubectl` tutor.
  You're learning the API or training new joiners. You like opinionated
  software.

Each card links out honestly:
- `→ k9scli.io`
- `→ github.com/jesseduffield/lazykube`
- `→ github.com/LywwKkA-aD/k4s`

#### 3.4 CTABanner

---

## Page 4 — `/faq/` (long-tail SEO)

**`<title>`** — `k4s FAQ — Kubernetes TUI questions answered | k4scli.io`
**Meta description** — `Frequently asked questions about k4s — installation, k9s comparison, k3s support, kubectl requirements, kubeconfig contexts, licensing.`
**Canonical** — `https://k4scli.io/faq/`
**JSON-LD** — `FAQPage` (every Q below as a `Question` entity, see SEO block)

### Sections

#### 4.1 Hero

**h1:** `Frequently asked questions`
**Lead:** `Common questions about k4s, its relationship with k9s, and how to install it.`

#### 4.2 Categorised accordion

Use `<details>` / `<summary>` for native, JS-free accordion. Group by
heading; emit one `Question` JSON-LD entry per item.

**About k4s**

1. **What is k4s?** — k4s is an open-source terminal UI (TUI) for
   Kubernetes and k3s, written in Go. It surfaces the `kubectl`
   equivalent of every shortcut in the footer so users can learn the CLI
   while they work.
2. **What does k4s mean?** — k4s is a play on the existing Kubernetes
   TUIs (`k9s`, `k3s`). The "4" has no formal meaning beyond rhyming —
   the project leans into "kubectl by osmosis" as its real identifier.
3. **Is k4s free and open source?** — Yes. MIT-licensed. Full source
   at <https://github.com/LywwKkA-aD/k4s>. Contributions welcome.

**k4s vs k9s**

4. **Is k4s the same as k9s?** — No. k9s (<https://k9scli.io>) is a
   mature TUI for Kubernetes that focuses on speed and breadth of
   features. k4s is a smaller, opinionated tool built around the idea of
   teaching `kubectl` through use. They can coexist.
5. **What's the difference between k4s and k9s?** — The biggest
   difference is the kubectl footer hints — k4s prints the underlying
   `kubectl` command for every keystroke. k9s does not. k4s is also
   younger, has a narrower feature set, and is MIT licensed.
6. **Is k4s a fork of k9s?** — No. k4s is written from scratch on
   Bubble Tea. It shares no code with k9s.
7. **Should I switch from k9s to k4s?** — Not unless you specifically
   want the `kubectl`-teaching angle. k9s is more mature. k4s is for
   people learning Kubernetes or training new joiners.

**Installation**

8. **How do I install k4s on macOS?** — `go install
   github.com/LywwKkA-aD/k4s/cmd/k4s@latest` — you'll need Go 1.26+.
   A Homebrew tap is on the roadmap.
9. **How do I install k4s on Linux?** — Same `go install` command.
   Linux `.deb` and `.rpm` packages are on the roadmap.
10. **Does k4s work on Windows?** — Untested. Bubble Tea supports
    Windows terminals, but k4s has only been verified on macOS and Linux.
11. **Does k4s require kubectl on PATH?** — Only for `exec` and the
    "coming soon" Homebrew install. Pure TUI usage (lists, describe,
    logs, top) goes through `client-go` directly and does not need
    `kubectl` installed.

**Compatibility**

12. **Can I use k4s with k3s?** — Yes. k4s works against any cluster
    reachable via kubeconfig — k3s, k8s, EKS, GKE, AKS, Kind, Minikube,
    Rancher Desktop, OrbStack, anything that speaks the Kubernetes API.
13. **Does k4s support multiple kubeconfig contexts?** — Yes. Press
    `:ctx` to switch between contexts. k4s drops stale navigation history
    when you switch — list views are rebuilt against the new cluster.
14. **Which Kubernetes versions does k4s support?** — k4s uses
    `client-go` v0.36, which supports Kubernetes API versions 1.27
    through 1.30. Newer and older versions usually work but are not
    explicitly tested.

**Design**

15. **What is "kubectl by osmosis"?** — It's the design idea behind k4s.
    Every TUI action shows you the `kubectl` command it would have run
    in the footer. Use the TUI for speed; absorb the CLI as a
    side-effect.
16. **Why is the binary 38 MB?** — Most of it is `client-go` and its
    transitive dependencies. Static Go binaries trade size for zero
    runtime requirements.

#### 4.3 CTABanner

---

## Page 5 — `/changelog/` (versions + roadmap)

**`<title>`** — `k4s changelog & roadmap — release history | k4scli.io`
**Meta description** — `Release notes and public roadmap for k4s, the kubectl-aware Kubernetes TUI. Resource actions, YAML view, events view and more coming soon.`
**Canonical** — `https://k4scli.io/changelog/`
**JSON-LD** — `Article` (latest release as a representative entry)

### Sections

#### 5.1 Hero

**h1:** `Changelog & roadmap`
**Lead:** `Release notes for k4s and the public list of what's coming next.`

#### 5.2 Version timeline

Vertical timeline (left rail with dots in `--lavender`).

```
2026-05-08  v0.1 release candidate
            ─ help popup on '?'
            ─ live '/' filter on every list view
            ─ kubeconfig context switcher
            ─ kubectl top pods / nodes view

2026-05-07  MVP feature complete
            ─ pods, namespaces, deployments, services
            ─ kubectl-style describe + events
            ─ multi-pod log streaming
            ─ container picker, exec, auto-refresh

2026-05-06  Project scaffold
            ─ initial Bubble Tea TUI shell
            ─ quality gate (golangci-lint v2 + govulncheck)
            ─ docker-compose k3s + seed manifests
```

#### 5.3 Roadmap — card grid (4 cards)

1. **Resource actions** — delete pod, scale deployment, rollout restart.
2. **YAML view** — raw manifest with chroma syntax highlighting.
3. **Events view** — cluster-wide and per-resource.
4. **ConfigMaps & Secrets** — read-only list views first.

Each card has a `lucide-circle-dashed` icon; on hover the icon swaps to
`lucide-circle-dot` with the `--lavender` accent.

#### 5.4 GitHub releases link

A small panel at the bottom: *"Subscribe to GitHub Releases for
machine-readable changelogs:"* with a link to
`https://github.com/LywwKkA-aD/k4s/releases.atom`.

#### 5.5 CTABanner

---

## SEO requirements (mandatory, per page)

### Per-page `<head>` (use the title/description/canonical/JSON-LD
specified in each page block above)

```html
<meta name="robots" content="index, follow, max-image-preview:large">
<meta name="theme-color" content="#1E1E2E">
<meta name="author" content="LywwKkA-aD">
```

### Open Graph + Twitter (per page, with page-specific title/description
and image)

```html
<meta property="og:type" content="website">
<meta property="og:title" content="{page-specific}">
<meta property="og:description" content="{page-specific}">
<meta property="og:url" content="{page-specific canonical}">
<meta property="og:image" content="https://k4scli.io/assets/og.png">
<meta property="og:image:width" content="1200">
<meta property="og:image:height" content="630">
<meta property="og:site_name" content="k4s">

<meta name="twitter:card" content="summary_large_image">
<meta name="twitter:title" content="{page-specific}">
<meta name="twitter:description" content="{page-specific}">
<meta name="twitter:image" content="https://k4scli.io/assets/og.png">
```

You must produce **one** `assets/og.png` (1200×630) used across all
pages. Lavender on Catppuccin mantle, large `k4s` wordmark, tagline,
terminal mockup snippet.

### Cross-page linking (internal link graph)

Every page links to at least three other pages from inline body copy
(not just nav). This builds a tight internal link graph that helps
PageRank flow:

- `/` links to `/why-k4s/`, `/vs-k9s/`, `/faq/`
- `/why-k4s/` links to `/`, `/vs-k9s/`, `/faq/`
- `/vs-k9s/` links to `/`, `/why-k4s/`, `/faq/`
- `/faq/` links to `/`, `/why-k4s/`, `/vs-k9s/`, `/changelog/`
- `/changelog/` links to `/`, `/faq/`

External links to k9scli.io and github.com/jesseduffield/lazykube use
`rel="noopener"` (not `nofollow` — we want reciprocal goodwill).

### `robots.txt` (at site root)

```
User-agent: *
Allow: /
Sitemap: https://k4scli.io/sitemap.xml
```

### `sitemap.xml` — five entries

```xml
<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>https://k4scli.io/</loc>           <changefreq>weekly</changefreq>  <priority>1.0</priority></url>
  <url><loc>https://k4scli.io/why-k4s/</loc>   <changefreq>monthly</changefreq> <priority>0.8</priority></url>
  <url><loc>https://k4scli.io/vs-k9s/</loc>    <changefreq>monthly</changefreq> <priority>0.9</priority></url>
  <url><loc>https://k4scli.io/faq/</loc>       <changefreq>monthly</changefreq> <priority>0.8</priority></url>
  <url><loc>https://k4scli.io/changelog/</loc> <changefreq>weekly</changefreq>  <priority>0.7</priority></url>
</urlset>
```

### JSON-LD payloads

**`SoftwareApplication`** (on `/` only):

```json
{
  "@context": "https://schema.org",
  "@type": "SoftwareApplication",
  "name": "k4s",
  "alternateName": ["k4scli", "k4s cli"],
  "applicationCategory": "DeveloperApplication",
  "operatingSystem": "macOS, Linux",
  "description": "Open-source TUI for Kubernetes and k3s. Every keystroke prints its kubectl equivalent in the footer. A k9s alternative.",
  "offers": { "@type": "Offer", "price": "0", "priceCurrency": "USD" },
  "softwareVersion": "0.1",
  "license": "https://opensource.org/licenses/MIT",
  "url": "https://k4scli.io/",
  "downloadUrl": "https://github.com/LywwKkA-aD/k4s/releases",
  "author": {
    "@type": "Person",
    "name": "LywwKkA-aD",
    "url": "https://github.com/LywwKkA-aD"
  },
  "programmingLanguage": "Go",
  "keywords": "kubernetes, k3s, k8s, tui, kubectl, terminal, bubble tea"
}
```

**`FAQPage`** (on `/faq/` only) — emit one `Question` entity per FAQ
item with the exact verbatim question text and the answer text.

**`Article`** (on `/why-k4s/`, `/vs-k9s/`, `/changelog/`) — basic
headline / author / datePublished / image.

**`BreadcrumbList`** (on every sub-page) — `Home → {page name}`.

### Performance budget (per page)

- LCP < 1.5s on 4G (lazy-load `demo.gif`, preconnect to fonts.gstatic).
- CLS < 0.05 (set width/height on every image, including `demo.gif`).
- Total JS < 5 KB. **No React, no Vue, no client framework.** A tiny
  vanilla snippet for copy buttons + mobile nav is fine.
- Total CSS (Tailwind purged) < 20 KB.
- Inline critical CSS, async-load the rest.

### Accessibility

- WCAG AA contrast on every text/background pair (Catppuccin lavender
  on Mocha base passes — verify yourself with Stark or a similar tool).
- Every image has `alt`. The hero GIF: `alt="Recording of k4s, a TUI
  for Kubernetes, showing pod lists, log streaming, describe and top
  views."`
- Tab order matches visual order. Skip-to-main link on every page.
- `prefers-reduced-motion` disables auto-playing GIF (replace with
  static first frame `assets/demo-poster.png`).

---

## Tech and delivery

### File layout

```
docs/site/                  # publish this directory to Pages
  index.html                # /
  why-k4s/index.html        # /why-k4s/
  vs-k9s/index.html         # /vs-k9s/
  faq/index.html            # /faq/
  changelog/index.html      # /changelog/
  assets/
    demo.gif                # symlink or copy of docs/demo.gif
    demo-poster.png         # first frame, lazy-load placeholder
    og.png                  # 1200×630 social card (shared)
    favicon.svg             # lavender 'k4s' on mocha
    favicon-32.png          # fallback
  robots.txt
  sitemap.xml
  404.html                  # styled like the rest, link back to /
  CNAME                     # contains: k4scli.io
  .nojekyll                 # empty file, disables Jekyll on Pages
```

### Stack

- **Tailwind CSS via CDN** during prototyping;
  for production, run `tailwindcss --minify` against the HTML files
  (config inline at `<script>`-level is acceptable).
- **No JS framework.** Vanilla ES2022 only. One small script (≤ 80 LOC)
  shared across pages: copy-to-clipboard, mobile nav toggle,
  IntersectionObserver fade-up.
- **Fonts:** preconnect to fonts.gstatic.com, load JetBrains Mono +
  Inter with `font-display: swap` and `crossorigin`.

### Shared HTML chunks

To keep things DRY in a static-HTML setup, extract these chunks into
shared snippets that you paste verbatim into each page:

- `<head>` boilerplate (fonts, viewport, theme-color)
- `TopNav` component
- `Footer` component
- `CTABanner` component
- One small `<script>` for clipboard / mobile nav / IntersectionObserver

If using Astro or 11ty for the build, these become components/partials.
If pure HTML, copy-paste is acceptable — there are only 5 files.

### Deployment

The output should be deployable to **GitHub Pages** by pointing the
Pages source at `docs/site/` on `main`. CNAME contains `k4scli.io`.
`.nojekyll` is empty.

For Cloudflare Pages: build command `none`, output directory
`docs/site/`. Add the custom domain `k4scli.io` in the dashboard.

Provide:

1. All five `index.html` files (formatted, commented at section
   boundaries).
2. `robots.txt`, `sitemap.xml`, `404.html`, `CNAME`, `.nojekyll`.
3. Both `assets/og.png` and `assets/favicon.svg`.
4. A short note in the chat (≤ 200 words) explaining how to deploy and
   any known trade-offs.

Do **not** invent screenshots. Use only `docs/demo.gif` as the visual
asset and the comparison table for visual variety.

---

## Tone of voice

- Direct, technical, dry. No "supercharge your workflow", no "level up
  your DevOps".
- Lowercase headlines where stylistically appropriate (terminal-y).
- First-person singular allowed in `/why-k4s/` only.
- Mention competitors by name and link to them. Confidence > defensiveness.
- Never claim k4s is "the best" — claim it is *opinionated*.

---

## Non-goals

- **No** signup, no email capture, no newsletter modal, no popup,
  no cookies banner (we set zero cookies).
- **No** analytics — privacy is a brand value here. (If you must,
  use Cloudflare's privacy-respecting analytics, no client-side script.)
- **No** "made with [tool]" badges that link to anything other than
  Bubble Tea / Go.
- **No** dark/light toggle. The site is dark, full stop. (Catppuccin
  Mocha is non-negotiable.)
- **No** AI-generated illustration. Terminal mockups + the real
  demo.gif only.

---

## Acceptance checklist (verify before delivering)

- [ ] All 5 pages return 200, every internal link resolves
- [ ] Lighthouse 100 across all four scores on every page (4G throttle)
- [ ] WCAG AA contrast on every text/bg pair
- [ ] Schema.org JSON-LD validates on every page (Google Rich Results
      Test) — `SoftwareApplication`, `FAQPage`, `Article`,
      `BreadcrumbList` as specified
- [ ] OG card renders correctly in Slack, Twitter, LinkedIn, Discord
- [ ] Site is fully functional with JavaScript disabled
- [ ] `prefers-reduced-motion` respected
- [ ] `sitemap.xml` validates and lists all 5 pages
- [ ] `robots.txt` references the sitemap
- [ ] `CNAME` and `.nojekyll` present
- [ ] `docs/demo.gif` referenced with explicit width/height and
      `loading="lazy"`
- [ ] Internal link graph: every page links to ≥ 3 others from body copy
- [ ] No `console.log`, no commented-out code, no TODOs in shipped HTML

---

When the brief is unclear, default to **less is more**. A site that is
plain, fast and honest will outrank a busy one. We are not here to
imitate Vercel; we are here to ship a tool that respects the user's
time.
