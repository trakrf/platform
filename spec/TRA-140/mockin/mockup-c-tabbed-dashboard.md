# Mockup C: Tabbed Dashboard with Stats

**Design Philosophy**: Dashboard-style layout with summary statistics, tabbed report selection, and quick action cards. Designed for users who need an overview before drilling down.

---

## Main View: Reports Dashboard

```
+-----------------------------------------------------------------------------------+
|  [Logo]  Home | Inventory | Locate | Assets | Locations | [Reports] | Settings   |
+-----------------------------------------------------------------------------------+
|                                                                                   |
|  REPORTS                                                                          |
|  ===============================================================================  |
|                                                                                   |
|  +---------------------------+  +---------------------------+  +----------------+ |
|  |  TOTAL ASSETS TRACKED    |  |  ASSETS SEEN TODAY        |  |  STALE (>7d)   | |
|  |  [icon]                  |  |  [icon]                   |  |  [icon]        | |
|  |         247              |  |          89               |  |      12        | |
|  |  +4 from last week       |  |  36% of total             |  |  [View ->]     | |
|  +---------------------------+  +---------------------------+  +----------------+ |
|                                                                                   |
|  +-----------------------------------------------------------------------------+  |
|  |  [Current Locations]  [Movement History]  [Stale Assets]                    |  |
|  +-----------------------------------------------------------------------------+  |
|                                                                                   |
|  +---------------------------------------------------------------------------+    |
|  |  +----------------------------+                                           |    |
|  |  | [Q] Search by asset name   |  Location: [All v]  Seen: [Any time v]   |    |
|  |  +----------------------------+                                           |    |
|  +---------------------------------------------------------------------------+    |
|                                                                                   |
|  +---------------------------------------------------------------------------+    |
|  |                                                                           |    |
|  |   ASSET                    LOCATION              LAST SEEN       STATUS   |    |
|  |  -------------------------------------------------------------------------+    |
|  |   Projector A1             Room 101              2 min ago       [Live]   |    |
|  |  -------------------------------------------------------------------------+    |
|  |   Laptop Cart B            Storage               15 min ago      [Live]   |    |
|  |  -------------------------------------------------------------------------+    |
|  |   AV Rack #3               Auditorium            1 hour ago      [Live]   |    |
|  |  -------------------------------------------------------------------------+    |
|  |   Camera Kit               Room 205              3 hours ago     [Today]  |    |
|  |  -------------------------------------------------------------------------+    |
|  |   Microphone Set           Conference A          Yesterday       [Recent] |    |
|  |  -------------------------------------------------------------------------+    |
|  |   Display Stand            Lobby                 8 days ago      [Stale]  |    |
|  |  -------------------------------------------------------------------------+    |
|  |                                                                           |    |
|  +---------------------------------------------------------------------------+    |
|                                                                                   |
|  [Download CSV]  [Download PDF]          Showing 1-6 of 247    [< 1 2 3 ... >]   |
|                                                                                   |
+-----------------------------------------------------------------------------------+
```

**Key Features:**
- Summary stat cards at top (Total, Seen Today, Stale)
- Stale assets card links directly to filtered view
- Tabbed report selection for future expansion
- Status badges: [Live] [Today] [Recent] [Stale]
- Multiple export format buttons
- Clean table with row separators

---

## Slide-Over Panel: Asset History

```
+-----------------------------------------------------------------------------------+
|  [Logo]  Home | Inventory | Locate | Assets | Locations | [Reports] | Settings   |
+-----------------------------------------------------------------------------------+
|                                                        +-------------------------+|
|  REPORTS                                               | PROJECTOR A1         [X]||
|  ======================================                +-------------------------+|
|                                                        |                         ||
|  +------------+  +------------+  +------------+        |  +-------------------+  ||
|  | TRACKED    |  | TODAY      |  | STALE      |        |  | ASSET DETAILS     |  ||
|  |   247      |  |   89       |  |    12      |        |  | ID: AST-001       |  ||
|  +------------+  +------------+  +------------+        |  | Type: Device      |  ||
|                                                        |  | Location: Room 101|  ||
|  +----------------------------------------+            |  +-------------------+  ||
|  | [Current Locations] [Movement] [Stale] |            |                         ||
|  +----------------------------------------+            |  +-------------------+  ||
|                                                        |  | DATE RANGE        |  ||
|  +----------------------------------------+            |  +-------------------+  ||
|  | [Q] Search...    [All v]  [Any time v] |            |  | [Today] [7 Days]  |  ||
|  +----------------------------------------+            |  | [30 Days] [90 d]  |  ||
|                                                        |  |                   |  ||
|  +----------------------------------------+            |  | [Custom Range...] |  ||
|  | ASSET       | LOC      | SEEN   | STAT |            |  | 01/15 - 01/23     |  ||
|  |-------------|----------|--------|------|            |  +-------------------+  ||
|  | Projector.. | Room 101 | 2m     |[Live]|            |                         ||
|  | Laptop...   | Storage  | 15m    |[Live]|            |  MOVEMENT TIMELINE      ||
|  | AV Rack...  | Auditor..| 1h     |[Live]|            |  =====================  ||
|  +----------------------------------------+            |                         ||
|                                                        |  +-------------------+  ||
|                                                        |  | 10:32 | Room 101  |  ||
|                                                        |  |       | 2h 15m *  |  ||
|                                                        |  |-------|-----------|  ||
|                                                        |  | 08:17 | Storage   |  ||
|                                                        |  |       | 45m       |  ||
|                                                        |  |-------|-----------|  ||
|                                                        |  | 04:30 | Auditorium|  ||
|                                                        |  | (Jan 22) 3h 10m   |  ||
|                                                        |  |-------|-----------|  ||
|                                                        |  | 02:15 | Room 101  |  ||
|                                                        |  | (Jan 22) 2h 15m   |  ||
|                                                        |  +-------------------+  ||
|                                                        |                         ||
|                                                        |  [Download History CSV] ||
|                                                        +-------------------------+|
+-----------------------------------------------------------------------------------+
```

**Slide-Over Features:**
- Asset details card at top
- Preset date range buttons in grid layout
- "Custom Range..." expands to show date pickers
- Table-style timeline (easier to scan)
- Asterisk (*) indicates current/active location
- Date annotations for entries not from today
- Single export button for history

---

## Mobile Responsive View (Bonus)

```
+---------------------------+
|  [=] REPORTS         [?]  |
+---------------------------+
|                           |
| +-------+ +-------+ +---+ |
| |TRACKED| |TODAY  | |OLD| |
| | 247   | |  89   | | 12| |
| +-------+ +-------+ +---+ |
|                           |
| [Current] [History] [Old] |
|                           |
| [Search...             ]  |
| [All Locations v]         |
| [Any time v]              |
|                           |
| +------------------------+|
| | Projector A1       [>] ||
| | Room 101 | 2 min ago   ||
| | [Live]                 ||
| +------------------------+|
| | Laptop Cart B      [>] ||
| | Storage | 15 min ago   ||
| | [Live]                 ||
| +------------------------+|
| | AV Rack #3         [>] ||
| | Auditorium | 1 hr ago  ||
| | [Live]                 ||
| +------------------------+|
|                           |
| [< 1 / 42 >] [Download]   |
+---------------------------+
```

---

## Pros & Cons

**Pros:**
- Dashboard stats provide immediate insight
- Tab structure allows future report types
- Status badges make asset health visible at a glance
- "Stale" quick link helps with maintenance workflows
- Table-style timeline in slide-over is scannable
- Mobile-friendly card layout planned

**Cons:**
- More complex layout to implement
- Stats cards require additional API calls
- May be overkill for MVP if only 2 reports
- Takes more vertical space above the data table
