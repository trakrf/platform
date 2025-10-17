import React from 'react';
import { ReaderDisconnectedIcon } from './ReaderIcons';

interface ConnectIconProps {
  className?: string;
}

export const ConnectIcon: React.FC<ConnectIconProps> = ({ className = "w-5 h-5" }) => {
  return <ReaderDisconnectedIcon className={className} />;
};