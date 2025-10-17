# URL Parameter Pattern for Deep Linking

## Overview

This application uses URL parameters for navigation and state management, enabling deep linking, bookmarking, and external app integration. Since we're building a control interface (not a CRUD API), we can be flexible about how data flows into the app.

## Core Principles

1. **URL as Navigation Intent** - Parameters express what the user wants to do, not REST resources
2. **Transport Agnostic** - Use GET params, POST messages, localStorage, or WebSockets as appropriate
3. **Stateful Interface** - The app maintains state; URLs are hints to configure that state
4. **Deep Link Everything** - Any tab/mode should be directly addressable via URL

## Current Implementation

### Basic Navigation
```javascript
// Simple tab navigation
/#inventory
/#barcode
/#locate
/#settings

// Navigation with parameters
/#locate?epc=10023
/#inventory?filter=building-a
/#settings?tab=rfid
```

### Locate with EPC
```javascript
// From inventory/barcode screens
window.location.hash = `#locate?epc=${encodeURIComponent(targetEPC)}`;

// From external apps (QR codes, emails, other systems)
https://app.trakrf.com/#locate?epc=E28011700000020018A6B8C2
```

## Future Patterns

### Bulk Data Loading

#### Via URL Parameters (Small Data)
```javascript
// Load specific inventory items
/#inventory?items=E280117,E280118,E280119

// Configure and start
/#barcode?mode=continuous&start=true

// Multiple settings
/#settings?power=20&session=1&target=A
```

#### Via PostMessage (Large Data)
```javascript
// External app sends bulk data
window.postMessage({
  type: 'LOAD_INVENTORY',
  items: [
    { epc: 'E280117...', rssi: -45, timestamp: Date.now() },
    // ... hundreds more
  ]
}, 'https://app.trakrf.com');

// Navigate to view
window.location.hash = '#inventory?source=import';
```

#### Via localStorage (Persistent Import)
```javascript
// Store bulk data
localStorage.setItem('inventory-import', JSON.stringify({
  timestamp: Date.now(),
  source: 'warehouse-system',
  items: [/* ... */]
}));

// Navigate to trigger import
window.location.hash = '#inventory?import=true';
```

#### Via WebSocket (Real-time Streaming)
```javascript
// Connect to data source
ws.send({ subscribe: 'warehouse/zone-a/tags' });

// Navigate to view with live filter
window.location.hash = '#inventory?live=zone-a';
```

## Implementation Guidelines

### Reading Parameters

```javascript
// Generic parameter reader
function getUrlParams() {
  const hash = window.location.hash;
  const queryStart = hash.indexOf('?');
  if (queryStart === -1) return {};

  const params = new URLSearchParams(hash.slice(queryStart + 1));
  return Object.fromEntries(params);
}

// In component
useEffect(() => {
  const params = getUrlParams();

  if (params.epc) {
    setTargetEPC(params.epc);
  }

  if (params.import) {
    const data = localStorage.getItem('inventory-import');
    if (data) {
      loadInventory(JSON.parse(data));
      localStorage.removeItem('inventory-import');
    }
  }
}, [location.hash]);
```

### Navigation Helpers

```javascript
// Navigate with parameters
function navigateTo(tab, params = {}) {
  const queryString = new URLSearchParams(params).toString();
  window.location.hash = queryString ? `#${tab}?${queryString}` : `#${tab}`;
}

// Usage
navigateTo('locate', { epc: '10023' });
navigateTo('inventory', { filter: 'recent', sort: 'rssi' });
```

## Integration Examples

### QR Code on Asset
```
QR Code contains: https://app.trakrf.com/#locate?epc=E28011700000020018A6B8C2
User scans → App opens → Immediately in locate mode for that item
```

### Email Alert Link
```html
<a href="https://app.trakrf.com/#locate?epc=10023">
  Click to locate missing item #10023
</a>
```

### Warehouse Management System
```javascript
// WMS generates link
const items = await getMissingItems();
const itemList = items.map(i => i.epc).join(',');
const link = `https://app.trakrf.com/#inventory?highlight=${itemList}`;

// Or for large lists, use postMessage
iframe.contentWindow.postMessage({
  type: 'HIGHLIGHT_ITEMS',
  items: items
}, '*');
```

### Mobile App Integration
```javascript
// React Native WebView
webview.postMessage(JSON.stringify({
  type: 'SCAN_RESULT',
  barcode: '123456789'
}));

webview.loadUrl('file:///android_asset/app.html#barcode?external=true');
```

## Benefits

1. **No Backend Required** - Everything works client-side
2. **Instant Integration** - Any app that can generate URLs can integrate
3. **User Control** - Users can bookmark, share, and modify URLs
4. **Progressive Enhancement** - Start simple (URL params), add complexity as needed (PostMessage, WebSocket)
5. **Debugging Friendly** - State is visible in the URL bar
6. **Platform Agnostic** - Works from QR codes, emails, native apps, web apps, etc.

## Security Considerations

1. **Validate All Input** - Treat URL params as untrusted user input
2. **Sanitize EPCs** - Ensure they match expected format before sending to hardware
3. **Origin Checks** - For PostMessage, validate message origin
4. **Size Limits** - Implement reasonable limits for bulk imports
5. **No Sensitive Data** - Never put credentials or secrets in URLs

## Testing

```javascript
// E2E test pattern
test('deep link to locate with EPC', async () => {
  await page.goto('/#locate?epc=10023');
  expect(await getTargetEPC()).toBe('10023');
  expect(await getHardwareMask()).toBe('10023');
});

test('bulk import via postMessage', async () => {
  await page.evaluate(() => {
    window.postMessage({
      type: 'LOAD_INVENTORY',
      items: [/* test data */]
    }, '*');
  });

  await page.goto('/#inventory');
  expect(await getInventoryCount()).toBe(testData.length);
});
```

## Migration Path

1. **Phase 1** ✅ - URL parameters for locate EPC
2. **Phase 2** - URL parameters for other tabs (filters, settings)
3. **Phase 3** - PostMessage for bulk operations
4. **Phase 4** - WebSocket for real-time updates
5. **Phase 5** - Full external API (if needed)

This approach lets us start simple and add complexity only where it provides value.