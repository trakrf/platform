/**
 * Bridge Transport implementation for testing via ble-mcp-test WebSocket server
 * Allows testing without Chrome's Web Bluetooth requirements
 */

import type { Transport, BLEMessage } from './Transport';

export interface BridgeTransportConfig {
  url?: string;
  deviceType?: string;
  reconnectAttempts?: number;
  reconnectDelay?: number;
}

export class BridgeTransport implements Transport {
  private config: Required<BridgeTransportConfig>;
  private websocket: WebSocket | null = null;
  private messagePort: MessagePort | null = null;
  private connected = false;
  
  constructor(config: BridgeTransportConfig = {}) {
    this.config = {
      url: config.url || 'ws://localhost:8765',
      deviceType: config.deviceType || 'CS108',
      reconnectAttempts: config.reconnectAttempts || 3,
      reconnectDelay: config.reconnectDelay || 1000
    };
  }
  
  /**
   * Connect to bridge server and set up MessagePort communication
   */
  async connect(): Promise<MessagePort> {
    return new Promise((resolve, reject) => {
      let attempts = 0;
      
      const attemptConnection = () => {
        attempts++;
        // Connecting to bridge server
        
        try {
          this.websocket = new WebSocket(this.config.url);
          
          this.websocket.onopen = () => {
            console.info('Bridge connected');
            
            // Send connect command to bridge
            this.websocket!.send(JSON.stringify({
              type: 'connect',
              device: this.config.deviceType
            }));
            
            // Create MessageChannel for worker communication
            const channel = new MessageChannel();
            this.messagePort = channel.port1;
            
            // Set up message handling from worker
            this.messagePort.onmessage = (event) => {
              const message = event.data as BLEMessage;
              if (message.type === 'ble:write' && message.data) {
                // Forward write to bridge
                if (this.websocket?.readyState === WebSocket.OPEN) {
                  this.websocket.send(JSON.stringify({
                    type: 'write',
                    data: Array.from(message.data)
                  }));
                }
              }
            };
            
            this.connected = true;
            
            // Notify worker of connection
            this.messagePort.postMessage({ 
              type: 'ble:connected' 
            } as BLEMessage);
            
            resolve(channel.port2);
          };
          
          this.websocket.onmessage = (event) => {
            try {
              const message = JSON.parse(event.data);
              
              // Handle different message types from bridge
              if (message.type === 'notification' && message.data) {
                // Convert array back to Uint8Array and forward to worker
                const data = new Uint8Array(message.data);
                if (this.messagePort) {
                  this.messagePort.postMessage({
                    type: 'ble:data',
                    data
                  } as BLEMessage);
                }
              } else if (message.type === 'error') {
                console.error('Bridge error:', message.error);
                if (this.messagePort) {
                  this.messagePort.postMessage({
                    type: 'ble:error',
                    error: message.error
                  } as BLEMessage);
                }
              } else if (message.type === 'connected') {
                // Bridge confirmed BLE connection
              } else if (message.type === 'disconnected') {
                // Bridge reported BLE disconnection
                this.handleDisconnect();
              }
            } catch (error) {
              console.error('Error parsing bridge message:', error);
            }
          };
          
          this.websocket.onerror = (error) => {
            console.error('Bridge WebSocket error:', error);
            
            if (attempts < this.config.reconnectAttempts) {
              // Retrying connection
              setTimeout(attemptConnection, this.config.reconnectDelay);
            } else {
              reject(new Error(`Failed to connect to bridge after ${attempts} attempts`));
            }
          };
          
          this.websocket.onclose = () => {
            // Bridge WebSocket closed
            this.handleDisconnect();
          };
          
        } catch (error) {
          console.error('Error creating WebSocket:', error);
          
          if (attempts < this.config.reconnectAttempts) {
            setTimeout(attemptConnection, this.config.reconnectDelay);
          } else {
            reject(error);
          }
        }
      };
      
      attemptConnection();
    });
  }
  
  /**
   * Disconnect from bridge
   */
  async disconnect(): Promise<void> {
    if (this.websocket) {
      // Send disconnect command to bridge
      if (this.websocket.readyState === WebSocket.OPEN) {
        this.websocket.send(JSON.stringify({
          type: 'disconnect'
        }));
      }
      
      // Close WebSocket
      this.websocket.close();
      this.websocket = null;
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
  }
  
  /**
   * Check if connected
   */
  isConnected(): boolean {
    return this.connected && this.websocket?.readyState === WebSocket.OPEN;
  }
  
  /**
   * Get transport type
   */
  getType(): string {
    return 'bridge';
  }
  
  /**
   * Handle disconnection
   */
  private handleDisconnect(): void {
    this.connected = false;
    
    // Notify worker
    if (this.messagePort) {
      this.messagePort.postMessage({ 
        type: 'ble:disconnected' 
      } as BLEMessage);
    }
    
    // Clean up
    this.websocket = null;
    if (this.messagePort) {
      this.messagePort.close();
      this.messagePort = null;
    }
  }
}