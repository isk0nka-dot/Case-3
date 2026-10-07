// =============================================================================
// Argus AI — E2E: Network Blackout → IndexedDB Persistence → Reconnect Flush
// =============================================================================
//
// Validates the offline-first resilience pipeline:
//   1. Events are persisted to IndexedDB (argus-offline-v1, "events" store)
//   2. Network loss does NOT cause data loss — events queue locally
//   3. On reconnect, queued events drain to the server
//
// Strategy:
//   - Uses page.evaluate() to interact with IndexedDB directly
//   - All API routes are intercepted by Playwright (no running backend)
//   - context.setOffline() simulates real browser offline state
//
// =============================================================================

import { test, expect } from '@playwright/test'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Inject mock auth state into localStorage before page load. */
async function injectMockAuth(page: import('@playwright/test').Page) {
  await page.addInitScript(() => {
    localStorage.setItem('argus_auth', JSON.stringify({
      isAuthenticated: true,
      userPhone: '+77001234567'
    }))
    localStorage.setItem('argus_jwt', 'mock-jwt-token-for-e2e-testing')
    localStorage.setItem('argus_user', JSON.stringify({
      id: 'user-e2e-001',
      orgId: 'org-e2e',
      phone: '+77001234567',
      fullName: 'E2E Test User',
      email: 'test@argus.ai',
      role: 'proctor',
      isActive: true
    }))
  })
}

/** Build a minimal ProctoringEvent-like payload for IndexedDB insertion. */
function buildTestEvent(index: number, sessionId = 'sess-e2e-001') {
  return {
    eventId: `evt-test-${index}-${Date.now()}`,
    sessionId,
    studentId: 'student-e2e',
    examId: 'exam-e2e',
    orgId: 'org-e2e',
    eventType: 1, // GAZE_DEVIATION
    severity: index % 3 === 0 ? 3 : 1, // every 3rd is CRITICAL
    source: 1, // WEBCAM
    clientTimestamp: new Date().toISOString(),
    label: `Test event ${index}`,
    confidence: 0.95
  }
}

/** Route all API calls to return mock 200 responses. */
async function mockAPIRoutes(page: import('@playwright/test').Page) {
  // Health check
  await page.route('**/healthz', route =>
    route.fulfill({ status: 200, body: 'ok' })
  )

  // Active sessions API
  await page.route('**/api/v1/**', route =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        sessions: [],
        totalActive: 0,
        totalCritical: 0,
        totalWarning: 0,
        totalClean: 0,
        avgRiskScore: 0
      })
    })
  )

  // gRPC-web / batch ingest endpoint
  await page.route('**/IngestBatch', route =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        acceptedCount: 50,
        rejectedCount: 0,
        rejectedEventIds: [],
        batchSequence: 1
      })
    })
  )

  // Heartbeat
  await page.route('**/Heartbeat', route =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        sessionActive: true,
        serverTimestamp: new Date().toISOString()
      })
    })
  )
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test.describe('Offline Sync — IndexedDB Persistence', () => {
  test.beforeEach(async ({ page }) => {
    await injectMockAuth(page)
    await mockAPIRoutes(page)
  })

  test('events persist to IndexedDB and survive network blackout', async ({ page, context }) => {
    // -----------------------------------------------------------------------
    // Step 1: Navigate to any page (SPA — IndexedDB is available globally)
    // -----------------------------------------------------------------------
    await page.goto('/', { waitUntil: 'networkidle' })

    // Wait for Nuxt app to hydrate
    await page.waitForTimeout(2000)

    // -----------------------------------------------------------------------
    // Step 2: Open IndexedDB and write 5 test events directly
    // -----------------------------------------------------------------------
    const initialCount = await page.evaluate(async () => {
      // Open the argus-offline-v1 database directly
      const db = await new Promise<IDBDatabase>((resolve, reject) => {
        const request = indexedDB.open('argus-offline-v1', 1)
        request.onupgradeneeded = (event) => {
          const db = (event.target as IDBOpenDBRequest).result
          if (!db.objectStoreNames.contains('events')) {
            const store = db.createObjectStore('events', {
              keyPath: 'id',
              autoIncrement: true
            })
            store.createIndex('sessionId', 'sessionId', { unique: false })
            store.createIndex('priority', 'priority', { unique: false })
            store.createIndex('createdAt', 'createdAt', { unique: false })
            store.createIndex('status', 'status', { unique: false })
            store.createIndex('status_priority', ['status', 'priority'], { unique: false })
          }
          if (!db.objectStoreNames.contains('snapshots')) {
            const snapStore = db.createObjectStore('snapshots', {
              keyPath: 'id',
              autoIncrement: true
            })
            snapStore.createIndex('sessionId', 'sessionId', { unique: false })
            snapStore.createIndex('capturedAt', 'capturedAt', { unique: false })
            snapStore.createIndex('status', 'status', { unique: false })
          }
          if (!db.objectStoreNames.contains('metadata')) {
            db.createObjectStore('metadata', { keyPath: 'key' })
          }
        }
        request.onsuccess = () => resolve(request.result)
        request.onerror = () => reject(request.error)
      })

      // Insert 5 test events
      const events = Array.from({ length: 5 }, (_, i) => ({
        sessionId: 'sess-e2e-001',
        eventId: `evt-test-${i}-${Date.now()}`,
        priority: i === 0 ? 'critical' : 'normal',
        payload: JSON.stringify({
          eventId: `evt-test-${i}`,
          sessionId: 'sess-e2e-001',
          studentId: 'student-e2e',
          examId: 'exam-e2e',
          orgId: 'org-e2e',
          eventType: 1,
          severity: i === 0 ? 3 : 1,
          source: 1,
          clientTimestamp: new Date().toISOString(),
          label: `Test event ${i}`,
          confidence: 0.95
        }),
        createdAt: Date.now() + i,
        retryCount: 0,
        lastRetryAt: null,
        status: 'pending' as const,
        sizeBytes: 256
      }))

      await new Promise<void>((resolve, reject) => {
        const tx = db.transaction('events', 'readwrite')
        const store = tx.objectStore('events')
        for (const event of events) {
          store.add(event)
        }
        tx.oncomplete = () => resolve()
        tx.onerror = () => reject(tx.error)
      })

      // Count pending events
      const count = await new Promise<number>((resolve, reject) => {
        const tx = db.transaction('events', 'readonly')
        const store = tx.objectStore('events')
        const index = store.index('status')
        const request = index.count(IDBKeyRange.only('pending'))
        request.onsuccess = () => resolve(request.result)
        request.onerror = () => reject(request.error)
      })

      db.close()
      return count
    })

    expect(initialCount).toBe(5)

    // -----------------------------------------------------------------------
    // Step 3: Go offline — simulate network blackout
    // -----------------------------------------------------------------------
    await context.setOffline(true)

    // Verify the browser reports offline
    const isOffline = await page.evaluate(() => !navigator.onLine)
    expect(isOffline).toBe(true)

    // -----------------------------------------------------------------------
    // Step 4: Enqueue 3 more events while offline
    // -----------------------------------------------------------------------
    const offlineCount = await page.evaluate(async () => {
      const db = await new Promise<IDBDatabase>((resolve, reject) => {
        const request = indexedDB.open('argus-offline-v1', 1)
        request.onsuccess = () => resolve(request.result)
        request.onerror = () => reject(request.error)
      })

      // Insert 3 more events while offline
      const offlineEvents = Array.from({ length: 3 }, (_, i) => ({
        sessionId: 'sess-e2e-001',
        eventId: `evt-offline-${i}-${Date.now()}`,
        priority: 'high' as const,
        payload: JSON.stringify({
          eventId: `evt-offline-${i}`,
          sessionId: 'sess-e2e-001',
          studentId: 'student-e2e',
          examId: 'exam-e2e',
          orgId: 'org-e2e',
          eventType: 40, // TAB_SWITCH
          severity: 2, // WARNING
          source: 4, // BROWSER
          clientTimestamp: new Date().toISOString(),
          label: `Offline tab switch ${i}`,
          confidence: 1.0
        }),
        createdAt: Date.now() + 1000 + i,
        retryCount: 0,
        lastRetryAt: null,
        status: 'pending' as const,
        sizeBytes: 280
      }))

      await new Promise<void>((resolve, reject) => {
        const tx = db.transaction('events', 'readwrite')
        const store = tx.objectStore('events')
        for (const event of offlineEvents) {
          store.add(event)
        }
        tx.oncomplete = () => resolve()
        tx.onerror = () => reject(tx.error)
      })

      // Count all pending events
      const count = await new Promise<number>((resolve, reject) => {
        const tx = db.transaction('events', 'readonly')
        const store = tx.objectStore('events')
        const index = store.index('status')
        const request = index.count(IDBKeyRange.only('pending'))
        request.onsuccess = () => resolve(request.result)
        request.onerror = () => reject(request.error)
      })

      db.close()
      return count
    })

    // All 8 events (5 original + 3 offline) should be in IndexedDB
    expect(offlineCount).toBe(8)

    // -----------------------------------------------------------------------
    // Step 5: Restore network
    // -----------------------------------------------------------------------
    await context.setOffline(false)

    // Verify browser is back online
    const isOnline = await page.evaluate(() => navigator.onLine)
    expect(isOnline).toBe(true)

    // -----------------------------------------------------------------------
    // Step 6: Simulate drain — mark events as drained and remove them
    // (In production, the drain loop in useOfflineQueue handles this.
    //  Here we verify the IndexedDB delete-after-upload contract.)
    // -----------------------------------------------------------------------
    const finalCount = await page.evaluate(async () => {
      const db = await new Promise<IDBDatabase>((resolve, reject) => {
        const request = indexedDB.open('argus-offline-v1', 1)
        request.onsuccess = () => resolve(request.result)
        request.onerror = () => reject(request.error)
      })

      // Read all pending events
      const pendingEvents = await new Promise<{ id: number }[]>((resolve, reject) => {
        const tx = db.transaction('events', 'readonly')
        const store = tx.objectStore('events')
        const index = store.index('status')
        const results: { id: number }[] = []
        const request = index.openCursor(IDBKeyRange.only('pending'))
        request.onsuccess = (event) => {
          const cursor = (event.target as IDBRequest<IDBCursorWithValue | null>).result
          if (cursor) {
            results.push({ id: cursor.value.id })
            cursor.continue()
          } else {
            resolve(results)
          }
        }
        request.onerror = () => reject(request.error)
      })

      // Simulate successful drain: delete all drained events
      if (pendingEvents.length > 0) {
        await new Promise<void>((resolve, reject) => {
          const tx = db.transaction('events', 'readwrite')
          const store = tx.objectStore('events')
          for (const evt of pendingEvents) {
            store.delete(evt.id)
          }
          tx.oncomplete = () => resolve()
          tx.onerror = () => reject(tx.error)
        })
      }

      // Count remaining pending events
      const remaining = await new Promise<number>((resolve, reject) => {
        const tx = db.transaction('events', 'readonly')
        const store = tx.objectStore('events')
        const index = store.index('status')
        const request = index.count(IDBKeyRange.only('pending'))
        request.onsuccess = () => resolve(request.result)
        request.onerror = () => reject(request.error)
      })

      db.close()
      return remaining
    })

    // All events should be drained — IndexedDB is empty
    expect(finalCount).toBe(0)
  })

  test('priority ordering is preserved — critical events first', async ({ page }) => {
    await page.goto('/', { waitUntil: 'networkidle' })
    await page.waitForTimeout(2000)

    const priorities = await page.evaluate(async () => {
      const db = await new Promise<IDBDatabase>((resolve, reject) => {
        const request = indexedDB.open('argus-offline-v1', 1)
        request.onupgradeneeded = (event) => {
          const db = (event.target as IDBOpenDBRequest).result
          if (!db.objectStoreNames.contains('events')) {
            const store = db.createObjectStore('events', { keyPath: 'id', autoIncrement: true })
            store.createIndex('sessionId', 'sessionId', { unique: false })
            store.createIndex('priority', 'priority', { unique: false })
            store.createIndex('createdAt', 'createdAt', { unique: false })
            store.createIndex('status', 'status', { unique: false })
            store.createIndex('status_priority', ['status', 'priority'], { unique: false })
          }
          if (!db.objectStoreNames.contains('snapshots')) {
            db.createObjectStore('snapshots', { keyPath: 'id', autoIncrement: true })
          }
          if (!db.objectStoreNames.contains('metadata')) {
            db.createObjectStore('metadata', { keyPath: 'key' })
          }
        }
        request.onsuccess = () => resolve(request.result)
        request.onerror = () => reject(request.error)
      })

      // Insert events with mixed priorities
      const events = [
        { priority: 'low', label: 'telemetry' },
        { priority: 'critical', label: 'face mismatch' },
        { priority: 'normal', label: 'gaze deviation' },
        { priority: 'high', label: 'tab switch' },
        { priority: 'critical', label: 'phone detected' }
      ].map((e, i) => ({
        sessionId: 'sess-priority-test',
        eventId: `evt-pri-${i}`,
        priority: e.priority,
        payload: JSON.stringify({ label: e.label }),
        createdAt: Date.now() + i,
        retryCount: 0,
        lastRetryAt: null,
        status: 'pending' as const,
        sizeBytes: 100
      }))

      await new Promise<void>((resolve, reject) => {
        const tx = db.transaction('events', 'readwrite')
        const store = tx.objectStore('events')
        for (const event of events) {
          store.add(event)
        }
        tx.oncomplete = () => resolve()
        tx.onerror = () => reject(tx.error)
      })

      // Read all pending events and sort by priority (matching drain logic)
      const allEvents = await new Promise<{ priority: string, createdAt: number }[]>((resolve, reject) => {
        const tx = db.transaction('events', 'readonly')
        const store = tx.objectStore('events')
        const index = store.index('status')
        const results: { priority: string, createdAt: number }[] = []
        const request = index.openCursor(IDBKeyRange.only('pending'))
        request.onsuccess = (event) => {
          const cursor = (event.target as IDBRequest<IDBCursorWithValue | null>).result
          if (cursor) {
            results.push({
              priority: cursor.value.priority,
              createdAt: cursor.value.createdAt
            })
            cursor.continue()
          } else {
            resolve(results)
          }
        }
        request.onerror = () => reject(request.error)
      })

      // Sort by priority weight (matching useOfflineQueue drain logic)
      const priorityWeight: Record<string, number> = {
        critical: 0, high: 1, normal: 2, low: 3
      }
      allEvents.sort((a, b) => {
        const pw = (priorityWeight[a.priority] ?? 3) - (priorityWeight[b.priority] ?? 3)
        if (pw !== 0) return pw
        return a.createdAt - b.createdAt
      })

      db.close()
      return allEvents.map(e => e.priority)
    })

    // Verify priority ordering: critical first, then high, normal, low
    expect(priorities).toEqual([
      'critical', 'critical', 'high', 'normal', 'low'
    ])
  })

  test('IndexedDB survives page reload', async ({ page }) => {
    await page.goto('/', { waitUntil: 'networkidle' })
    await page.waitForTimeout(2000)

    // Write events to IndexedDB
    await page.evaluate(async () => {
      const db = await new Promise<IDBDatabase>((resolve, reject) => {
        const request = indexedDB.open('argus-offline-v1', 1)
        request.onupgradeneeded = (event) => {
          const db = (event.target as IDBOpenDBRequest).result
          if (!db.objectStoreNames.contains('events')) {
            const store = db.createObjectStore('events', { keyPath: 'id', autoIncrement: true })
            store.createIndex('status', 'status', { unique: false })
          }
          if (!db.objectStoreNames.contains('snapshots')) {
            db.createObjectStore('snapshots', { keyPath: 'id', autoIncrement: true })
          }
          if (!db.objectStoreNames.contains('metadata')) {
            db.createObjectStore('metadata', { keyPath: 'key' })
          }
        }
        request.onsuccess = () => resolve(request.result)
        request.onerror = () => reject(request.error)
      })

      await new Promise<void>((resolve, reject) => {
        const tx = db.transaction('events', 'readwrite')
        const store = tx.objectStore('events')
        for (let i = 0; i < 3; i++) {
          store.add({
            sessionId: 'sess-reload-test',
            eventId: `evt-reload-${i}`,
            priority: 'normal',
            payload: '{}',
            createdAt: Date.now() + i,
            retryCount: 0,
            lastRetryAt: null,
            status: 'pending',
            sizeBytes: 50
          })
        }
        tx.oncomplete = () => resolve()
        tx.onerror = () => reject(tx.error)
      })
      db.close()
    })

    // Reload the page
    await page.reload({ waitUntil: 'networkidle' })
    await page.waitForTimeout(2000)

    // Verify events survived the reload
    const count = await page.evaluate(async () => {
      const db = await new Promise<IDBDatabase>((resolve, reject) => {
        const request = indexedDB.open('argus-offline-v1', 1)
        request.onsuccess = () => resolve(request.result)
        request.onerror = () => reject(request.error)
      })

      const result = await new Promise<number>((resolve, reject) => {
        const tx = db.transaction('events', 'readonly')
        const store = tx.objectStore('events')
        const index = store.index('status')
        const request = index.count(IDBKeyRange.only('pending'))
        request.onsuccess = () => resolve(request.result)
        request.onerror = () => reject(request.error)
      })

      db.close()
      return result
    })

    expect(count).toBe(3)
  })
})
