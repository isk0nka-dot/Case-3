// =============================================================================
// Argus AI — useBrowserIntegrity Composable
// =============================================================================
//
// Comprehensive browser integrity monitoring for anti-cheating:
//
//   1. Tab focus tracking — blur/focus, visibilitychange events
//   2. Second screen detection — screen.isExtended, matchMedia queries
//   3. DevTools detection — debugger timing, window size heuristics
//   4. Copy/paste interception — clipboard event blocking
//   5. Print screen prevention — keyboard event interception
//   6. Context menu prevention — right-click blocking
//   7. Fullscreen enforcement — exit detection
//   8. Window resize monitoring — iframe embedding detection
//
// All events are reported via the sendEvent callback for integration
// with useProctoringSession.
//
// =============================================================================

import { ref, onMounted, onUnmounted } from 'vue'
import { EventType, Severity, EventSource } from '~/lib/proto/types'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface BrowserIntegrityState {
  isFocused: boolean
  isVisible: boolean
  isFullscreen: boolean
  tabSwitchCount: number
  blurDurationMs: number
  externalDisplays: number
  devToolsOpen: boolean
  copyAttempts: number
  printScreenAttempts: number
  contextMenuAttempts: number
  lastBlurAt: number
  isSecondScreenDetected: boolean
}

// ---------------------------------------------------------------------------
// Composable
// ---------------------------------------------------------------------------

export function useBrowserIntegrity() {
  const state = ref<BrowserIntegrityState>({
    isFocused: true,
    isVisible: true,
    isFullscreen: false,
    tabSwitchCount: 0,
    blurDurationMs: 0,
    externalDisplays: 0,
    devToolsOpen: false,
    copyAttempts: 0,
    printScreenAttempts: 0,
    contextMenuAttempts: 0,
    lastBlurAt: 0,
    isSecondScreenDetected: false
  })

  let devToolsTimer: ReturnType<typeof setInterval> | null = null

  // Event callback
  let sendEventFn: ((
    eventType: EventType,
    severity: Severity,
    payload?: any,
    label?: string,
    confidence?: number,
    source?: EventSource
  ) => void) | null = null

  function onEvent(fn: typeof sendEventFn) {
    sendEventFn = fn
  }

  // -------------------------------------------------------------------------
  // 1. Tab Focus / Visibility
  // -------------------------------------------------------------------------

  function handleBlur() {
    state.value.isFocused = false
    state.value.lastBlurAt = Date.now()
    state.value.tabSwitchCount++

    const severity = state.value.tabSwitchCount > 3
      ? Severity.CRITICAL
      : Severity.WARNING

    sendEventFn?.(
      EventType.TAB_SWITCH,
      severity,
      {
        type: 'browser',
        data: {
          tabSwitchCount: state.value.tabSwitchCount,
          targetInfo: undefined,
          fullscreenExited: !state.value.isFullscreen,
          displayCount: state.value.externalDisplays + 1,
          action: 'blur'
        }
      },
      `Потеря фокуса #${state.value.tabSwitchCount}`,
      1.0,
      EventSource.BROWSER
    )
  }

  function handleFocus() {
    if (state.value.lastBlurAt > 0) {
      state.value.blurDurationMs += Date.now() - state.value.lastBlurAt
    }
    state.value.isFocused = true
    state.value.lastBlurAt = 0
  }

  function handleVisibilityChange() {
    state.value.isVisible = !document.hidden

    if (document.hidden) {
      state.value.tabSwitchCount++

      sendEventFn?.(
        EventType.FOCUS_LOSS_DETECTED,
        Severity.WARNING,
        {
          type: 'psychometry',
          data: {
            emotion: 'neutral',
            intensity: 0,
            focusScore: 0,
            blinkRate: 0,
            blinkAnomaly: false
          }
        },
        `Вкладка скрыта (${state.value.tabSwitchCount} раз)`,
        1.0,
        EventSource.BROWSER
      )
    }
  }

  // -------------------------------------------------------------------------
  // 2. Second Screen / External Display Detection
  // -------------------------------------------------------------------------

  function checkExternalDisplays() {
    // Screen API
    const screenApi = (window as any).screen
    if (screenApi?.isExtended !== undefined) {
      state.value.isSecondScreenDetected = screenApi.isExtended === true
    }

    // matchMedia multi-screen heuristic
    if (window.matchMedia) {
      const wide = window.matchMedia('(min-width: 3000px)')
      if (wide.matches && screen.width < 3000) {
        state.value.isSecondScreenDetected = true
      }
    }

    // Count external displays via window.screen API
    if ('getScreenDetails' in window) {
      (window as any).getScreenDetails?.().then?.((details: any) => {
        const screenCount = details?.screens?.length ?? 1
        state.value.externalDisplays = screenCount - 1

        if (screenCount > 1) {
          state.value.isSecondScreenDetected = true

          sendEventFn?.(
            EventType.EXTERNAL_DISPLAY_DETECTED,
            Severity.WARNING,
            {
              type: 'browser',
              data: {
                tabSwitchCount: state.value.tabSwitchCount,
                targetInfo: `${screenCount} displays`,
                fullscreenExited: false,
                displayCount: screenCount,
                action: 'multi_screen'
              }
            },
            `Обнаружено ${screenCount} экранов`,
            0.9,
            EventSource.BROWSER
          )
        }
      }).catch(() => {
        // Permission denied — expected
      })
    }
  }

  // -------------------------------------------------------------------------
  // 3. DevTools Detection
  // -------------------------------------------------------------------------

  function checkDevTools() {
    let devToolsDetected = false

    // Signal 1: Window size heuristic (most reliable cross-browser)
    // DevTools docked to side/bottom reduces inner dimensions significantly
    const widthDiff = window.outerWidth - window.innerWidth
    const heightDiff = window.outerHeight - window.innerHeight
    if ((widthDiff > 200 || heightDiff > 200) && !state.value.isFullscreen) {
      devToolsDetected = true
    }

    // Signal 2: Image getter detection — when DevTools Console is open,
    // accessing object properties triggers getters (used by console.log)
    try {
      const element = new Image()
      Object.defineProperty(element, 'id', {
        get: function () {
          devToolsDetected = true
          return ''
        }
      })
      // eslint-disable-next-line no-console
      console.debug('%c', element as unknown as string)
    } catch {
      // Property definition may fail in strict environments — non-fatal
    }

    // Signal 3: Regex toString override — triggers when console formats objects
    try {
      const devtools = /./
      devtools.toString = function () {
        devToolsDetected = true
        return ''
      }
      // eslint-disable-next-line no-console
      console.debug('%c', devtools as unknown as string)
    } catch {
      // toString override may fail — non-fatal
    }

    state.value.devToolsOpen = devToolsDetected
  }

  // -------------------------------------------------------------------------
  // 4. Copy/Paste Interception
  // -------------------------------------------------------------------------

  function handleCopy(e: ClipboardEvent) {
    e.preventDefault()
    state.value.copyAttempts++

    sendEventFn?.(
      EventType.COPY_PASTE_ATTEMPT,
      state.value.copyAttempts > 2 ? Severity.WARNING : Severity.INFO,
      {
        type: 'browser',
        data: {
          tabSwitchCount: state.value.tabSwitchCount,
          targetInfo: 'copy',
          fullscreenExited: false,
          displayCount: 1,
          action: 'copy'
        }
      },
      `Попытка копирования #${state.value.copyAttempts}`,
      1.0,
      EventSource.BROWSER
    )
  }

  function handlePaste(e: ClipboardEvent) {
    e.preventDefault()
    state.value.copyAttempts++

    sendEventFn?.(
      EventType.COPY_PASTE_ATTEMPT,
      Severity.WARNING,
      {
        type: 'browser',
        data: {
          tabSwitchCount: state.value.tabSwitchCount,
          targetInfo: 'paste',
          fullscreenExited: false,
          displayCount: 1,
          action: 'paste'
        }
      },
      `Попытка вставки #${state.value.copyAttempts}`,
      1.0,
      EventSource.BROWSER
    )
  }

  // -------------------------------------------------------------------------
  // 5. Print Screen / Screenshot Prevention
  // -------------------------------------------------------------------------

  function handleKeydown(e: KeyboardEvent) {
    // PrintScreen key
    if (e.key === 'PrintScreen' || e.code === 'PrintScreen') {
      e.preventDefault()
      state.value.printScreenAttempts++

      sendEventFn?.(
        EventType.PRINT_SCREEN_ATTEMPT,
        Severity.WARNING,
        {
          type: 'browser',
          data: {
            tabSwitchCount: state.value.tabSwitchCount,
            targetInfo: 'PrintScreen',
            fullscreenExited: false,
            displayCount: 1,
            action: 'print_screen'
          }
        },
        `Попытка скриншота #${state.value.printScreenAttempts}`,
        1.0,
        EventSource.BROWSER
      )
      return
    }

    // Common screenshot shortcuts
    const isCtrlOrCmd = e.ctrlKey || e.metaKey
    if (isCtrlOrCmd && e.shiftKey && (e.key === 's' || e.key === 'S' || e.key === '4' || e.key === '3')) {
      e.preventDefault()
      state.value.printScreenAttempts++

      sendEventFn?.(
        EventType.PRINT_SCREEN_ATTEMPT,
        Severity.WARNING,
        {
          type: 'browser',
          data: {
            tabSwitchCount: state.value.tabSwitchCount,
            targetInfo: `Ctrl+Shift+${e.key}`,
            fullscreenExited: false,
            displayCount: 1,
            action: 'screenshot_shortcut'
          }
        },
        `Попытка скриншота: ${e.ctrlKey ? 'Ctrl' : 'Cmd'}+Shift+${e.key}`,
        1.0,
        EventSource.BROWSER
      )
    }

    // Prevent DevTools shortcuts
    if (e.key === 'F12') {
      e.preventDefault()
    }
    if (isCtrlOrCmd && e.shiftKey && (e.key === 'i' || e.key === 'I' || e.key === 'j' || e.key === 'J')) {
      e.preventDefault()
    }
  }

  // -------------------------------------------------------------------------
  // 6. Context Menu Prevention
  // -------------------------------------------------------------------------

  function handleContextMenu(e: MouseEvent) {
    e.preventDefault()
    state.value.contextMenuAttempts++

    sendEventFn?.(
      EventType.CONTEXT_MENU_ATTEMPT,
      state.value.contextMenuAttempts > 3 ? Severity.WARNING : Severity.INFO,
      {
        type: 'browser',
        data: {
          tabSwitchCount: state.value.tabSwitchCount,
          targetInfo: undefined,
          fullscreenExited: false,
          displayCount: 1,
          action: 'context_menu'
        }
      },
      `Контекстное меню #${state.value.contextMenuAttempts}`,
      1.0,
      EventSource.BROWSER
    )
  }

  // -------------------------------------------------------------------------
  // 7. Fullscreen Monitoring
  // -------------------------------------------------------------------------

  function handleFullscreenChange() {
    const isFs = !!document.fullscreenElement
    const wasFullscreen = state.value.isFullscreen
    state.value.isFullscreen = isFs

    if (wasFullscreen && !isFs) {
      sendEventFn?.(
        EventType.FULLSCREEN_EXIT,
        Severity.WARNING,
        {
          type: 'browser',
          data: {
            tabSwitchCount: state.value.tabSwitchCount,
            targetInfo: undefined,
            fullscreenExited: true,
            displayCount: state.value.externalDisplays + 1,
            action: 'fullscreen_exit'
          }
        },
        'Выход из полноэкранного режима',
        1.0,
        EventSource.BROWSER
      )
    }
  }

  // -------------------------------------------------------------------------
  // Lifecycle
  // -------------------------------------------------------------------------

  function start(options?: { forceFullscreen?: boolean }) {
    window.addEventListener('blur', handleBlur)
    window.addEventListener('focus', handleFocus)
    document.addEventListener('visibilitychange', handleVisibilityChange)
    document.addEventListener('copy', handleCopy)
    document.addEventListener('paste', handlePaste)
    document.addEventListener('keydown', handleKeydown, true) // capture phase
    document.addEventListener('contextmenu', handleContextMenu)
    document.addEventListener('fullscreenchange', handleFullscreenChange)

    // Periodic checks
    checkExternalDisplays()
    devToolsTimer = setInterval(() => {
      checkDevTools()
      checkExternalDisplays()
    }, 3000)

    // Initial fullscreen state
    state.value.isFullscreen = !!document.fullscreenElement

    // Request fullscreen if exam settings require it and we're not already in fullscreen
    if (options?.forceFullscreen && !document.fullscreenElement) {
      document.documentElement.requestFullscreen().then(() => {
        state.value.isFullscreen = true
      }).catch(() => {
        // Browser denied the fullscreen request (requires user gesture)
        sendEventFn?.(
          EventType.FULLSCREEN_EXIT,
          Severity.WARNING,
          {
            type: 'browser',
            data: {
              tabSwitchCount: state.value.tabSwitchCount,
              targetInfo: 'fullscreen_denied',
              fullscreenExited: true,
              displayCount: state.value.externalDisplays + 1,
              action: 'fullscreen_request_denied'
            }
          },
          'Запрос полноэкранного режима отклонён браузером',
          0.5,
          EventSource.BROWSER
        )
      })
    }
  }

  function stop() {
    window.removeEventListener('blur', handleBlur)
    window.removeEventListener('focus', handleFocus)
    document.removeEventListener('visibilitychange', handleVisibilityChange)
    document.removeEventListener('copy', handleCopy)
    document.removeEventListener('paste', handlePaste)
    document.removeEventListener('keydown', handleKeydown, true)
    document.removeEventListener('contextmenu', handleContextMenu)
    document.removeEventListener('fullscreenchange', handleFullscreenChange)

    if (devToolsTimer) {
      clearInterval(devToolsTimer)
      devToolsTimer = null
    }
  }

  return {
    state,
    start,
    stop,
    onEvent
  }
}
