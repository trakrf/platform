/**
 * Barcode Parser Functions
 * Parsers for barcode-specific data formats
 */

/**
 * Barcode Data Parser
 * Parses CS108 barcode notification (0x9100)
 * Handles fragmented data, prefixes, suffixes, and Code/AIM IDs
 */
export const parseBarcodeData = (data: Uint8Array): {
  symbology: string;
  data: string;
  rawData: Uint8Array;
} => {
  if (data.length === 0) {
    return { symbology: 'UNKNOWN', data: '', rawData: data };
  }

  // Convert to string for pattern analysis
  let workingString = new TextDecoder().decode(data);
  let barcodeType = 'Unknown';

  // Known prefix patterns to remove
  const prefixes = [
    '\x02\x00\x07\x10\x17\x13', // Common prefix
    '\x06\x06',                   // Alternate prefix
    '\x06\x02\x00\x07\x10\x17\x13' // Another variant
  ];

  // Remove prefixes
  for (const prefix of prefixes) {
    if (workingString.startsWith(prefix)) {
      workingString = workingString.substring(prefix.length);
      break;
    }
  }

  // Remove non-printable characters at the start
  while (workingString.length > 0 && workingString.charCodeAt(0) < 0x20) {
    workingString = workingString.substring(1);
  }

  // Handle Code ID + AIM ID patterns (e.g., j]C0, u]d4, Q]Q1)
  // Code ID can be upper or lowercase depending on scanner config
  const codeAimPattern = /^([a-zA-Z])(]\w\w)/;
  const codeAimMatch = workingString.match(codeAimPattern);
  if (codeAimMatch) {
    const aimId = codeAimMatch[2];
    workingString = workingString.substring(4); // Remove Code ID + AIM ID

    // Determine type from AIM ID
    const aimPrefix = aimId[1]; // Character after ']'
    switch (aimPrefix) {
      case 'C': barcodeType = 'Code 128'; break;
      case 'd': barcodeType = 'Data Matrix'; break;
      case 'E': barcodeType = 'EAN/UPC'; break;
      case 'I': barcodeType = 'Interleaved 2 of 5'; break;
      case 'A': barcodeType = 'Code 39'; break;
      case 'Q': barcodeType = 'QR Code'; break;
    }
  }

  // Remove specific Code/AIM patterns if still present
  const codePatterns = ['j]C0', 'u]d4', ']C0', ']d4'];
  for (const pattern of codePatterns) {
    if (workingString.startsWith(pattern)) {
      workingString = workingString.substring(pattern.length);
      // Set type if not already determined
      if (barcodeType === 'Unknown') {
        if (pattern.includes('C')) barcodeType = 'Code 128';
        else if (pattern.includes('d')) barcodeType = 'Data Matrix';
      }
      break;
    }
  }

  // Remove known suffix pattern
  const suffixPattern = '\x05\x01\x11\x16\x03\x04';
  const suffixIndex = workingString.indexOf(suffixPattern);
  if (suffixIndex >= 0) {
    workingString = workingString.substring(0, suffixIndex);
  }

  // Find and remove terminator (0x0D)
  const terminatorIndex = workingString.indexOf('\x0D');
  if (terminatorIndex >= 0) {
    workingString = workingString.substring(0, terminatorIndex);
  }

  // Final cleanup - trim whitespace
  const cleanBarcodeText = workingString.trim();

  // Handle the case where the barcode might have leading zeros
  // A barcode with leading zeros should be recognized without them
  let finalBarcode = cleanBarcodeText;

  // Remove leading zeros for numeric barcodes
  if (/^\d+$/.test(finalBarcode) && finalBarcode.startsWith('0')) {
    // Keep removing leading zeros but preserve at least one digit
    while (finalBarcode.length > 1 && finalBarcode.startsWith('0')) {
      finalBarcode = finalBarcode.substring(1);
    }
  }

  return {
    symbology: barcodeType,
    data: finalBarcode,
    rawData: data
  };
};