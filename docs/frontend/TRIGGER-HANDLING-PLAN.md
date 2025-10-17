# Trigger Handling Resilience Plan

## Current State Analysis

### Trigger Event Flow
1. **Hardware → Bridge**: Physical trigger press/release on CS108
2. **Bridge → Worker**: WebSocket message with trigger state
3. **Worker → CS108Reader**: `handleNotificationEvent` processes TRIGGER_STATE_CHANGED
4. **CS108Reader → Commands**: Calls `startScanning()` or `stopScanning()` based on state
5. **CS108Reader → Store**: Emits state changes via `postWorkerEvent`
6. **Store → UI**: React components respond to store updates

### Identified Pain Points

#### 1. Race Condition: Concurrent Stop Operations
- **Issue**: Multiple trigger releases or mode changes can call `stopScanning()` simultaneously
- **Current mitigation**: `isStoppingScanning` flag (basic but effective)
- **Remaining risk**: Flag isn't atomic, potential for edge cases

#### 2. Race Condition: Mode Change During Scan
- **Issue**: Tab navigation triggers `setMode()` while scanning is active
- **Current mitigation**: `executeSetMode()` aborts active sequences
- **Remaining risk**: Abort timing can conflict with stop operations

#### 3. Unpredictable Trigger Timing
- **Issue**: Users can press/release trigger at any time, including:
  - During mode transitions (BUSY state)
  - During connection/disconnection
  - Rapidly in succession
  - While commands are executing
- **Current mitigation**: State checks in trigger handler
- **Remaining risk**: No debouncing, no queueing, no rate limiting

#### 4. Command Collision at Hardware Level
- **Issue**: Sending commands too quickly causes CS108 to restart
- **Evidence**: Bridge stability issues, BLE disconnections
- **Current mitigation**: None
- **Impact**: Complete connection loss requiring manual recovery

#### 5. Doubled Mode Changes
- **Issue**: Tab navigation was triggering setMode twice
- **Fixed**: Removed redundant `setActiveTab()` calls from screen components
- **Remaining risk**: Other sources of duplicate events

## Solution Design Options

### Option 1: Command Queue with Rate Limiting
**Approach**: Queue all trigger events and process them sequentially with minimum spacing

**Pros**:
- Prevents command collisions
- Guarantees order of operations
- Can implement priority (stop > start)

**Cons**:
- Adds latency to trigger response
- Complex queue management
- May feel unresponsive to users

**Implementation Complexity**: High

### Option 2: State Machine with Guard Conditions
**Approach**: Implement formal state machine with allowed transitions and guards

**Pros**:
- Clear, predictable behavior
- Easy to reason about
- Prevents invalid operations

**Cons**:
- Rigid, may block valid operations
- Requires comprehensive state mapping
- May need UI feedback for blocked operations

**Implementation Complexity**: Medium

### Option 3: Debouncing with Smart Coalescing
**Approach**: Debounce trigger events and coalesce rapid changes

**Pros**:
- Simple to implement
- Reduces command traffic
- Handles rapid trigger presses well

**Cons**:
- Adds small latency (100-200ms)
- May miss intentional rapid presses
- Doesn't solve all race conditions

**Implementation Complexity**: Low

### Option 4: Hybrid - Minimal Protection Layer
**Approach**: Combine lightweight solutions for maximum benefit with minimal complexity

**Components**:
1. **Trigger debouncing** (100ms) - prevent rapid toggles
2. **Command spacing** (50ms minimum) - prevent hardware overload
3. **State guards** - only allow operations in valid states
4. **Operation flags** - prevent concurrent operations (existing)
5. **Clear error recovery** - handle and recover from failures

**Pros**:
- Balanced approach
- Incremental implementation possible
- Maintains responsiveness
- Minimal added complexity

**Cons**:
- Not a "perfect" solution
- Still some edge cases possible

**Implementation Complexity**: Low-Medium

## Recommended Approach: Eventual Consistency Model

### Core Principle
**Physical trigger state is the source of truth**. The scanning state should eventually converge to match the physical trigger state, even if there are temporary mismatches due to debouncing or command delays.

### Rationale
- Prevents getting out of sync with hardware
- Handles rapid trigger events gracefully
- Self-correcting system that always converges
- Simple to reason about and implement
- Aligns with NoSQL eventual consistency patterns

## Implementation Plan

### Phase 1: Eventual Consistency Core (Priority: HIGH)
1. **Track physical trigger state** in CS108Reader (worker thread)
   - Add `physicalTriggerState: boolean` instance variable
   - Update on every TRIGGER_STATE_CHANGED event
   - This represents the current hardware state
   - Note: Worker thread can't access stores (different thread) - needs local state
   - Worker maintains authoritative hardware state, store maintains UI state

2. **Add reconciliation checks** after operations
   - After `startScanning()` completes: if `physicalTriggerState === false`, immediately call `stopScanning()`
   - After `stopScanning()` completes: if `physicalTriggerState === true`, immediately call `startScanning()`
   - Log all reconciliation actions for debugging

3. **Light debouncing** to prevent command hammering
   - Keep the 100ms debounce to prevent rapid toggles
   - But always update `physicalTriggerState` immediately
   - Debounce only affects when we act on state changes

4. **Command spacing** in CommandManager
   - Track last command timestamp
   - Enforce 50ms minimum spacing
   - Prevents overwhelming the hardware

### Example Implementation (Pseudocode)
```typescript
class CS108Reader {
  private physicalTriggerState = false; // Track hardware state

  handleNotificationEvent(event) {
    if (event.type === 'TRIGGER_STATE_CHANGED') {
      // Always update physical state immediately
      this.physicalTriggerState = event.payload.pressed;

      // Debounce actual operations
      if (Date.now() - this.lastTriggerTime < this.triggerDebounceMs) {
        logger.debug('Trigger event debounced, but state tracked');
        return;
      }
      this.lastTriggerTime = Date.now();

      // Act on state change
      if (this.physicalTriggerState && readerState === READY) {
        await this.startScanning();
      } else if (!this.physicalTriggerState && readerState === SCANNING) {
        await this.stopScanning();
      }
    }
  }

  async startScanning() {
    // ... execute start commands ...

    // Reconciliation: Check if trigger was released while we were starting
    if (!this.physicalTriggerState) {
      logger.debug('Trigger released during start, reconciling by stopping');
      await this.stopScanning();
    }
  }

  async stopScanning() {
    // ... execute stop commands ...

    // Reconciliation: Check if trigger was pressed while we were stopping
    if (this.physicalTriggerState) {
      logger.debug('Trigger pressed during stop, reconciling by starting');
      await this.startScanning();
    }
  }
}
```

### Phase 2: Robustness (Priority: MEDIUM)
1. **Add operation timeouts**
   - Timeout for startScanning (2s)
   - Timeout for stopScanning (3s)
   - Timeout for setMode (5s)
   - Recovery action on timeout

2. **Improve error recovery**
   - Auto-retry failed operations (with backoff)
   - Clear error states automatically
   - Emit user-friendly error events

3. **Add operation queuing**
   - Queue for setMode operations
   - Priority queue for stop operations
   - Maximum queue size with overflow handling

### Phase 3: Monitoring (Priority: LOW)
1. **Add metrics collection**
   - Track trigger event frequency
   - Track command success/failure rates
   - Track state transition times

2. **Add debug mode**
   - Verbose logging of all decisions
   - Event timeline visualization
   - Command flow tracing

## Key Benefits of Eventual Consistency Approach
- **Self-healing**: System automatically corrects mismatches
- **Resilient**: Handles rapid trigger events without losing sync
- **Simple**: No complex state machines or queues
- **Predictable**: Physical state always wins
- **Debuggable**: Clear log trail of reconciliation actions

## Edge Cases to Consider
1. **Infinite loop protection**: Add recursion depth limit to prevent start→stop→start loops
2. **Mode changes**: Clear physicalTriggerState when changing modes
3. **Disconnection**: Reset physicalTriggerState on disconnect
4. **Error states**: Don't reconcile when in ERROR state

## Success Metrics
- No "Command already active" errors during normal operation
- No CS108 restarts due to command timing
- Trigger response time < 200ms (perceived as instant)
- Zero data loss during mode transitions
- Clear user feedback when operations are blocked
- Scanning state always matches physical trigger state (within 1 second)

## Testing Strategy
1. **Unit tests**: Test each protection mechanism in isolation
2. **Integration tests**: Test trigger sequences with timing variations
3. **Stress tests**: Rapid trigger pressing, rapid tab switching
4. **Hardware tests**: Verify with real CS108 device

## Migration Notes
- All changes should be backward compatible
- Existing tests should continue to pass
- Can be deployed incrementally (feature flags if needed)
- Monitor production metrics after each phase

## Timeline Estimate
- Phase 1: 2-3 hours (high confidence)
- Phase 2: 3-4 hours (medium confidence)
- Phase 3: 2-3 hours (low priority, can defer)

## Risks and Mitigations
| Risk | Impact | Mitigation |
|------|---------|------------|
| Debouncing feels sluggish | User dissatisfaction | Tune timing, add visual feedback |
| Queue grows unbounded | Memory issues | Set max queue size, drop oldest |
| Hardware behaves differently | Commands fail | Test with real device frequently |
| Edge cases remain | Occasional failures | Add comprehensive logging |