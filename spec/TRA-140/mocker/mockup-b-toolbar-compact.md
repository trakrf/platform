# Mockup B: Compact Toolbar Filters

**Design Philosophy**: Horizontal toolbar with inline filters. Maximizes vertical space for data. More modern, streamlined appearance.

---

## Main View: Current Asset Locations

```
+-----------------------------------------------------------------------------------+
|  [Logo]  Home | Inventory | Locate | Assets | Locations | [Reports] | Settings   |
+-----------------------------------------------------------------------------------+
|                                                                                   |
|  REPORTS > CURRENT ASSET LOCATIONS                                                |
|  -------------------------------------------------------------------------------  |
|                                                                                   |
|  +-----------------------------------------------------------------------------+  |
|  |  [Search assets...        ]  Location: [All Locations v]  [Last 24h v]     |  |
|  |                                                             [Export CSV v]  |  |
|  +-----------------------------------------------------------------------------+  |
|                                                                                   |
|  +-----------------------------------------------------------------------------+  |
|  |  ASSET NAME            CURRENT LOCATION        LAST SEEN           TAGS    |  |
|  |----------------------------------------------------------------------------|  |
|  |  [P] Projector A1      Room 101                2 min ago           AV   -> |  |
|  |----------------------------------------------------------------------------|  |
|  |  [D] Laptop Cart B     Storage                 15 min ago          IT   -> |  |
|  |----------------------------------------------------------------------------|  |
|  |  [D] AV Rack #3        Auditorium              1 hour ago          AV   -> |  |
|  |----------------------------------------------------------------------------|  |
|  |  [D] Camera Kit        Room 205                3 hours ago         Media-> |  |
|  |----------------------------------------------------------------------------|  |
|  |  [D] Microphone Set    Conference A            Yesterday                -> |  |
|  |----------------------------------------------------------------------------|  |
|  |  [D] Display Stand     Lobby                   2 days ago           AV  -> |  |
|  +-----------------------------------------------------------------------------+  |
|                                                                                   |
|  Showing 1-6 of 124         [10 per page v]         [<] [1] [2] [3] ... [>]       |
|                                                                                   |
+-----------------------------------------------------------------------------------+
```

**Key Features:**
- Horizontal filter bar with dropdowns
- Search, Location filter, Time filter all inline
- Breadcrumb shows navigation context
- Type icon prefix [P]=Person, [D]=Device, etc.
- Tags column for quick categorization
- Arrow indicates clickable row
- Clean, minimal chrome

---

## Slide-Over Panel: Asset History

```
+-----------------------------------------------------------------------------------+
|  [Logo]  Home | Inventory | Locate | Assets | Locations | [Reports] | Settings   |
+-----------------------------------------------------------------------------------+
|                                                    +------------------------------+
|  REPORTS > CURRENT ASSET LOCATIONS                 |  [<-] PROJECTOR A1       [X]|
|  -------------------------------------------       +------------------------------+
|                                                    |  ID: AST-001 | Type: Device |
|  +------------------------------------------+      |  Tags: AV, Equipment         |
|  |  [Search...]  [All Locations v] [24h v]  |      +------------------------------+
|  +------------------------------------------+      |                              |
|                                                    |  LOCATION HISTORY            |
|  +------------------------------------------+      |  ---------------------------  |
|  |  ASSET NAME    | LOCATION    | SEEN      |      |                              |
|  |----------------|-------------|-----------|      |  [Today] [7d] [30d] [All]    |
|  |  Projector A1  | Room 101    | 2 min  -> |      |  [01/15/2026] to [01/23/2026]|
|  |  Laptop Cart B | Storage     | 15 min    |      |                              |
|  |  AV Rack #3    | Auditorium  | 1 hr      |      |  +------------------------+  |
|  |  Camera Kit    | Room 205    | 3 hr      |      |  |  TODAY                 |  |
|  +------------------------------------------+      |  |  10:32 AM              |  |
|                                                    |  |  [*] Room 101          |  |
|  Showing 1-6 of 124      [<] 1 2 3 [>]             |  |      Currently here    |  |
|                                                    |  |      (2h 15m so far)   |  |
|                                                    |  |                        |  |
|                                                    |  |  08:17 AM              |  |
|                                                    |  |  [o] Storage           |  |
|                                                    |  |      45 minutes        |  |
|                                                    |  |                        |  |
|                                                    |  |  YESTERDAY             |  |
|                                                    |  |  04:30 PM              |  |
|                                                    |  |  [o] Auditorium        |  |
|                                                    |  |      3h 10m            |  |
|                                                    |  |                        |  |
|                                                    |  |  02:15 PM              |  |
|                                                    |  |  [o] Room 101          |  |
|                                                    |  |      2h 15m            |  |
|                                                    |  +------------------------+  |
|                                                    |                              |
|                                                    |  [Download CSV]              |
|                                                    +------------------------------+
```

**Slide-Over Features:**
- Back arrow for quick return to list
- Asset metadata in header (ID, Type, Tags)
- Combined preset + date picker filter
- Visual timeline with:
  - Day groupings (TODAY, YESTERDAY, JAN 21, etc.)
  - Filled circle [*] for current location
  - Empty circles [o] for past locations
  - Duration displayed below each location
- "Currently here" indicator for active location

---

## Pros & Cons

**Pros:**
- Maximizes vertical space for data
- Filters are quick to access in toolbar
- Clean, modern appearance
- Timeline grouping by day improves readability
- Asset context (tags, type) visible in slide-over

**Cons:**
- Horizontal filters can get crowded with many options
- Less obvious filter state than sidebar checkboxes
- May need responsive breakpoints for mobile
