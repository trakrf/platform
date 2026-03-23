/**
 * Reconciliation utilities - CSV parsing and EPC normalization
 * Extracted from ReconciliationScreen.tsx for reuse in InventoryScreen.tsx
 */

// Type for reconciliation data items
export interface ReconciliationItem {
  epc: string;            // Normalized EPC for matching
  originalEpc?: string;   // Original EPC from CSV for display
  assetIdentifier?: string; // "ASSET-0003" from CSV Asset ID column (matches TagInfo.assetIdentifier)
  description?: string;
  location?: string;
  rssi?: number;         // RSSI value from CSV
  count: number;
  found: boolean;
  lastSeen?: number;
}

// Asset-level reconciliation for grouping tags by parent asset
export interface ReconciliationAsset {
  assetIdentifier: string;
  tagIds: string[];
  name?: string;
  description?: string;
  location?: string;
  found: boolean;
  foundTagIds: string[];
}

// Lookup map: normalized tag EPC → parent ReconciliationAsset
export type TagToAssetMap = Map<string, ReconciliationAsset>;

/**
 * Helper function to properly parse CSV rows (handling quoted fields with commas)
 */
export const parseCSVRow = (row: string): string[] => {
  const result = [];
  let inQuotes = false;
  let fieldValue = '';
  let i = 0;
  
  while (i < row.length) {
    const char = row[i];
    
    // Handle quoted fields
    if (char === '"') {
      if (i + 1 < row.length && row[i + 1] === '"') {
        // Escaped quote inside a quoted field
        fieldValue += '"';
        i += 2;
      } else {
        // Toggle quote state
        inQuotes = !inQuotes;
        i++;
      }
    } 
    // Handle field separator
    else if (char === ',' && !inQuotes) {
      result.push(fieldValue);
      fieldValue = '';
      i++;
    } 
    // Handle regular character
    else {
      fieldValue += char;
      i++;
    }
  }
  
  // Push the last field
  result.push(fieldValue);
  
  return result;
};

/**
 * Normalize any EPC string for comparison
 */
export const normalizeEpc = (epc: string): string => {
  if (!epc) return '';
  
  // First convert to uppercase to match scanner format
  let normalized = epc.toUpperCase();
  
  // Remove any non-hex characters
  normalized = normalized.replace(/[^0-9A-F]/g, '');
  
  // Return the normalized EPC
  return normalized;
};

/**
 * Helper function to remove leading zeros from EPC
 */
export const removeLeadingZeros = (epc: string): string => {
  if (!epc) return '';
  // Remove leading zeros but keep at least one digit if all zeros
  return epc.replace(/^0+(?=\d)/, '');
};

/**
 * Parse CSV content and extract reconciliation items
 */
export const parseReconciliationCSV = (csvData: string): {
  success: boolean;
  data: ReconciliationItem[];
  error?: string;
} => {
  try {
    console.info(`Parsing CSV file with ${csvData.length} characters`);
    
    // Parse CSV
    const lines = csvData.split(/\r\n|\n/);
    console.info(`Found ${lines.length} lines in CSV file`);
    
    const headers = lines[0].split(',').map(h => h.trim());
    console.info(`CSV headers: ${headers.join(', ')}`);
    
    // Find ALL Tag ID column indices (supports multi-tag asset CSVs)
    // First check for exact "Tag ID" columns (from asset export format)
    const tagIdColumnIndices: number[] = headers
      .map((h, i) => /^tag\s*id$/i.test(h.trim()) ? i : -1)
      .filter(i => i !== -1);

    // If no "Tag ID" columns found, fall back to broader EPC pattern matching
    if (tagIdColumnIndices.length === 0) {
      const epcHeaderPatterns = [
        /epc/i,
        /rfid/i,
        /id/i,
        /code/i,
        /number/i,
        /serial/i
      ];

      let epcColumnIndex = -1;
      for (const pattern of epcHeaderPatterns) {
        epcColumnIndex = headers.findIndex(header => pattern.test(header));
        if (epcColumnIndex !== -1) {
          console.info(`Found EPC column at index ${epcColumnIndex} (${headers[epcColumnIndex]}) using pattern ${pattern}`);
          break;
        }
      }

      if (epcColumnIndex === -1) {
        console.warn('Could not find a clear EPC column, using first column as fallback');
        epcColumnIndex = 0;
      }
      tagIdColumnIndices.push(epcColumnIndex);
    } else {
      console.info(`Found ${tagIdColumnIndices.length} Tag ID column(s) at indices: ${tagIdColumnIndices.join(', ')}`);
    }

    // Find Asset ID column (from asset export format)
    const assetIdColumnIndex = headers.findIndex(h => /^asset\s*id$/i.test(h.trim()));
    
    // Find optional description, location, and RSSI columns with broader patterns
    const descriptionPatterns = [/desc/i, /name/i, /title/i, /item/i, /product/i];
    const locationPatterns = [/loc/i, /place/i, /area/i, /zone/i, /region/i, /position/i, /where/i];
    const rssiPatterns = [/rssi/i, /signal/i, /strength/i, /dbm/i];
    
    let descriptionColumnIndex = -1;
    for (const pattern of descriptionPatterns) {
      descriptionColumnIndex = headers.findIndex(header => pattern.test(header));
      if (descriptionColumnIndex !== -1) {
        console.info(`Found description column at index ${descriptionColumnIndex} (${headers[descriptionColumnIndex]})`);
        break;
      }
    }
    
    let locationColumnIndex = -1;
    for (const pattern of locationPatterns) {
      locationColumnIndex = headers.findIndex(header => pattern.test(header));
      if (locationColumnIndex !== -1) {
        console.info(`Found location column at index ${locationColumnIndex} (${headers[locationColumnIndex]})`);
        break;
      }
    }
    
    let rssiColumnIndex = -1;
    for (const pattern of rssiPatterns) {
      rssiColumnIndex = headers.findIndex(header => pattern.test(header));
      if (rssiColumnIndex !== -1) {
        console.info(`Found RSSI column at index ${rssiColumnIndex} (${headers[rssiColumnIndex]})`);
        break;
      }
    }
    
    // Parse data rows
    const parsedData: ReconciliationItem[] = [];
    let skippedCount = 0;
    
    for (let i = 1; i < lines.length; i++) {
      if (!lines[i].trim()) {
        skippedCount++;
        continue; // Skip empty lines
      }
      
      const values = parseCSVRow(lines[i]);

      // Extract shared row metadata
      const assetIdentifier = assetIdColumnIndex !== -1 && assetIdColumnIndex < values.length
        ? values[assetIdColumnIndex]?.trim() || undefined
        : undefined;

      const description = descriptionColumnIndex !== -1 && descriptionColumnIndex < values.length
        ? values[descriptionColumnIndex]?.trim()
        : undefined;

      const location = locationColumnIndex !== -1 && locationColumnIndex < values.length
        ? values[locationColumnIndex]?.trim()
        : undefined;

      // Parse RSSI value
      let rssi: number | undefined;
      if (rssiColumnIndex !== -1 && rssiColumnIndex < values.length) {
        const rssiValue = values[rssiColumnIndex]?.trim();
        if (rssiValue) {
          const cleanRssi = rssiValue.replace(/[^-0-9.]/g, '');
          rssi = parseFloat(cleanRssi);
          if (isNaN(rssi)) {
            rssi = undefined;
          }
        }
      }

      // Emit one ReconciliationItem per non-empty tag column
      let rowHasTag = false;
      for (const colIdx of tagIdColumnIndices) {
        if (colIdx >= values.length) continue;

        const epc = values[colIdx]?.trim();
        if (!epc) continue;

        const normalizedEpc = normalizeEpc(epc);
        if (!normalizedEpc) continue;

        if (normalizedEpc.length < 4) {
          console.warn(`Skipping invalid EPC at line ${i+1}: "${epc}" (normalized: "${normalizedEpc}") - too short`);
          continue;
        }

        rowHasTag = true;
        parsedData.push({
          epc: normalizedEpc,
          originalEpc: epc,
          assetIdentifier,
          description,
          location,
          rssi,
          count: 0,
          found: false,
        });
      }

      if (!rowHasTag) {
        skippedCount++;
      }
    }
    
    if (parsedData.length === 0) {
      return {
        success: false,
        data: [],
        error: 'No valid tag IDs found in the CSV file'
      };
    }
    
    console.info(`Successfully parsed ${parsedData.length} tags from CSV file (${skippedCount} rows skipped)`);
    
    // Log sample data for verification
    console.info("Sample EPCs from parsed data:");
    parsedData.slice(0, Math.min(5, parsedData.length)).forEach((item, i) => {
      console.info(`Item ${i+1}: Original="${item.originalEpc}", Normalized="${item.epc}"`);
    });
    
    return {
      success: true,
      data: parsedData
    };
    
  } catch (error) {
    console.error('Error parsing CSV:', error);
    return {
      success: false,
      data: [],
      error: 'Failed to parse CSV file. Please ensure it\'s a valid CSV format.'
    };
  }
};

/**
 * Create a Set from reconciliation data for fast EPC lookup
 */
export const createReconciliationSet = (data: ReconciliationItem[]): Set<string> => {
  return new Set(data.map(item => item.epc));
};

/**
 * Get reconciliation statistics from data
 */
export const getReconciliationStats = (data: ReconciliationItem[]) => {
  const total = data.length;
  const found = data.filter(item => item.found).length;
  const missing = total - found;
  
  return {
    total,
    found,
    missing
  };
};

/**
 * Build a lookup map from tag EPC → parent ReconciliationAsset.
 * Tags without assetIdentifier are excluded.
 */
export function buildAssetMap(items: ReconciliationItem[]): TagToAssetMap {
  const assetsByIdentifier = new Map<string, ReconciliationAsset>();
  const tagToAsset = new Map<string, ReconciliationAsset>();

  for (const item of items) {
    if (!item.assetIdentifier) continue;

    let asset = assetsByIdentifier.get(item.assetIdentifier);
    if (!asset) {
      asset = {
        assetIdentifier: item.assetIdentifier,
        tagIds: [],
        name: item.description,
        description: item.description,
        location: item.location,
        found: false,
        foundTagIds: [],
      };
      assetsByIdentifier.set(item.assetIdentifier, asset);
    }
    asset.tagIds.push(item.epc);
    tagToAsset.set(item.epc, asset);
  }
  return tagToAsset;
}

/**
 * Compute asset-level reconciliation stats.
 * Groups items by assetIdentifier (falls back to epc for items without one).
 * An asset is "Found" if ANY of its tags are found.
 */
export function getAssetReconciliationStats(items: ReconciliationItem[]): {
  totalAssets: number;
  foundAssets: number;
  missingAssets: number;
} {
  const assetStatus = new Map<string, boolean>();
  for (const item of items) {
    const key = item.assetIdentifier ?? item.epc;
    const current = assetStatus.get(key) ?? false;
    assetStatus.set(key, current || item.found);
  }
  const totalAssets = assetStatus.size;
  const foundAssets = [...assetStatus.values()].filter(Boolean).length;
  return { totalAssets, foundAssets, missingAssets: totalAssets - foundAssets };
}

/**
 * Update reconciliation items with found tags
 */
export const updateReconciliationMatches = (
  reconciliationData: ReconciliationItem[],
  inventoryEpcs: Set<string>
): ReconciliationItem[] => {
  return reconciliationData.map(item => {
    if (!item.found && inventoryEpcs.has(item.epc)) {
      return {
        ...item,
        found: true,
        count: 1,
        lastSeen: Date.now()
      };
    }
    return item;
  });
};