// Web Bluetooth API type definitions
// Augment the navigator interface
interface Navigator {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  bluetooth: any; // Complex Web Bluetooth API - proper types defined in module below
}

// Augment the Window interface for testing utilities
interface Window {
  __webBluetoothBridged?: boolean;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  __TRANSPORT_MANAGER__?: any; // TransportManager is a private class, exposed for testing
  __ZUSTAND_STORES__?: {
    // These are the actual Zustand store hook functions
    // Using unknown since the exact signatures are complex and only for testing
    deviceStore: unknown;
    tagStore: unknown;
    uiStore: unknown;
    settingsStore: unknown;
    packetStore: unknown;
    barcodeStore: unknown;
  };
}

declare module '@/types/web-bluetooth' {
  export interface BluetoothDevice {
  id: string;
  name?: string;
  gatt?: BluetoothRemoteGATTServer;
  watchingAdvertisements: boolean;
  watchAdvertisements(): Promise<void>;
  unwatchAdvertisements(): void;
  addEventListener(type: string, listener: EventListenerOrEventListenerObject, options?: boolean | AddEventListenerOptions): void;
  removeEventListener(type: string, listener: EventListenerOrEventListenerObject, options?: boolean | EventListenerOptions): void;
}

export interface BluetoothRemoteGATTServer {
  device: BluetoothDevice;
  connected: boolean;
  connect(): Promise<BluetoothRemoteGATTServer>;
  disconnect(): void;
  getPrimaryService(service: string | number): Promise<BluetoothRemoteGATTService>;
  getPrimaryServices(service?: string | number): Promise<BluetoothRemoteGATTService[]>;
}

export interface BluetoothRemoteGATTService {
  device: BluetoothDevice;
  uuid: string;
  isPrimary: boolean;
  getCharacteristic(characteristic: string | number): Promise<BluetoothRemoteGATTCharacteristic>;
  getCharacteristics(characteristic?: string | number): Promise<BluetoothRemoteGATTCharacteristic[]>;
  getIncludedService(service: string | number): Promise<BluetoothRemoteGATTService>;
  getIncludedServices(service?: string | number): Promise<BluetoothRemoteGATTService[]>;
}

export interface BluetoothRemoteGATTCharacteristic {
  service: BluetoothRemoteGATTService;
  uuid: string;
  properties: BluetoothCharacteristicProperties;
  value?: DataView;
  getDescriptor(descriptor: string | number): Promise<BluetoothRemoteGATTDescriptor>;
  getDescriptors(descriptor?: string | number): Promise<BluetoothRemoteGATTDescriptor[]>;
  readValue(): Promise<DataView>;
  writeValue(value: BufferSource): Promise<void>;
  writeValueWithResponse(value: BufferSource): Promise<void>;
  writeValueWithoutResponse(value: BufferSource): Promise<void>;
  startNotifications(): Promise<BluetoothRemoteGATTCharacteristic>;
  stopNotifications(): Promise<BluetoothRemoteGATTCharacteristic>;
  addEventListener(type: string, listener: EventListenerOrEventListenerObject, options?: boolean | AddEventListenerOptions): void;
  removeEventListener(type: string, listener: EventListenerOrEventListenerObject, options?: boolean | EventListenerOptions): void;
}

export interface BluetoothRemoteGATTDescriptor {
  characteristic: BluetoothRemoteGATTCharacteristic;
  uuid: string;
  value?: DataView;
  readValue(): Promise<DataView>;
  writeValue(value: BufferSource): Promise<void>;
}

export interface BluetoothCharacteristicProperties {
  broadcast: boolean;
  read: boolean;
  writeWithoutResponse: boolean;
  write: boolean;
  notify: boolean;
  indicate: boolean;
  authenticatedSignedWrites: boolean;
  reliableWrite: boolean;
  writableAuxiliaries: boolean;
}

export interface BluetoothRequestDeviceOptions {
  filters?: BluetoothRequestDeviceFilter[];
  optionalServices?: (string | number)[];
  acceptAllDevices?: boolean;
}

export interface BluetoothRequestDeviceFilter {
  services?: (string | number)[];
  name?: string;
  namePrefix?: string;
  manufacturerId?: number;
  serviceData?: { [key: string]: DataView };
}

export interface BluetoothNavigator {
  getAvailability(): Promise<boolean>;
  requestDevice(options?: BluetoothRequestDeviceOptions): Promise<BluetoothDevice>;
  getDevices(): Promise<BluetoothDevice[]>;
  addEventListener(type: string, listener: EventListenerOrEventListenerObject, options?: boolean | AddEventListenerOptions): void;
  removeEventListener(type: string, listener: EventListenerOrEventListenerObject, options?: boolean | EventListenerOptions): void;
}
}