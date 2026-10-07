// =============================================================================
// Argus SDK — Browser Integrity Monitor
// =============================================================================
//
// Detects security violations: tab switches, copy-paste, print screen,
// DevTools, fullscreen exit, and external displays.
// Extracted from useBrowserIntegrity — already near-pure JS.
// =============================================================================

import { EventType, Severity, EventSource } from '../types';
import { EventEmitter } from '../core/event-emitter';

interface BrowserEvents {
  violation: {
    eventType: EventType;
    severity: Severity;
    source: EventSource;
    label: string;
    confidence: number;
  };
  stateChange: BrowserIntegrityState;
}

/** Browser integrity state snapshot. */
export interface BrowserIntegrityState {
  isFullscreen: boolean;
  isVisible: boolean;
  devToolsOpen: boolean;
  externalDisplayCount: number;
  tabSwitchCount: number;
  copyAttemptCount: number;
  printScreenCount: number;
}

/**
 * Browser integrity monitor — detects security violations.
 *
 * Listens for browser events that indicate potential exam cheating
 * and emits violation events for each detection.
 */
export class BrowserIntegrityMonitor extends EventEmitter<BrowserEvents> {
  private _state: BrowserIntegrityState = {
    isFullscreen: false,
    isVisible: true,
    devToolsOpen: false,
    externalDisplayCount: 0,
    tabSwitchCount: 0,
    copyAttemptCount: 0,
    printScreenCount: 0,
  };

  private _cleanupFns: Array<() => void> = [];
  private _devToolsCheckTimer: ReturnType<typeof setInterval> | null = null;

  // ---------------------------------------------------------------------------
  // Public
  // ---------------------------------------------------------------------------

  get state(): Readonly<BrowserIntegrityState> { return this._state; }

  /** Start monitoring browser integrity. */
  start(): void {
    this._addListener(document, 'visibilitychange', () => this._handleVisibility());
    this._addListener(window, 'blur', () => this._handleBlur());
    this._addListener(window, 'focus', () => this._handleFocus());
    this._addListener(document, 'copy', (e: Event) => this._handleCopy(e));
    this._addListener(document, 'cut', (e: Event) => this._handleCopy(e));
    this._addListener(document, 'paste', (e: Event) => this._handleCopy(e));
    this._addListener(document, 'contextmenu', (e: Event) => this._handleContextMenu(e));
    this._addListener(document, 'keydown', (e: Event) => this._handleKeydown(e as KeyboardEvent));
    this._addListener(document, 'fullscreenchange', () => this._handleFullscreen());

    // DevTools detection (periodic check).
    this._devToolsCheckTimer = setInterval(() => this._checkDevTools(), 2000);

    // External display detection (if API available).
    this._checkExternalDisplays();

    // Set initial fullscreen state.
    this._state.isFullscreen = !!document.fullscreenElement;
  }

  /** Stop monitoring. */
  stop(): void {
    for (const cleanup of this._cleanupFns) cleanup();
    this._cleanupFns = [];
    if (this._devToolsCheckTimer) {
      clearInterval(this._devToolsCheckTimer);
      this._devToolsCheckTimer = null;
    }
  }

  /** Destroy the monitor. */
  destroy(): void {
    this.stop();
    this.removeAllListeners();
  }

  /** Request fullscreen mode. */
  async requestFullscreen(element?: HTMLElement): Promise<void> {
    const el = element ?? document.documentElement;
    try {
      await el.requestFullscreen();
    } catch {
      // Fullscreen not supported or denied.
    }
  }

  // ---------------------------------------------------------------------------
  // Event handlers
  // ---------------------------------------------------------------------------

  private _handleVisibility(): void {
    const wasVisible = this._state.isVisible;
    this._state.isVisible = !document.hidden;

    if (wasVisible && document.hidden) {
      this._state.tabSwitchCount++;
      this._emitViolation(EventType.TAB_SWITCH, Severity.WARNING, 'Tab switch detected', 0.95);
    }
    this._emitState();
  }

  private _handleBlur(): void {
    if (this._state.isVisible) {
      this._state.tabSwitchCount++;
      this._emitViolation(EventType.TAB_SWITCH, Severity.WARNING, 'Window lost focus', 0.85);
    }
  }

  private _handleFocus(): void {
    // Focus restored — no violation.
  }

  private _handleCopy(e: Event): void {
    e.preventDefault();
    this._state.copyAttemptCount++;
    this._emitViolation(EventType.COPY_PASTE_ATTEMPT, Severity.WARNING, 'Copy/paste attempt blocked', 0.99);
    this._emitState();
  }

  private _handleContextMenu(e: Event): void {
    e.preventDefault();
    this._emitViolation(EventType.CONTEXT_MENU_ATTEMPT, Severity.INFO, 'Context menu blocked', 0.99);
  }

  private _handleKeydown(e: KeyboardEvent): void {
    // Block Print Screen.
    if (e.key === 'PrintScreen') {
      e.preventDefault();
      this._state.printScreenCount++;
      this._emitViolation(EventType.PRINT_SCREEN_ATTEMPT, Severity.CRITICAL, 'Print screen attempt', 0.99);
      this._emitState();
      return;
    }

    // Block common keyboard shortcuts.
    if ((e.ctrlKey || e.metaKey) && ['c', 'v', 'x', 'a', 'p', 's'].includes(e.key.toLowerCase())) {
      e.preventDefault();
      this._state.copyAttemptCount++;
      this._emitViolation(EventType.COPY_PASTE_ATTEMPT, Severity.WARNING, `Keyboard shortcut blocked: ${e.key}`, 0.95);
      this._emitState();
    }

    // Block Alt+Tab (can't actually prevent, but detect).
    if (e.altKey && e.key === 'Tab') {
      this._emitViolation(EventType.TAB_SWITCH, Severity.WARNING, 'Alt+Tab detected', 0.9);
    }
  }

  private _handleFullscreen(): void {
    this._state.isFullscreen = !!document.fullscreenElement;

    if (!this._state.isFullscreen) {
      this._emitViolation(EventType.FULLSCREEN_EXIT, Severity.WARNING, 'Exited fullscreen', 0.99);
    }
    this._emitState();
  }

  private _checkDevTools(): void {
    // Heuristic: window.outerWidth - window.innerWidth > 160 suggests DevTools.
    const widthDiff = window.outerWidth - window.innerWidth;
    const heightDiff = window.outerHeight - window.innerHeight;
    const wasOpen = this._state.devToolsOpen;
    this._state.devToolsOpen = widthDiff > 160 || heightDiff > 200;

    if (!wasOpen && this._state.devToolsOpen) {
      this._emitViolation(EventType.TAB_SWITCH, Severity.CRITICAL, 'DevTools opened', 0.8);
      this._emitState();
    }
  }

  private async _checkExternalDisplays(): Promise<void> {
    try {
      // Screen Details API (Chrome 100+).
      if ('getScreenDetails' in window) {
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        const details = await (window as any).getScreenDetails();
        const count = details?.screens?.length ?? 1;
        if (count > 1) {
          this._state.externalDisplayCount = count - 1;
          this._emitViolation(
            EventType.EXTERNAL_DISPLAY_DETECTED,
            Severity.WARNING,
            `${count - 1} external display(s) detected`,
            0.95,
          );
          this._emitState();
        }
      }
    } catch {
      // API not available or permission denied.
    }
  }

  // ---------------------------------------------------------------------------
  // Helpers
  // ---------------------------------------------------------------------------

  private _emitViolation(
    eventType: EventType,
    severity: Severity,
    label: string,
    confidence: number,
  ): void {
    this.emit('violation', {
      eventType,
      severity,
      source: EventSource.BROWSER,
      label,
      confidence,
    });
  }

  private _emitState(): void {
    this.emit('stateChange', { ...this._state });
  }

  private _addListener(target: EventTarget, event: string, handler: (e: Event) => void): void {
    target.addEventListener(event, handler);
    this._cleanupFns.push(() => target.removeEventListener(event, handler));
  }
}
