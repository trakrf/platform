import React, { useState } from 'react';
import { ChevronDown, ChevronUp, HelpCircle, Package2, Search, ScanLine, Settings, AlertCircle, Zap } from 'lucide-react';
import { useUIStore } from '@/stores';
import { useBluetoothSupport } from '@/hooks/useBluetoothSupport';

interface FAQItem {
  question: string;
  answer: string;
  icon?: React.ReactNode;
}

interface FAQSection {
  title: string;
  icon: React.ReactNode;
  items: FAQItem[];
}

const FAQAccordion: React.FC<{ item: FAQItem; isOpen: boolean; onToggle: () => void }> = ({ item, isOpen, onToggle }) => {
  return (
    <div className="border-b border-gray-200 dark:border-gray-700">
      <button
        onClick={onToggle}
        className="w-full px-6 py-4 text-left hover:bg-gray-50 dark:hover:bg-gray-800 transition-colors"
      >
        <div className="flex items-center justify-between">
          <div className="flex items-center space-x-3">
            {item.icon && <div className="text-blue-600 dark:text-blue-400">{item.icon}</div>}
            <h3 className="text-sm font-medium text-gray-900 dark:text-gray-100">{item.question}</h3>
          </div>
          {isOpen ? (
            <ChevronUp className="w-5 h-5 text-gray-400" />
          ) : (
            <ChevronDown className="w-5 h-5 text-gray-400" />
          )}
        </div>
      </button>
      {isOpen && (
        <div className="px-6 pb-4">
          <p className="text-sm text-gray-600 dark:text-gray-400 leading-relaxed whitespace-pre-line">
            {item.answer}
          </p>
        </div>
      )}
    </div>
  );
};

export default function HelpScreen() {
  // Set active tab when component mounts - standard React pattern
  React.useEffect(() => {
    useUIStore.getState().setActiveTab('help');
  }, []);

  const [openItems, setOpenItems] = useState<Set<string>>(new Set());

  // Help and the Scan-tab banner answer "which browser?" from the same place,
  // so the advice cannot drift between them the way it had before TRA-1078.
  const { recommendation } = useBluetoothSupport();

  const toggleItem = (itemId: string) => {
    const newOpenItems = new Set(openItems);
    if (newOpenItems.has(itemId)) {
      newOpenItems.delete(itemId);
    } else {
      newOpenItems.add(itemId);
    }
    setOpenItems(newOpenItems);
  };

  // Every nav name and control label quoted below is the label the app renders
  // today. When a surface is renamed, this file is part of the rename — see
  // HelpScreen.test.tsx, which cross-checks these against the real sidebar.
  const faqSections: FAQSection[] = [
    {
      title: 'Getting Started - Read This First!',
      icon: <Zap className="w-5 h-5" />,
      items: [
        {
          question: 'Which browser should I use?',
          answer: `Use ${recommendation.browsers}.

${recommendation.note}${recommendation.links.map((link) => `\n\n${link.label}: ${link.url}`).join('')}`
        },
        {
          question: 'How do I connect my scanner?',
          answer: `1. Turn on your CS108 scanner (green light)
2. Click "Settings" on the left menu
3. Click the blue "Connect Device" button
4. Pick your CS108 from the popup list
5. You'll see "Connected" in green when ready`
        },
        {
          question: 'How do I scan items?',
          answer: `1. First connect your scanner (see above)
2. Click "Scan" on the left menu
3. Point scanner at your tags
4. Hold down the trigger button - or click "Start" in the toolbar
5. Items appear on screen instantly!`
        },
        {
          question: 'What is this app for?',
          answer: `This app helps you:
• Find lost items with RFID tags
• Check what's here against a list you upload
• Read barcodes with your scanner's barcode reader
• Track items in real-time`
        }
      ]
    },
    {
      title: 'Scan',
      icon: <Package2 className="w-5 h-5" />,
      items: [
        {
          question: 'What do the tiles at the top mean?',
          answer: `• Scans (blue) = every tag you've read
• Assets (purple) = reads matched to an asset in your organization

Upload a list and three more appear:
• Found (green) = on your list and here
• Missing (red) = on your list but not read
• Extra (gray) = read but not on your list

Click any tile to show only those items. Click "Scans" to clear the filter.`
        },
        {
          question: 'How do I check what\'s missing?',
          answer: `1. Click "Reconcile" in the toolbar
2. Upload your list (Excel file saved as CSV)
3. Scan your area
4. Red items are missing!`
        },
        {
          question: 'How do I save my scanned items?',
          answer: `"Save" records the matched assets in your organization - sign in first.

"Share" downloads the list instead: PDF Report, Excel Spreadsheet, or CSV File.`
        },
        {
          question: 'How do I find a specific item?',
          answer: `Type part of the item number in the search box.
Or click the "Missing" tile to show only missing items.`
        }
      ]
    },
    {
      title: 'Locate',
      icon: <Search className="w-5 h-5" />,
      items: [
        {
          question: 'How do I find a specific item?',
          answer: `1. Click "Locate" on the left menu
2. Type the item number
3. Hold the trigger and walk around
4. Watch the gauge - higher = closer!`
        },
        {
          question: 'What does the gauge mean?',
          answer: `Think of it like "hot and cold":
• Red (left) = Cold - far away
• Orange / yellow (middle) = Warm - getting closer
• Green (right) = Hot - very close!`
        },
        {
          question: 'Quick way to get there from Scan?',
          answer: `Click the blue "Locate" button next to any item.
Takes you straight to the finder!`
        }
      ]
    },
    {
      title: 'Barcodes',
      icon: <ScanLine className="w-5 h-5" />,
      items: [
        {
          question: 'How do I scan regular barcodes?',
          answer: `Barcodes are a mode of the Scan screen, not a separate page:

1. Click "Scan" on the left menu
2. Switch the RFID / Barcode toggle to "Barcode"
3. Point the scanner at the barcode and pull the trigger
4. It appears in the same list as your tag reads`
        },
        {
          question: 'What types work?',
          answer: `Your scanner's barcode reader handles the common ones:
• Product barcodes (EAN/UPC)
• Code 128, Code 39, Interleaved 2 of 5
• QR Code and Data Matrix`
        }
      ]
    },
    {
      title: 'Settings',
      icon: <Settings className="w-5 h-5" />,
      items: [
        {
          question: 'How do I make it scan further?',
          answer: `On the Scan screen, click the gear icon (bottom right).
Slide "RF Power" to the right for more range.`
        },
        {
          question: 'How do I turn off the beeping?',
          answer: `Click the speaker button in the Scan toolbar.
It shows what the next click will do: "Off" mutes, "On" turns sound back on.`
        },
        {
          question: 'The item numbers are too long!',
          answer: `On the Scan screen, click the gear icon (bottom right).
Turn OFF "Show Leading Zeros" to shorten them.`
        },
        {
          question: 'How do I stop re-scanning what I just saved?',
          answer: `On the Scan screen, click the gear icon (bottom right).
Turn ON "Auto-clear after Save" and the list empties each time you save.`
        }
      ]
    },
    {
      title: 'Troubleshooting',
      icon: <AlertCircle className="w-5 h-5" />,
      items: [
        {
          question: 'Scanner won\'t connect',
          answer: `1. Make sure scanner has green light
2. Using ${recommendation.browsers}? (Required!)
3. Turn scanner off and on
4. Refresh the webpage`
        },
        {
          question: 'Not finding any items',
          answer: `1. Is scanner connected? (green "Connected")
2. Are you holding the trigger, or did you click "Start"?
3. Try the gear icon → slide RF Power right`
        },
        {
          question: 'Connection keeps dropping',
          answer: `• Charge your scanner
• Stay closer to scanner (within 30 feet)
• Keep this browser tab active`
        },
        {
          question: 'Can\'t find a specific item',
          answer: `• Double-check the item number
• Walk slowly while scanning
• Item might be behind metal (blocks signal)`
        }
      ]
    }
  ];

  return (
    <div className="h-full flex flex-col">
      {/* Header */}
      <div className="bg-white dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700 px-6 py-4">
        <div className="flex items-center">
          <HelpCircle className="w-6 h-6 text-blue-600 dark:text-blue-400 mr-3" />
          <div>
            <h1 className="text-xl font-semibold text-gray-900 dark:text-gray-100">Help</h1>
            <p className="text-sm text-gray-500 dark:text-gray-400">Quick answers to get you started</p>
          </div>
        </div>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-y-auto">
        {faqSections.map((section, sectionIndex) => (
          <div key={sectionIndex} className="mb-6">
            {/* Section Header */}
            <div className="bg-gray-50 dark:bg-gray-900 px-6 py-3 border-b border-gray-200 dark:border-gray-700">
              <div className="flex items-center space-x-2">
                <div className="text-gray-600 dark:text-gray-400">{section.icon}</div>
                <h2 className="text-sm font-semibold text-gray-900 dark:text-gray-100">{section.title}</h2>
              </div>
            </div>

            {/* FAQ Items */}
            <div className="bg-white dark:bg-gray-800">
              {section.items.map((item, itemIndex) => {
                const itemId = `${sectionIndex}-${itemIndex}`;
                return (
                  <FAQAccordion
                    key={itemId}
                    item={item}
                    isOpen={openItems.has(itemId)}
                    onToggle={() => toggleItem(itemId)}
                  />
                );
              })}
            </div>
          </div>
        ))}

        {/* Quick Tips */}
        <div className="bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 m-6 p-4 rounded-lg">
          <h3 className="text-sm font-semibold text-blue-900 dark:text-blue-100 mb-2">Remember</h3>
          <ul className="text-sm text-blue-800 dark:text-blue-200 space-y-1">
            <li>• Must use {recommendation.browsers}</li>
            <li>• Connect scanner first before anything else</li>
            <li>• Hold trigger button to scan</li>
            <li>• After you upload a list: Green = Found, Red = Missing</li>
          </ul>
        </div>
      </div>
    </div>
  );
}