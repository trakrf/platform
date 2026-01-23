# Mockup A: Classic Sidebar Filters

**Design Philosophy**: Follows the existing Assets/Locations page pattern with a filter sidebar on the left. Familiar to users, consistent with existing UI.

---

## Main View: Current Asset Locations

```
+-----------------------------------------------------------------------------------+
|  [Logo]  Home | Inventory | Locate | Assets | Locations | [Reports] | Settings   |
+-----------------------------------------------------------------------------------+
|                                                                                   |
|  +-- FILTERS --------+  +--------------------------------------------------+      |
|  |                   |  |  CURRENT ASSET LOCATIONS                         |      |
|  |  Location         |  |  ------------------------------------------------|      |
|  |  +-------------+  |  |  [Search assets...]              [Export CSV v]  |      |
|  |  | All         v |  |  ------------------------------------------------|      |
|  |  +-------------+  |  |                                                  |      |
|  |                   |  |  +----------------------------------------------+|      |
|  |  Asset Type       |  |  | ASSET NAME     | LOCATION      | LAST SEEN   ||      |
|  |  [ ] Device       |  |  |----------------|---------------|-------------||      |
|  |  [ ] Person       |  |  | Projector A1   | Room 101      | 2 min ago   >|      |
|  |  [ ] Inventory    |  |  | Laptop Cart B  | Storage       | 15 min ago  >|      |
|  |  [x] Asset        |  |  | AV Rack #3     | Auditorium    | 1 hour ago  >|      |
|  |  [ ] Other        |  |  | Camera Kit     | Room 205      | 3 hours ago >|      |
|  |                   |  |  | Microphone Set | Conference A  | Yesterday   >|      |
|  |  Last Seen        |  |  | Display Stand  | Lobby         | 2 days ago  >|      |
|  |  ( ) Any time     |  |  +----------------------------------------------+|      |
|  |  (x) Last 24h     |  |                                                  |      |
|  |  ( ) Last 7 days  |  |  Showing 1-6 of 124 assets    < 1 2 3 ... 21 >   |      |
|  |  ( ) Last 30 days |  +--------------------------------------------------+      |
|  |                   |                                                            |
|  |  [Clear Filters]  |                                                            |
|  +-------------------+                                                            |
|                                                                                   |
+-----------------------------------------------------------------------------------+
```

**Key Features:**
- Sidebar filters match existing Assets page pattern
- Location dropdown filter
- Asset type checkboxes
- "Last Seen" time filter for quick stale-asset detection
- Table rows are clickable (arrow indicator)
- Export CSV dropdown in header

---

## Slide-Over Panel: Asset History

```
+-----------------------------------------------------------------------------------+
|  [Logo]  Home | Inventory | Locate | Assets | Locations | [Reports] | Settings   |
+-----------------------------------------------------------------------------------+
|                                                          +------------------------+
|  +-- FILTERS --------+  +-----------------------------+  | ASSET HISTORY      [X] |
|  |                   |  |  CURRENT ASSET LOCATIONS    |  +------------------------+
|  |  Location         |  |  ---------------------------+  |                        |
|  |  +-------------+  |  |  [Search...]    [Export v]  |  |  Projector A1          |
|  |  | All         v |  |  ---------------------------+  |  ID: AST-001           |
|  |  +-------------+  |  |                             |  |                        |
|  |                   |  |  +-------------------------+|  |  +-- DATE RANGE -----+ |
|  |  Asset Type       |  |  | ASSET      | LOC | SEEN ||  |  | [Today] [7d] [30d] | |
|  |  [x] Asset        |  |  |------------|-----|------||  |  | [90d] [Custom]     | |
|  |                   |  |  | Projector  |  ...|  ... ||  |  +--------------------+ |
|  |  Last Seen        |  |  | Laptop ... |  ...|  ... ||  |  01/15 - 01/23/2026    |
|  |  (x) Last 24h     |  |  | AV Rack .. |  ...|  ... ||  |                        |
|  |                   |  |  +-------------------------+|  |  +-- TIMELINE -------+ |
|  |  [Clear Filters]  |  |                             |  |  |                    | |
|  +-------------------+  +-----------------------------+  |  | Jan 23, 10:32 AM   | |
|                                                          |  | Room 101           | |
|                                                          |  | Duration: 2h 15m   | |
|                                                          |  |                    | |
|                                                          |  | Jan 23, 08:17 AM   | |
|                                                          |  | Storage            | |
|                                                          |  | Duration: 45m      | |
|                                                          |  |                    | |
|                                                          |  | Jan 22, 04:30 PM   | |
|                                                          |  | Auditorium         | |
|                                                          |  | Duration: 3h 10m   | |
|                                                          |  |                    | |
|                                                          |  +--------------------+ |
|                                                          |                        |
|                                                          |  [Download CSV]        |
|                                                          +------------------------+
```

**Slide-Over Features:**
- 400px width, slides from right
- Asset identifier and name in header
- Date range presets: Today, 7d, 30d, 90d, Custom
- Custom date picker appears when "Custom" selected
- Timeline shows chronological location history
- Each entry: timestamp, location, duration at that location
- CSV download button at bottom

---

## Pros & Cons

**Pros:**
- Consistent with existing Assets/Locations pages
- Users already familiar with filter sidebar pattern
- Clear visual separation of filters from data
- Slide-over keeps context of main list

**Cons:**
- Sidebar takes up horizontal space
- Filter sidebar may feel heavy for a simple report
- Less room for the main table on smaller screens
