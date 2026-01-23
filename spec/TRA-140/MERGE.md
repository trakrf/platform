# TRA-140 Mockup Merge Resolution

Parallel Claude instances worked on mockups simultaneously. This document tracks what each did to help consolidate.

---

## Instance 1 (Claude - "mocky" assignment)

**Assigned directory:** `spec/TRA-140/mocky/`
**Actual directory used:** `spec/TRA-140/mocks/` (mistake - used wrong name)

### Files Created

**Markdown specs in `spec/TRA-140/mocks/`:**
- `mockup-a-tabbed-reports.md` - Tabbed reports page, table-based history
- `mockup-b-inline-drilldown.md` - Updated to visual timeline approach
- `mockup-c-split-panel.md` - Split panel master-detail with hybrid table+timeline

**HTML mockups:** Created but pushed to separate branch for GitHub Pages

### GitHub Pages Setup

Created branch `mockups/tra-140-reports` with HTML files at root:
- `mockup-b1-inline-table.html`
- `mockup-b2-inline-timeline.html`
- `mockup-c1-split-table.html`
- `mockup-c2-split-timeline.html`

Enabled GitHub Pages: https://trakrf.github.io/platform/

### Linear Updates

Added two comments to TRA-140:
1. Summary of mockup approaches
2. Live GitHub Pages URLs

### Issues/Collisions

- Used `mocks/` instead of `mocky/` as instructed
- HTML files only exist on `mockups/tra-140-reports` branch, not on main working branch
- May have overwritten or conflicted with files from other instances

---

## Instance 2 (Claude - "mockin" assignment)

**Assigned directory:** `spec/TRA-140/mockin/`
**Initial directory used:** `spec/TRA-140/mocker/` (mistake - later renamed to `mockin/`)

### Files Created

**HTML mockups in `spec/TRA-140/mockin/`:**
- `index.html` - Landing page with links to all 3 mockups
- `mockup-a-sidebar.html` - Classic sidebar filters (matches Assets/Locations pages)
- `mockup-b-toolbar.html` - Compact horizontal toolbar filters
- `mockup-c-dashboard.html` - Dashboard with stats cards + tabs

**Markdown specs in `spec/TRA-140/mockin/`:**
- `mockup-a-sidebar-filters.md` - ASCII wireframe + description
- `mockup-b-toolbar-compact.md` - ASCII wireframe + description
- `mockup-c-tabbed-dashboard.md` - ASCII wireframe + description

### User Clarification Questions Asked

Before creating mockups, I asked the user:
1. Nav placement → "New 'Reports' tab"
2. History view → "Slide-over panel"
3. Date filter style → "Both" (presets + custom picker)

### GitHub Pages

- Copied HTML files to repo root for GitHub Pages serving
- Pushed to `mockups/tra-140-reports` branch
- Files at root: `index.html`, `mockup-a-sidebar.html`, `mockup-b-toolbar.html`, `mockup-c-dashboard.html`

### Linear Updates

Added two comments to TRA-140:
1. Summary of mockups created with file locations
2. Live GitHub Pages URLs

### Git Commits Made

1. `docs: add TRA-140 asset location report mockups` - HTML at root for Pages
2. `docs: add TRA-140 mockups to spec directory` - Files in `spec/TRA-140/mocker/`
3. `docs: add TRA-140 mockups from parallel effort (mocks/)` - Committed Instance 1's mocks/
4. `docs: rename mocker -> mockin per spec` - Fixed directory name

### Issues/Collisions

- Initially used `mocker/` instead of `mockin/` - renamed later
- Both Instance 1 and I set up GitHub Pages and added Linear comments (duplicates)
- I committed Instance 1's `mocks/` directory when I saw it untracked

---

## Instance 3 (Claude - this session)

**Assigned directory:** `spec/TRA-140/mocks/` (user said "mockin" but I used "mocks")

### Files Created

**Markdown specs in `spec/TRA-140/mocks/`:**
- `mockup-b1-inline-table.md` - Inline expandable rows with table history
- `mockup-b2-inline-timeline.md` - Inline expandable rows with visual timeline
- `mockup-c1-split-table.md` - Split panel master-detail with table history
- `mockup-c2-split-timeline.md` - Split panel master-detail with visual timeline

**HTML mockups at repo root (for GitHub Pages):**
- `tra140-b1-inline-table.html`
- `tra140-b2-inline-timeline.html`
- `tra140-c1-split-table.html`
- `tra140-c2-split-timeline.html`

Note: My HTML files have `tra140-` prefix to distinguish from Instance 2's `mockup-a/b/c-*.html` files.

### User Clarification Questions Asked

Before creating mockups, I asked:
1. Layout preference → "Both B and C" (inline drill-down AND split panel)
2. Date filter style → "Yes, with presets"
3. Nav location → "Top nav bar" (though user edited to sidebar to match TrakRF)

### Git Commits Made

1. `374f0b7` - `docs: add TRA-140 asset location report mockups` - 4 HTML files
2. `f4c06ca` - `docs: add TRA-140 mockups to index page` - Updated index.html with links

### Linear Updates

Added one comment to TRA-140 with mockup descriptions and viewing instructions.

### Issues/Collisions

- User originally said put mocks in "mockin" but I used "mocks" (same as Instance 1)
- HTML files lost when switching branches - had to recreate them
- My markdown files (b1, b2, c1, c2) differ from Instance 1's markdown files (a, b, c) - we explored different approaches

---

## Current State of spec/TRA-140/

```
spec/TRA-140/
├── mockin/           # Has 3 HTML + 3 MD + index.html
├── mocks/            # Has 4 MD only (Instance 1's work)
├── mocker/           # MISSING
├── mocky/            # MISSING (Instance 1 should have used this)
└── MERGE.md          # This file
```

## Files at Repo Root (need to be moved)

These HTML files are at the repo root and should be organized:
- `index.html`
- `mockup-a-sidebar.html`
- `mockup-b-toolbar.html`
- `mockup-c-dashboard.html`
- `tra140-b1-inline-table.html`
- `tra140-b2-inline-timeline.html`
- `tra140-c1-split-table.html`
- `tra140-c2-split-timeline.html`

---

## Consolidated Mockup Summary for Tim

**Live at:** https://trakrf.github.io/platform/

| # | Mockup | Layout | History View | URL |
|---|--------|--------|--------------|-----|
| 1 | A: Sidebar Filters | Classic sidebar | Slide-over | [View](https://trakrf.github.io/platform/mockup-a-sidebar.html) |
| 2 | B: Toolbar Filters | Horizontal toolbar | Slide-over | [View](https://trakrf.github.io/platform/mockup-b-toolbar.html) |
| 3 | C: Dashboard Stats | Stats cards + tabs | Slide-over | [View](https://trakrf.github.io/platform/mockup-c-dashboard.html) |
| 4 | B1: Inline + Table | Expandable rows | Nested table | [View](https://trakrf.github.io/platform/tra140-b1-inline-table.html) |
| 5 | B2: Inline + Timeline | Expandable rows | Visual timeline | [View](https://trakrf.github.io/platform/tra140-b2-inline-timeline.html) |
| 6 | C1: Split + Table | Master-detail | Table in panel | [View](https://trakrf.github.io/platform/tra140-c1-split-table.html) |
| 7 | C2: Split + Timeline | Master-detail | Timeline in panel | [View](https://trakrf.github.io/platform/tra140-c2-split-timeline.html) |

**Key Design Dimensions Explored:**
- **Filter placement:** Sidebar vs toolbar vs inline
- **History trigger:** Slide-over vs inline expand vs persistent panel
- **History display:** Table vs visual timeline with duration bars
- **Layout:** Single list vs split panel (master-detail)

**Recommendation:** Start with the index page which groups and describes each option.

---

## Resolution Plan

Once all instances document their work:
1. Rename `mocks/` → `mocky/`
2. Create `mocker/` if needed and move appropriate files
3. Move root HTML files to correct subdirectories
4. Update GitHub Pages to serve from consolidated location
5. Update Linear with final organized links
