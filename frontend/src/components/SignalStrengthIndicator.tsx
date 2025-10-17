import React from 'react';

interface SignalStrengthIndicatorProps {
  rssi: number;
  size?: 'sm' | 'md' | 'lg';
}

export const SignalStrengthIndicator: React.FC<SignalStrengthIndicatorProps> = ({ rssi, size = 'sm' }) => {
  // Convert RSSI to signal strength bars (1-4 bars)
  // RSSI typically ranges from -30 (excellent) to -90 (poor)
  const getSignalStrength = (rssi: number): number => {
    if (rssi >= -50) return 4; // Excellent
    if (rssi >= -60) return 3; // Good
    if (rssi >= -70) return 2; // Fair
    if (rssi >= -80) return 1; // Poor
    return 0; // No signal
  };

  const strength = getSignalStrength(rssi);
  
  // Get color based on signal strength
  const getColor = (barIndex: number): string => {
    if (barIndex > strength) return 'bg-gray-300';
    if (strength === 4) return 'bg-green-500';
    if (strength === 3) return 'bg-yellow-500';
    if (strength === 2) return 'bg-orange-500';
    return 'bg-red-500';
  };

  // Size configurations
  const sizes = {
    sm: { container: 'w-4 h-4', bars: 'w-0.5' },
    md: { container: 'w-6 h-6', bars: 'w-1' },
    lg: { container: 'w-8 h-8', bars: 'w-1.5' }
  };

  const sizeConfig = sizes[size];

  return (
    <div className={`flex items-end space-x-0.5 ${sizeConfig.container}`}>
      {[1, 2, 3, 4].map((bar) => (
        <div
          key={bar}
          className={`${sizeConfig.bars} rounded-t transition-colors ${getColor(bar)}`}
          style={{ height: `${bar * 25}%` }}
        />
      ))}
    </div>
  );
};