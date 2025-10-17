import React, { useState, useEffect } from 'react';
import { usePacketStore, useDeviceStore } from '@/stores';

export default function DebugPanel() {
  // Use proper hook pattern for reactive updates
  const packetLog = usePacketStore((state) => state.packetLog);
  const clearPacketLog = usePacketStore((state) => state.clearPacketLog);
  const [filter, setFilter] = useState<'all' | 'command' | 'response' | 'notification'>('all');
  const [expanded, setExpanded] = useState(true);
  const [showRawData, setShowRawData] = useState(false);
  const [triggerState, setTriggerState] = useState(useDeviceStore.getState().triggerState);
  const [copyToast, setCopyToast] = useState<{show: boolean, message: string}>({show: false, message: ''});

  // Subscribe to trigger state changes and packet log updates
  useEffect(() => {
    // Subscribe to device store for trigger state
    const unsubDeviceStore = useDeviceStore.subscribe((state) => {
      setTriggerState(state.triggerState);
    });
    
    return () => {
      unsubDeviceStore();
    };
  }, []);
  
  // Use memoized filtering for better performance
  // Filter packets based on selected type and whether to show raw notifications
  // This is more efficient than chaining multiple filter operations
  const filteredPackets = React.useMemo(() => {
    return packetLog.filter(packet => {
      // First check if the packet type matches the filter
      if (filter !== 'all' && packet.type !== filter) {
        return false;
      }
      
      // Then check if we should hide raw notifications
      if (!showRawData && packet.description) {
        const desc = packet.description;
        if (desc.startsWith('Raw ') || desc.startsWith('RAW ') || desc.startsWith('Reassembled ')) {
          return false;
        }
      }
      
      return true;
    });
  }, [packetLog, filter, showRawData]);
  
  // Function to get CSS color based on packet type
  const getPacketTypeColor = (type: string) => {
    switch (type) {
      case 'command':
        return 'text-blue-600';
      case 'response': 
        return 'text-green-600';
      case 'notification':
        return 'text-orange-600';
      default:
        return 'text-gray-600';
    }
  };
  
  // Get packet background color
  const getRowBackground = (index: number) => {
    return index % 2 === 0 ? 'bg-gray-50' : 'bg-white';
  };
  
  // Helper to create a color legend item
  const ColorLegendItem = ({ color, text }: { color: string; text: string }) => (
    <div className="flex items-center mr-3 text-xs">
      <span className={`inline-block w-3 h-3 ${color} mr-1 rounded-sm`}></span>
      <span>{text}</span>
    </div>
  );

  // Effect to hide toast after delay
  useEffect(() => {
    if (copyToast.show) {
      const timer = setTimeout(() => {
        setCopyToast({show: false, message: ''});
      }, 2000);
      return () => clearTimeout(timer);
    }
  }, [copyToast.show]);

  // Function to convert packet log to CSV and copy to clipboard
  const copyAsCsv = () => {
    // Generate CSV header
    let csvContent = "Timestamp,Type,Length,Description,Data\n";
    
    // Add each packet as a row
    packetLog.forEach(packet => {
      const timestamp = packet.timestamp 
        ? new Date(packet.timestamp).toLocaleTimeString([], {hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false}) + 
          "." + String(packet.timestamp % 1000).padStart(3, '0')
        : "";
      const type = packet.type || "";
      const length = packet.length !== undefined ? packet.length : "";
      // Escape quotes in description and data fields
      const description = packet.description ? `"${packet.description.replace(/"/g, '""')}"` : "";
      const data = packet.data ? `"${packet.data.replace(/"/g, '""')}"` : "";
      
      // Add row to CSV
      csvContent += `${timestamp},${type},${length},${description},${data}\n`;
    });
    
    // Copy to clipboard
    navigator.clipboard.writeText(csvContent)
      .then(() => {
        console.debug('Debug packet log copied to clipboard as CSV');
        setCopyToast({show: true, message: 'Copied as CSV!'});
      })
      .catch(err => {
        console.error('Failed to copy debug log as CSV:', err);
        setCopyToast({show: true, message: 'Copy failed!'});
      });
  };
  
  // Function to copy packet log as JSON for coding tools
  const copyAsJson = () => {
    const packetJson = JSON.stringify(packetLog, null, 2);
    navigator.clipboard.writeText(packetJson)
      .then(() => {
        console.debug('Debug packet log copied to clipboard as JSON');
        setCopyToast({show: true, message: 'Copied as JSON!'});
      })
      .catch(err => {
        console.error('Failed to copy debug log as JSON:', err);
        setCopyToast({show: true, message: 'Copy failed!'});
      });
  };

  return (
    <div className="mt-4 border border-gray-300 rounded-md overflow-hidden shadow-lg">
      <div className="bg-gray-100 px-4 py-2 flex justify-between items-center">
        <button 
          onClick={() => setExpanded(!expanded)}
          className="text-sm font-medium text-gray-700 flex items-center"
        >
          <span className={`mr-2 transition-transform duration-200 ${expanded ? 'rotate-90' : ''}`}>▶</span>
          Debug Panel - Packet Log ({packetLog.length})
        </button>
        <div className="flex space-x-2">
          <select 
            value={filter}
            onChange={(e) => setFilter(e.target.value as 'all' | 'command' | 'response' | 'notification')}
            className="text-xs px-2 py-1 border border-gray-300 rounded-md"
          >
            <option value="all">All Types</option>
            <option value="command">Commands</option>
            <option value="response">Responses</option>
            <option value="notification">Notifications</option>
          </select>
          <button 
            onClick={() => setShowRawData(!showRawData)}
            className={`text-xs px-2 py-1 ${showRawData ? 'bg-blue-500 text-white' : 'bg-gray-200 text-gray-800'} rounded-md hover:opacity-90`}
          >
            {showRawData ? 'Hide Raw' : 'Show Raw'}
          </button>
          <div className="flex space-x-1">
            <button 
              onClick={copyAsCsv}
              className="text-xs px-2 py-1 bg-green-500 text-white rounded-l-md hover:bg-green-600 active:bg-green-700 transition-colors"
              title="Copy as CSV for spreadsheets"
              disabled={packetLog.length === 0}
            >
              CSV
            </button>
            <button 
              onClick={copyAsJson}
              className="text-xs px-2 py-1 bg-green-500 text-white rounded-r-md hover:bg-green-600 active:bg-green-700 transition-colors"
              title="Copy as JSON for coding tools"
              disabled={packetLog.length === 0}
            >
              JSON
            </button>
          </div>
          <button 
            onClick={() => {
              // Call the store action to clear packets
              clearPacketLog();
              // Provide visual confirmation
              console.debug('Debug packet log cleared');
            }}
            className="text-xs px-2 py-1 bg-red-500 text-white rounded-md hover:bg-red-600 active:bg-red-700 transition-colors"
          >
            Clear
          </button>
        </div>
      </div>
      
      {expanded && (
        <>
          {/* Color legend & Trigger state */}
          <div className="bg-gray-50 p-2 border-t border-b border-gray-200">
            <div className="flex flex-wrap mb-2">
              <ColorLegendItem color="bg-blue-600" text="Sync (0-1)" />
              <ColorLegendItem color="bg-indigo-600" text="Length (2)" />
              <ColorLegendItem color="bg-purple-600" text="Addr (3)" />
              <ColorLegendItem color="bg-teal-600" text="Port (4)" />
              <ColorLegendItem color="bg-green-600" text="Res (5)" />
              <ColorLegendItem color="bg-orange-600" text="CRC (6-7)" />
              <ColorLegendItem color="bg-red-600" text="CMD (8-9)" />
              <ColorLegendItem color="bg-gray-700" text="Payload" />
            </div>
            
            {/* Trigger button state indicator */}
            <div className="flex items-center text-xs">
              <span className="font-medium mr-2">Trigger Button:</span>
              <span 
                className="px-2 py-1 rounded-md text-xs font-medium"
                style={{
                  backgroundColor: triggerState ? '#ffcccc' : '#ccffcc',
                  borderColor: triggerState ? '#ff6666' : '#66ff66',
                  color: triggerState ? '#990000' : '#009900',
                }}
              >
                {triggerState ? 'DOWN' : 'UP'}
              </span>
            </div>
          </div>
          
          <div className="overflow-auto max-h-[300px]">
            <table className="min-w-full divide-y divide-gray-200 table-fixed">
            <thead className="bg-gray-50">
              <tr>
                <th scope="col" className="px-3 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider w-24">
                  Time
                </th>
                <th scope="col" className="px-3 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider w-16">
                  Type
                </th>
                <th scope="col" className="px-3 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider w-16">
                  Length
                </th>
                <th scope="col" className="px-3 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                  Description
                </th>
                <th scope="col" className="px-3 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                  Data
                </th>
              </tr>
            </thead>
            <tbody className="bg-white divide-y divide-gray-200">
              {filteredPackets.length === 0 ? (
                <tr>
                  <td colSpan={5} className="px-3 py-2 text-center text-sm text-gray-500">
                    No packets logged
                  </td>
                </tr>
              ) : (
                filteredPackets.map((packet, index) => (
                  <tr key={`${packet.timestamp}-${index}`} className={getRowBackground(index)}>
                    <td className="px-3 py-2 text-xs text-gray-500">
                      {new Date(packet.timestamp || 0).toLocaleTimeString([], {hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false})}
                      <span className="text-gray-400">.{String((packet.timestamp || 0) % 1000).padStart(3, '0')}</span>
                    </td>
                    <td className={`px-3 py-2 text-xs font-medium ${getPacketTypeColor(packet.type)}`}>
                      {packet.type === 'command' ? 'CMD' : 
                       packet.type === 'response' ? 'RESP' : 
                       packet.type === 'notification' ? 'NOTF' : 
                       String(packet.type).substring(0,4).toUpperCase()}
                    </td>
                    <td className="px-3 py-2 text-xs text-gray-500 text-center">
                      {packet.length !== undefined ? packet.length : '-'}
                    </td>
                    <td className={`px-3 py-2 text-xs ${packet.description?.startsWith('ERROR:') ? 'text-red-600 font-medium' : 'text-gray-900'}`}>
                      {packet.description || '-'}
                    </td>
                    <td className="px-3 py-2 text-xs font-mono text-gray-700 break-all">
                      {packet.data ? (
                        <span>
                          {/* Header parts with different colors */}
                          <span className="text-blue-600">{packet.data.substring(0, 5)}</span> {/* Bytes 0-1: A7 B3 (sync bytes) */}
                          <span className="text-indigo-600">{packet.data.substring(5, 8)}</span> {/* Byte 2: Length */}
                          <span className="text-purple-600">{packet.data.substring(8, 11)}</span> {/* Byte 3: Address */}
                          <span className="text-teal-600">{packet.data.substring(11, 14)}</span> {/* Byte 4: Port */}
                          <span className="text-green-600">{packet.data.substring(14, 17)}</span> {/* Byte 5: Res */}
                          <span className="text-orange-600">{packet.data.substring(17, 23)}</span> {/* Bytes 6-7: CRC */}
                          {packet.data.length > 23 && (
                            <>
                              <span className="text-red-600">{packet.data.substring(23, 29)}</span> {/* Bytes 8-9: Command */}
                              <span className="text-gray-700">{packet.data.substring(29)}</span> {/* Rest of packet - payload */}
                            </>
                          )}
                        </span>
                      ) : '-'}
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
        </>
      )}
      
      {/* Toast notification for copy actions */}
      {copyToast.show && (
        <div className="fixed bottom-4 right-4 bg-gray-800 text-white px-4 py-2 rounded-md shadow-lg z-50 flex items-center animate-popup">
          <svg 
            xmlns="http://www.w3.org/2000/svg" 
            className="h-5 w-5 mr-2 text-green-400" 
            viewBox="0 0 20 20" 
            fill="currentColor"
          >
            <path 
              fillRule="evenodd" 
              d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" 
              clipRule="evenodd" 
            />
          </svg>
          {copyToast.message}
        </div>
      )}
    </div>
  );
}