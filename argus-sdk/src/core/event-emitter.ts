// =============================================================================
// Argus SDK — Minimal EventEmitter
// =============================================================================
//
// Type-safe event emitter replacing Vue's watch/computed reactivity system.
// Used by all SDK modules for inter-component communication.
// =============================================================================

type EventHandler<T> = (data: T) => void;

/**
 * Minimal, type-safe event emitter.
 *
 * Replaces Vue `watch()` and `computed()` for reactive state propagation
 * in the framework-agnostic SDK.
 *
 * @example
 * ```ts
 * interface Events {
 *   statusChange: SessionStatus;
 *   violation: ViolationEvent;
 * }
 * class MyModule extends EventEmitter<Events> { ... }
 * myModule.on('statusChange', (status) => console.log(status));
 * ```
 */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export class EventEmitter<T extends Record<string, any>> {
  private _handlers = new Map<keyof T, Set<EventHandler<unknown>>>();

  /** Register an event listener. Returns an unsubscribe function. */
  on<K extends keyof T>(event: K, handler: EventHandler<T[K]>): () => void {
    if (!this._handlers.has(event)) {
      this._handlers.set(event, new Set());
    }
    const handlers = this._handlers.get(event)!;
    handlers.add(handler as EventHandler<unknown>);

    return () => { handlers.delete(handler as EventHandler<unknown>); };
  }

  /** Register a one-time event listener. */
  once<K extends keyof T>(event: K, handler: EventHandler<T[K]>): () => void {
    const wrapper: EventHandler<T[K]> = (data) => {
      unsub();
      handler(data);
    };
    const unsub = this.on(event, wrapper);
    return unsub;
  }

  /** Emit an event to all registered listeners. */
  protected emit<K extends keyof T>(event: K, data: T[K]): void {
    const handlers = this._handlers.get(event);
    if (!handlers) return;
    for (const handler of handlers) {
      try {
        handler(data);
      } catch {
        // Swallow listener errors to prevent cascading failures.
      }
    }
  }

  /** Remove all listeners for all events. */
  protected removeAllListeners(): void {
    this._handlers.clear();
  }
}
