/**
 * Mock Transport implementation for unit testing
 * Provides simulated device responses without real hardware
 */

import type { Transport, BLEMessage } from './Transport';

export interface MockTransportConfig {
  simulateErrors?: boolean;
  responseDelay?: number;
  tags?: Array<{
    epc: string;
    rssi: number;
  }>;
  batteryPercentage?: number;
  firmwareVersion?: string;
}

export class MockTransport implements Transport {
  private config: MockTransportConfig;
  private messagePort: MessagePort | null = null;
  private connected = false;
  private inventoryInterval: NodeJS.Timeout | null = null;
  private batteryInterval: NodeJS.Timeout | null = null;
  
  constructor(config: MockTransportConfig = {}) {
    this.config = {
      simulateErrors: config.simulateErrors || false,
      responseDelay: config.responseDelay || 10,
      tags: config.tags || [
        { epc: '300833B2DDD9014000000001', rssi: -45 },
        { epc: '300833B2DDD9014000000002', rssi: -52 },
        { epc: '300833B2DDD9014000000003', rssi: -68 }
      ],
      batteryPercentage: config.batteryPercentage || 85,
      firmwareVersion: config.firmwareVersion || '2.0.0-mock'
    };
  }
  
  /**
   * Simulate connection and set up MessagePort communication
   */
  async connect(): Promise<MessagePort> {
    if (this.config.simulateErrors && Math.random() < 0.1) {
      throw new Error('Mock connection failed (simulated)');
    }
    
    // Simulate connection delay
    await new Promise(resolve => setTimeout(resolve, this.config.responseDelay));
    
    // Create MessageChannel for worker communication
    const channel = new MessageChannel();
    this.messagePort = channel.port1;
    
    // Set up message handling from worker
    this.messagePort.onmessage = (event) => {
      const message = event.data as BLEMessage;
      if (message.type === 'ble:write' && message.data) {
        this.handleCommand(message.data);
      }
    };
    
    this.connected = true;
    
    // Notify worker of connection
    this.messagePort.postMessage({ 
      type: 'ble:connected' 
    } as BLEMessage);
    
    // Start simulated battery reporting
    this.startBatterySimulation();
    
    // Mock transport connected
    
    return channel.port2;
  }
  
  /**
   * Simulate disconnection
   */
  async disconnect(): Promise<void> {
    // Stop any running simulations
    if (this.inventoryInterval) {
      clearInterval(this.inventoryInterval);
      this.inventoryInterval = null;
    }
    
    if (this.batteryInterval) {
      clearInterval(this.batteryInterval);
      this.batteryInterval = null;
    }
    
    // Notify worker and close port
    if (this.messagePort) {
      this.messagePort.postMessage({ 
        type: 'ble:disconnected' 
      } as BLEMessage);
      this.messagePort.close();
      this.messagePort = null;
    }
    
    this.connected = false;
    // Mock transport disconnected
  }
  
  /**
   * Check if connected
   */
  isConnected(): boolean {
    return this.connected;
  }
  
  /**
   * Get transport type
   */
  getType(): string {
    return 'mock';
  }
  
  /**
   * Handle commands from worker
   */
  private handleCommand(data: Uint8Array): void {
    // Parse command (CS108 format)
    if (data.length < 10) return;
    
    const command = (data[8] << 8) | data[9];
    
    // Simulate responses based on command
    setTimeout(() => {
      switch (command) {
        case 0xA000: // GET_BATTERY_VOLTAGE
          this.sendBatteryResponse();
          break;
          
        case 0xA001: // START_BATTERY_REPORTING
          this.sendAck(command);
          break;
          
        case 0xA100: // GET_TRIGGER_STATE
          this.sendTriggerResponse(false);
          break;
          
        case 0xA004: // SET_TRIGGER_ABORT_MODE
        case 0xA101: // START_TRIGGER_REPORTING
          this.sendAck(command);
          break;
          
        case 0x4003: // START_INVENTORY
          this.sendAck(command);
          this.startInventorySimulation();
          break;
          
        case 0x4004: // STOP_INVENTORY
          this.stopInventorySimulation();
          // Note: CS108 doesn't always respond to STOP_INVENTORY
          break;
          
        case 0xE4: // BARCODE_SCAN
          this.sendBarcodeResponse();
          break;
          
        default:
          // Generic ACK for other commands
          this.sendAck(command);
      }
    }, this.config.responseDelay);
  }
  
  /**
   * Send a generic ACK response
   */
  private sendAck(command: number): void {
    const response = new Uint8Array(10);
    response[0] = 0xA7;
    response[1] = 0xB3;
    response[2] = 2; // Payload length
    response[3] = 0xC2; // Module ID
    response[8] = (command >> 8) & 0xFF;
    response[9] = command & 0xFF;
    
    this.sendResponse(response);
  }
  
  /**
   * Send battery voltage response
   */
  private sendBatteryResponse(): void {
    const voltage = Math.floor(this.config.batteryPercentage! * 40.95); // Convert percentage to voltage
    const response = new Uint8Array(12);
    response[0] = 0xA7;
    response[1] = 0xB3;
    response[2] = 4; // Payload length
    response[3] = 0xD9; // Notification module
    response[8] = 0xA0;
    response[9] = 0x00;
    response[10] = (voltage >> 8) & 0xFF;
    response[11] = voltage & 0xFF;
    
    this.sendResponse(response);
  }
  
  /**
   * Send trigger state response
   */
  private sendTriggerResponse(pressed: boolean): void {
    const response = new Uint8Array(11);
    response[0] = 0xA7;
    response[1] = 0xB3;
    response[2] = 3; // Payload length
    response[3] = 0xD9; // Notification module
    response[8] = 0xA1;
    response[9] = 0x00;
    response[10] = pressed ? 1 : 0;
    
    this.sendResponse(response);
  }
  
  /**
   * Send barcode response
   */
  private sendBarcodeResponse(): void {
    const barcode = '1234567890';
    const response = new Uint8Array(10 + barcode.length + 1);
    response[0] = 0xA7;
    response[1] = 0xB3;
    response[2] = barcode.length + 3;
    response[3] = 0x6A; // Barcode module
    response[8] = 0xE1; // Barcode data notification
    response[9] = 0x00;
    
    // Add barcode data
    for (let i = 0; i < barcode.length; i++) {
      response[10 + i] = barcode.charCodeAt(i);
    }
    response[10 + barcode.length] = 0; // Null terminator
    
    this.sendResponse(response);
  }
  
  /**
   * Start simulated inventory
   */
  private startInventorySimulation(): void {
    if (this.inventoryInterval) return;
    
    let tagIndex = 0;
    this.inventoryInterval = setInterval(() => {
      const tag = this.config.tags![tagIndex % this.config.tags!.length];
      
      // Create inventory packet (simplified)
      const epcBytes = this.hexToBytes(tag.epc);
      const packet = new Uint8Array(10 + 2 + epcBytes.length + 1);
      
      packet[0] = 0xA7;
      packet[1] = 0xB3;
      packet[2] = 2 + epcBytes.length + 1;
      packet[3] = 0xC2; // RFID module
      packet[8] = 0x81; // Inventory notification
      packet[9] = 0x00;
      
      // PC word (96-bit tag)
      packet[10] = 0x30;
      packet[11] = 0x00;
      
      // EPC
      for (let i = 0; i < epcBytes.length; i++) {
        packet[12 + i] = epcBytes[i];
      }
      
      // RSSI
      packet[12 + epcBytes.length] = Math.abs(tag.rssi);
      
      this.sendResponse(packet);
      tagIndex++;
      
    }, 100); // Send tag every 100ms
  }
  
  /**
   * Stop simulated inventory
   */
  private stopInventorySimulation(): void {
    if (this.inventoryInterval) {
      clearInterval(this.inventoryInterval);
      this.inventoryInterval = null;
    }
  }
  
  /**
   * Start battery level simulation
   */
  private startBatterySimulation(): void {
    if (this.batteryInterval) return;
    
    this.batteryInterval = setInterval(() => {
      // Slowly drain battery
      this.config.batteryPercentage = Math.max(0, this.config.batteryPercentage! - 0.1);
      this.sendBatteryResponse();
    }, 5000); // Every 5 seconds
  }
  
  /**
   * Send response to worker
   */
  private sendResponse(data: Uint8Array): void {
    if (this.messagePort) {
      this.messagePort.postMessage({
        type: 'ble:data',
        data
      } as BLEMessage);
    }
  }
  
  /**
   * Convert hex string to byte array
   */
  private hexToBytes(hex: string): Uint8Array {
    const bytes = new Uint8Array(hex.length / 2);
    for (let i = 0; i < hex.length; i += 2) {
      bytes[i / 2] = parseInt(hex.substr(i, 2), 16);
    }
    return bytes;
  }
}