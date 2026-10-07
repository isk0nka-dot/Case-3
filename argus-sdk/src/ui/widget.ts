// =============================================================================
// Argus SDK — Draggable Camera Widget
// =============================================================================
//
// Floating camera preview with drag support, status indicator, and controls.
// Uses Shadow DOM for CSS isolation from host page styles.
// =============================================================================

import type { SessionStatus } from '../types';

/** Widget options. */
export interface WidgetOptions {
  /** Mount point element ID. If null, creates a floating widget. */
  containerId?: string;
  /** Initial position. Default: bottom-right. */
  position?: 'top-left' | 'top-right' | 'bottom-left' | 'bottom-right';
  /** Initial size. Default: 240x180. */
  width?: number;
  height?: number;
  /** Whether the widget is draggable. Default: true. */
  draggable?: boolean;
  /** Locale for UI strings. Default: 'ru'. */
  locale?: 'kk' | 'ru' | 'en';
}

const LABELS: Record<string, Record<string, string>> = {
  kk: {
    recording: 'Жазылуда',
    paused: 'Тоқтатылды',
    error: 'Қате',
    initializing: 'Дайындалуда...',
    minimize: 'Кішірейту',
    maximize: 'Үлкейту',
  },
  ru: {
    recording: 'Запись',
    paused: 'Пауза',
    error: 'Ошибка',
    initializing: 'Инициализация...',
    minimize: 'Свернуть',
    maximize: 'Развернуть',
  },
  en: {
    recording: 'Recording',
    paused: 'Paused',
    error: 'Error',
    initializing: 'Initializing...',
    minimize: 'Minimize',
    maximize: 'Maximize',
  },
};

/**
 * Draggable camera widget with Shadow DOM CSS isolation.
 *
 * Renders a floating camera preview with:
 * - Status indicator (green=recording, yellow=reconnecting, red=error)
 * - Minimize/maximize toggle
 * - Drag to reposition
 */
export class CameraWidget {
  private _host: HTMLElement;
  private _shadow: ShadowRoot;
  private _container: HTMLElement;
  private _video: HTMLVideoElement;
  private _statusDot: HTMLElement;
  private _statusLabel: HTMLElement;
  private _minimizeBtn: HTMLElement;
  private _isMinimized = false;
  private _isDragging = false;
  private _dragOffsetX = 0;
  private _dragOffsetY = 0;
  private _labels: Record<string, string>;
  private readonly _draggable: boolean;
  private readonly _width: number;
  private readonly _height: number;

  // Bound handlers for cleanup.
  private _onMouseMove: ((e: MouseEvent) => void) | null = null;
  private _onMouseUp: (() => void) | null = null;
  private _onTouchMove: ((e: TouchEvent) => void) | null = null;
  private _onTouchEnd: (() => void) | null = null;

  constructor(options?: WidgetOptions) {
    this._labels = LABELS[options?.locale ?? 'ru'];
    this._draggable = options?.draggable ?? true;
    this._width = options?.width ?? 240;
    this._height = options?.height ?? 180;

    // Create or find host element.
    if (options?.containerId) {
      const existing = document.getElementById(options.containerId);
      if (existing) {
        this._host = existing;
      } else {
        this._host = this._createFloatingHost(options?.position);
      }
    } else {
      this._host = this._createFloatingHost(options?.position);
    }

    // Attach Shadow DOM for style isolation.
    this._shadow = this._host.attachShadow({ mode: 'open' });

    // Build DOM.
    const { container, video, statusDot, statusLabel, minimizeBtn } = this._buildDOM();
    this._container = container;
    this._video = video;
    this._statusDot = statusDot;
    this._statusLabel = statusLabel;
    this._minimizeBtn = minimizeBtn;

    this._shadow.appendChild(this._buildStyles());
    this._shadow.appendChild(this._container);

    // Enable drag.
    if (this._draggable && !options?.containerId) {
      this._enableDrag();
    }
  }

  // ---------------------------------------------------------------------------
  // Public
  // ---------------------------------------------------------------------------

  /** Get the video element (for connecting to MediaStream). */
  get videoElement(): HTMLVideoElement { return this._video; }

  /** Attach a media stream to the video element. */
  setStream(stream: MediaStream): void {
    this._video.srcObject = stream;
    this._video.play().catch(() => {});
  }

  /** Update the status indicator. */
  setStatus(status: SessionStatus): void {
    const colorMap: Record<string, string> = {
      idle: '#888',
      initializing: '#f59e0b',
      preflight: '#f59e0b',
      ready: '#22c55e',
      active: '#22c55e',
      paused: '#f59e0b',
      completed: '#888',
      error: '#ef4444',
    };

    const labelMap: Record<string, string> = {
      idle: '',
      initializing: this._labels.initializing,
      preflight: this._labels.initializing,
      ready: '',
      active: this._labels.recording,
      paused: this._labels.paused,
      completed: '',
      error: this._labels.error,
    };

    this._statusDot.style.backgroundColor = colorMap[status] ?? '#888';
    this._statusLabel.textContent = labelMap[status] ?? '';

    // Pulse animation for active recording.
    if (status === 'active') {
      this._statusDot.classList.add('pulse');
    } else {
      this._statusDot.classList.remove('pulse');
    }
  }

  /** Show the widget. */
  show(): void {
    this._host.style.display = '';
  }

  /** Hide the widget. */
  hide(): void {
    this._host.style.display = 'none';
  }

  /** Destroy the widget and clean up. */
  destroy(): void {
    if (this._onMouseMove) document.removeEventListener('mousemove', this._onMouseMove);
    if (this._onMouseUp) document.removeEventListener('mouseup', this._onMouseUp);
    if (this._onTouchMove) document.removeEventListener('touchmove', this._onTouchMove);
    if (this._onTouchEnd) document.removeEventListener('touchend', this._onTouchEnd);

    this._video.srcObject = null;
    this._host.remove();
  }

  // ---------------------------------------------------------------------------
  // Private — DOM construction
  // ---------------------------------------------------------------------------

  private _createFloatingHost(position?: string): HTMLElement {
    const host = document.createElement('div');
    host.id = 'argus-sdk-widget';
    host.style.cssText = `
      position: fixed;
      z-index: 2147483647;
      ${this._positionCSS(position)}
    `;
    document.body.appendChild(host);
    return host;
  }

  private _positionCSS(position?: string): string {
    switch (position) {
      case 'top-left': return 'top: 16px; left: 16px;';
      case 'top-right': return 'top: 16px; right: 16px;';
      case 'bottom-left': return 'bottom: 16px; left: 16px;';
      default: return 'bottom: 16px; right: 16px;';
    }
  }

  private _buildDOM(): {
    container: HTMLElement;
    video: HTMLVideoElement;
    statusDot: HTMLElement;
    statusLabel: HTMLElement;
    minimizeBtn: HTMLElement;
  } {
    const container = document.createElement('div');
    container.className = 'argus-widget';

    // Title bar (draggable).
    const titleBar = document.createElement('div');
    titleBar.className = 'title-bar';

    const statusDot = document.createElement('span');
    statusDot.className = 'status-dot';

    const statusLabel = document.createElement('span');
    statusLabel.className = 'status-label';

    const minimizeBtn = document.createElement('button');
    minimizeBtn.className = 'minimize-btn';
    minimizeBtn.textContent = '−';
    minimizeBtn.title = this._labels.minimize;
    minimizeBtn.addEventListener('click', () => this._toggleMinimize());

    titleBar.appendChild(statusDot);
    titleBar.appendChild(statusLabel);
    titleBar.appendChild(minimizeBtn);

    // Video container.
    const videoContainer = document.createElement('div');
    videoContainer.className = 'video-container';

    const video = document.createElement('video');
    video.autoplay = true;
    video.playsInline = true;
    video.muted = true;
    video.style.cssText = `width: 100%; height: 100%; object-fit: cover; border-radius: 0 0 8px 8px;`;

    videoContainer.appendChild(video);

    container.appendChild(titleBar);
    container.appendChild(videoContainer);

    return { container, video, statusDot, statusLabel, minimizeBtn };
  }

  private _buildStyles(): HTMLStyleElement {
    const style = document.createElement('style');
    style.textContent = `
      .argus-widget {
        width: ${this._width}px;
        background: #1a1a2e;
        border-radius: 8px;
        overflow: hidden;
        box-shadow: 0 4px 24px rgba(0, 0, 0, 0.4);
        border: 1px solid rgba(255, 255, 255, 0.1);
        font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
        transition: width 0.2s ease, height 0.2s ease;
      }
      .title-bar {
        display: flex;
        align-items: center;
        padding: 6px 10px;
        background: rgba(0, 0, 0, 0.3);
        cursor: ${this._draggable ? 'grab' : 'default'};
        user-select: none;
        gap: 6px;
      }
      .title-bar:active {
        cursor: ${this._draggable ? 'grabbing' : 'default'};
      }
      .status-dot {
        width: 8px;
        height: 8px;
        border-radius: 50%;
        background: #888;
        flex-shrink: 0;
      }
      .status-dot.pulse {
        animation: pulse 1.5s infinite;
      }
      @keyframes pulse {
        0%, 100% { opacity: 1; }
        50% { opacity: 0.4; }
      }
      .status-label {
        color: rgba(255, 255, 255, 0.8);
        font-size: 11px;
        font-weight: 500;
        flex: 1;
      }
      .minimize-btn {
        background: none;
        border: none;
        color: rgba(255, 255, 255, 0.6);
        font-size: 16px;
        cursor: pointer;
        padding: 0 4px;
        line-height: 1;
      }
      .minimize-btn:hover {
        color: white;
      }
      .video-container {
        width: ${this._width}px;
        height: ${this._height}px;
        overflow: hidden;
        transition: height 0.2s ease;
      }
      .video-container.minimized {
        height: 0;
      }
    `;
    return style;
  }

  // ---------------------------------------------------------------------------
  // Private — Drag handling
  // ---------------------------------------------------------------------------

  private _enableDrag(): void {
    const titleBar = this._container.querySelector('.title-bar') as HTMLElement;
    if (!titleBar) return;

    titleBar.addEventListener('mousedown', (e: MouseEvent) => {
      this._isDragging = true;
      const rect = this._host.getBoundingClientRect();
      this._dragOffsetX = e.clientX - rect.left;
      this._dragOffsetY = e.clientY - rect.top;
      e.preventDefault();
    });

    titleBar.addEventListener('touchstart', (e: TouchEvent) => {
      if (e.touches.length !== 1) return;
      this._isDragging = true;
      const rect = this._host.getBoundingClientRect();
      this._dragOffsetX = e.touches[0].clientX - rect.left;
      this._dragOffsetY = e.touches[0].clientY - rect.top;
    }, { passive: true });

    this._onMouseMove = (e: MouseEvent) => {
      if (!this._isDragging) return;
      const x = e.clientX - this._dragOffsetX;
      const y = e.clientY - this._dragOffsetY;
      this._host.style.left = `${x}px`;
      this._host.style.top = `${y}px`;
      this._host.style.right = 'auto';
      this._host.style.bottom = 'auto';
    };

    this._onMouseUp = () => { this._isDragging = false; };

    this._onTouchMove = (e: TouchEvent) => {
      if (!this._isDragging || e.touches.length !== 1) return;
      const x = e.touches[0].clientX - this._dragOffsetX;
      const y = e.touches[0].clientY - this._dragOffsetY;
      this._host.style.left = `${x}px`;
      this._host.style.top = `${y}px`;
      this._host.style.right = 'auto';
      this._host.style.bottom = 'auto';
    };

    this._onTouchEnd = () => { this._isDragging = false; };

    document.addEventListener('mousemove', this._onMouseMove);
    document.addEventListener('mouseup', this._onMouseUp);
    document.addEventListener('touchmove', this._onTouchMove, { passive: true });
    document.addEventListener('touchend', this._onTouchEnd);
  }

  private _toggleMinimize(): void {
    this._isMinimized = !this._isMinimized;
    const videoContainer = this._container.querySelector('.video-container') as HTMLElement;
    if (videoContainer) {
      videoContainer.classList.toggle('minimized', this._isMinimized);
    }
    this._minimizeBtn.textContent = this._isMinimized ? '+' : '−';
    this._minimizeBtn.title = this._isMinimized ? this._labels.maximize : this._labels.minimize;
  }
}
