import React from 'react';

interface TrakRFLogoProps {
  className?: string;
}

export const TrakRFLogo: React.FC<TrakRFLogoProps> = ({ className = "w-8 h-8" }) => {
  return (
    <div className={`bg-blue-600 rounded-lg flex items-center justify-center text-white font-bold ${className}`}>
      T
    </div>
  );
};