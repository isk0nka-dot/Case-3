// =============================================================================
// Argus AI — IndexedDB Storage Layer
// =============================================================================
//
// Zero-dependency IndexedDB wrapper for the offline evidence queue.
// Uses the raw IndexedDB API to avoid external dependencies.
//
// Database: argus-offline-v1
// Object stores:
//   - events:    Queued proctoring events (JSON payloads)
//   - snapshots: Queued binary snapshots (JPEG ArrayBuffers)
//   - metadata:  Queue statistics and session metadata
//
// Design principles:
//   - Zero external dependencies (raw IndexedDB API)
//   - Typed wrapper over IDBObjectStore
//   - Auto-incrementing IDs for ordering
//   - Indexed by sessionId, priority, status for efficient queries
//   - Large blobs (>256KB) routed to OPFS via opfsKey field
//   - EVIDENCE IS NEVER EVICTED — storage pressure triggers HardBlockerModal
//
// =============================================================================

import {
  isOPFSAvailable,
  writeBlob as opfsWriteBlob,
  readBlob as opfsReadBlob,
  deleteBlob as opfsDeleteBlob,
  OPFS_BLOB_THRESHOLD
} from './opfs'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** Queued event record stored in IndexedDB. */
export interface QueuedEvent {
  /** Auto-incremented unique ID. */
  id?: number
  /** Session this event belongs to. */
  sessionId: string
  /** Original event ID from the proctoring system. */
  eventId: string
  /** Upload priority: critical events drain first. */
  priority: 'critical' | 'high' | 'normal' | 'low'
  /** JSON-encoded ProctoringEvent. */
  payload: string
  /** Timestamp when queued (Date.now()). */
  createdAt: number
  /** Number of upload retries attempted. */
  retryCount: number
  /** Last retry timestamp (null if never retried). */
  lastRetryAt: number | null
  /** Current status in the queue. */
  status: 'pending' | 'sending' | 'failed'
  /** Approximate size of the payload in bytes. */
  sizeBytes: number
  /** HMAC-SHA256 of payload for tamper evidence (hex string). */
  hmac?: string
}

/** Queued snapshot record stored in IndexedDB. */
export interface QueuedSnapshot {
  /** Auto-incremented unique ID. */
  id?: number
  /** Session this snapshot belongs to. */
  sessionId: string
  /** JPEG binary data (may be empty ArrayBuffer if stored in OPFS). */
  blob: ArrayBuffer
  /** SHA-256 hash of the blob (hex string). */
  sha256: string
  /** Timestamp when the frame was captured. */
  capturedAt: number
  /** Capture resolution (e.g., "480p"). */
  resolution: string
  /** JPEG quality (0-1). */
  quality: number
  /** Size of the blob in bytes. */
  sizeBytes: number
  /** Current status in the queue. */
  status: 'pending' | 'sending' | 'failed'
  /** Number of upload retries attempted. */
  retryCount: number
  /** Organization ID for server-side routing. */
  orgId: string
  /** Exam ID. */
  examId: string
  /** Student ID. */
  studentId: string
  /**
   * OPFS file key — when set, the actual blob is stored in the Origin Private
   * File System instead of inline in IndexedDB. The `blob` field is an empty
   * ArrayBuffer in this case. Use `readBlob(opfsKey)` to reconstitute.
   */
  opfsKey?: string
}

/** Metadata key-value record. */
export interface QueueMeta {
  /** Unique key for this metadata entry. */
  key: string
  /** Value (string or number). */
  value: string | number
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const DB_NAME = 'argus-offline-v1'
const DB_VERSION = 1
const STORE_EVENTS = 'events'
const STORE_SNAPSHOTS = 'snapshots'
const STORE_METADATA = 'metadata'

// ---------------------------------------------------------------------------
// Database Initialization
// ---------------------------------------------------------------------------

/** Cached database connection. */
let dbInstance: IDBDatabase | null = null

/**
 * Open or create the Argus offline database.
 * Returns a cached connection on subsequent calls.
 */
export function openDB(): Promise<IDBDatabase> {
  if (dbInstance) return Promise.resolve(dbInstance)

  return new Promise<IDBDatabase>((resolve, reject) => {
    const request = indexedDB.open(DB_NAME, DB_VERSION)

    request.onupgradeneeded = (event) => {
      const db = (event.target as IDBOpenDBRequest).result

      // Events store
      if (!db.objectStoreNames.contains(STORE_EVENTS)) {
        const eventStore = db.createObjectStore(STORE_EVENTS, {
          keyPath: 'id',
          autoIncrement: true
        })
        eventStore.createIndex('sessionId', 'sessionId', { unique: false })
        eventStore.createIndex('priority', 'priority', { unique: false })
        eventStore.createIndex('createdAt', 'createdAt', { unique: false })
        eventStore.createIndex('status', 'status', { unique: false })
        eventStore.createIndex('status_priority', ['status', 'priority'], { unique: false })
      }

      // Snapshots store
      if (!db.objectStoreNames.contains(STORE_SNAPSHOTS)) {
        const snapshotStore = db.createObjectStore(STORE_SNAPSHOTS, {
          keyPath: 'id',
          autoIncrement: true
        })
        snapshotStore.createIndex('sessionId', 'sessionId', { unique: false })
        snapshotStore.createIndex('capturedAt', 'capturedAt', { unique: false })
        snapshotStore.createIndex('status', 'status', { unique: false })
      }

      // Metadata store
      if (!db.objectStoreNames.contains(STORE_METADATA)) {
        db.createObjectStore(STORE_METADATA, { keyPath: 'key' })
      }
    }

    request.onsuccess = (event) => {
      dbInstance = (event.target as IDBOpenDBRequest).result

      // Handle unexpected close (e.g., version change from another tab)
      dbInstance.onclose = () => {
        dbInstance = null
      }

      resolve(dbInstance)
    }

    request.onerror = () => {
      reject(new Error(`Failed to open IndexedDB: ${request.error?.message}`))
    }
  })
}

/**
 * Close the database connection and clear the cache.
 */
export function closeDB(): void {
  if (dbInstance) {
    dbInstance.close()
    dbInstance = null
  }
}

// ---------------------------------------------------------------------------
// Transaction Helpers
// ---------------------------------------------------------------------------

/**
 * Execute a read-only transaction on a single store.
 */
async function readTx<T>(
  storeName: string,
  fn: (store: IDBObjectStore) => IDBRequest<T>
): Promise<T> {
  const db = await openDB()
  return new Promise<T>((resolve, reject) => {
    const tx = db.transaction(storeName, 'readonly')
    const store = tx.objectStore(storeName)
    const request = fn(store)

    request.onsuccess = () => resolve(request.result)
    request.onerror = () => reject(request.error)
  })
}

/**
 * Execute a read-write transaction on a single store.
 */
async function writeTx<T>(
  storeName: string,
  fn: (store: IDBObjectStore) => IDBRequest<T>
): Promise<T> {
  const db = await openDB()
  return new Promise<T>((resolve, reject) => {
    const tx = db.transaction(storeName, 'readwrite')
    const store = tx.objectStore(storeName)
    const request = fn(store)

    request.onsuccess = () => resolve(request.result)
    request.onerror = () => reject(request.error)
  })
}

/**
 * Execute a read-write transaction with multiple operations.
 */
async function batchWriteTx(
  storeName: string,
  fn: (store: IDBObjectStore) => void
): Promise<void> {
  const db = await openDB()
  return new Promise<void>((resolve, reject) => {
    const tx = db.transaction(storeName, 'readwrite')
    const store = tx.objectStore(storeName)

    fn(store)

    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
  })
}

// ---------------------------------------------------------------------------
// Event Queue Operations
// ---------------------------------------------------------------------------

/**
 * Add a single event to the queue.
 * Returns the auto-generated ID (IDBValidKey).
 */
export async function addEvent(event: Omit<QueuedEvent, 'id'>): Promise<number> {
  const id = await writeTx<IDBValidKey>(STORE_EVENTS, store =>
    store.add(event)
  )
  return id as number
}

/**
 * Add multiple events to the queue in a single transaction.
 * Returns the auto-generated IDs.
 */
export async function addEvents(events: Omit<QueuedEvent, 'id'>[]): Promise<number[]> {
  const db = await openDB()
  return new Promise<number[]>((resolve, reject) => {
    const tx = db.transaction(STORE_EVENTS, 'readwrite')
    const store = tx.objectStore(STORE_EVENTS)
    const ids: number[] = []

    for (const event of events) {
      const req = store.add(event)
      req.onsuccess = () => ids.push(req.result as number)
    }

    tx.oncomplete = () => resolve(ids)
    tx.onerror = () => reject(tx.error)
  })
}

/**
 * Get events by status, ordered by priority then createdAt.
 * Priority order: critical > high > normal > low.
 */
export async function getEventsByStatus(
  status: QueuedEvent['status'],
  limit: number = 50
): Promise<QueuedEvent[]> {
  const db = await openDB()
  return new Promise<QueuedEvent[]>((resolve, reject) => {
    const tx = db.transaction(STORE_EVENTS, 'readonly')
    const store = tx.objectStore(STORE_EVENTS)
    const index = store.index('status')
    const results: QueuedEvent[] = []

    const request = index.openCursor(IDBKeyRange.only(status))

    request.onsuccess = (event) => {
      const cursor = (event.target as IDBRequest<IDBCursorWithValue | null>).result
      if (cursor && results.length < limit) {
        results.push(cursor.value as QueuedEvent)
        cursor.continue()
      } else {
        // Sort by priority weight then by createdAt (oldest first)
        const priorityWeight: Record<string, number> = {
          critical: 0, high: 1, normal: 2, low: 3
        }
        results.sort((a, b) => {
          const pw = (priorityWeight[a.priority] ?? 3) - (priorityWeight[b.priority] ?? 3)
          if (pw !== 0) return pw
          return a.createdAt - b.createdAt
        })
        resolve(results)
      }
    }

    request.onerror = () => reject(request.error)
  })
}

/**
 * Get the count of events by status.
 */
export async function countEventsByStatus(status: QueuedEvent['status']): Promise<number> {
  return readTx<number>(STORE_EVENTS, (store) => {
    const index = store.index('status')
    return index.count(IDBKeyRange.only(status))
  })
}

/**
 * Update an event's status and retry count.
 */
export async function updateEventStatus(
  id: number,
  status: QueuedEvent['status'],
  incrementRetry: boolean = false
): Promise<void> {
  const db = await openDB()
  return new Promise<void>((resolve, reject) => {
    const tx = db.transaction(STORE_EVENTS, 'readwrite')
    const store = tx.objectStore(STORE_EVENTS)
    const getReq = store.get(id)

    getReq.onsuccess = () => {
      const record = getReq.result as QueuedEvent | undefined
      if (!record) {
        resolve()
        return
      }

      record.status = status
      if (incrementRetry) {
        record.retryCount++
        record.lastRetryAt = Date.now()
      }

      store.put(record)
    }

    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
  })
}

/**
 * Remove events by their IDs.
 */
export async function removeEvents(ids: number[]): Promise<void> {
  if (ids.length === 0) return

  await batchWriteTx(STORE_EVENTS, (store) => {
    for (const id of ids) {
      store.delete(id)
    }
  })
}

/**
 * Remove all events for a session.
 */
export async function clearSessionEvents(sessionId: string): Promise<void> {
  const db = await openDB()
  return new Promise<void>((resolve, reject) => {
    const tx = db.transaction(STORE_EVENTS, 'readwrite')
    const store = tx.objectStore(STORE_EVENTS)
    const index = store.index('sessionId')
    const request = index.openCursor(IDBKeyRange.only(sessionId))

    request.onsuccess = (event) => {
      const cursor = (event.target as IDBRequest<IDBCursorWithValue | null>).result
      if (cursor) {
        cursor.delete()
        cursor.continue()
      }
    }

    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
  })
}

// ---------------------------------------------------------------------------
// Snapshot Queue Operations
// ---------------------------------------------------------------------------

/**
 * Add a snapshot to the queue. Returns the auto-generated ID.
 *
 * If the blob exceeds OPFS_BLOB_THRESHOLD (256KB) and OPFS is available,
 * the blob is written to OPFS and only metadata + opfsKey are stored in IDB.
 * This keeps IndexedDB transactions fast and avoids blob size limits.
 */
export async function addSnapshot(snapshot: Omit<QueuedSnapshot, 'id'>): Promise<number> {
  let record = { ...snapshot }

  // Route large blobs to OPFS for better performance.
  if (record.blob.byteLength > OPFS_BLOB_THRESHOLD && await isOPFSAvailable()) {
    const opfsKey = `snap-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`
    try {
      await opfsWriteBlob(opfsKey, record.blob)
      // Store only metadata in IDB — blob is in OPFS.
      record = {
        ...record,
        blob: new ArrayBuffer(0), // Empty placeholder.
        opfsKey
      }
    } catch {
      // OPFS write failed — fall back to inline IDB blob.
      console.warn('[argus:idb] OPFS write failed, storing blob inline in IndexedDB')
    }
  }

  const id = await writeTx<IDBValidKey>(STORE_SNAPSHOTS, store =>
    store.add(record)
  )
  return id as number
}

/**
 * Get snapshots by status, ordered by capturedAt (oldest first).
 *
 * For snapshots stored in OPFS (opfsKey is set), the blob is reconstituted
 * from OPFS before returning. This is transparent to the caller.
 */
export async function getSnapshotsByStatus(
  status: QueuedSnapshot['status'],
  limit: number = 5
): Promise<QueuedSnapshot[]> {
  const db = await openDB()
  const rawResults = await new Promise<QueuedSnapshot[]>((resolve, reject) => {
    const tx = db.transaction(STORE_SNAPSHOTS, 'readonly')
    const store = tx.objectStore(STORE_SNAPSHOTS)
    const index = store.index('status')
    const results: QueuedSnapshot[] = []

    const request = index.openCursor(IDBKeyRange.only(status))

    request.onsuccess = (event) => {
      const cursor = (event.target as IDBRequest<IDBCursorWithValue | null>).result
      if (cursor && results.length < limit) {
        results.push(cursor.value as QueuedSnapshot)
        cursor.continue()
      } else {
        results.sort((a, b) => a.capturedAt - b.capturedAt)
        resolve(results)
      }
    }

    request.onerror = () => reject(request.error)
  })

  // Reconstitute OPFS blobs.
  for (const snap of rawResults) {
    if (snap.opfsKey) {
      const blobData = await opfsReadBlob(snap.opfsKey)
      if (blobData) {
        snap.blob = blobData
      }
    }
  }

  return rawResults
}

/**
 * Count snapshots by status.
 */
export async function countSnapshotsByStatus(status: QueuedSnapshot['status']): Promise<number> {
  return readTx<number>(STORE_SNAPSHOTS, (store) => {
    const index = store.index('status')
    return index.count(IDBKeyRange.only(status))
  })
}

/**
 * Update a snapshot's status.
 */
export async function updateSnapshotStatus(
  id: number,
  status: QueuedSnapshot['status'],
  incrementRetry: boolean = false
): Promise<void> {
  const db = await openDB()
  return new Promise<void>((resolve, reject) => {
    const tx = db.transaction(STORE_SNAPSHOTS, 'readwrite')
    const store = tx.objectStore(STORE_SNAPSHOTS)
    const getReq = store.get(id)

    getReq.onsuccess = () => {
      const record = getReq.result as QueuedSnapshot | undefined
      if (!record) {
        resolve()
        return
      }

      record.status = status
      if (incrementRetry) {
        record.retryCount++
      }

      store.put(record)
    }

    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
  })
}

/**
 * Remove snapshots by their IDs.
 * Also deletes associated OPFS blobs for snapshots with opfsKey.
 */
export async function removeSnapshots(ids: number[]): Promise<void> {
  if (ids.length === 0) return

  // First, read records to find OPFS keys before deletion.
  const db = await openDB()
  const opfsKeys: string[] = []

  await new Promise<void>((resolve, reject) => {
    const tx = db.transaction(STORE_SNAPSHOTS, 'readonly')
    const store = tx.objectStore(STORE_SNAPSHOTS)

    for (const id of ids) {
      const req = store.get(id)
      req.onsuccess = () => {
        const record = req.result as QueuedSnapshot | undefined
        if (record?.opfsKey) {
          opfsKeys.push(record.opfsKey)
        }
      }
    }

    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
  })

  // Delete IDB records.
  await batchWriteTx(STORE_SNAPSHOTS, (store) => {
    for (const id of ids) {
      store.delete(id)
    }
  })

  // Clean up OPFS blobs (best-effort, non-blocking).
  for (const key of opfsKeys) {
    opfsDeleteBlob(key).catch(() => {
      // Silently ignore — blob cleanup is best-effort.
    })
  }
}

// ---------------------------------------------------------------------------
// Metadata Operations
// ---------------------------------------------------------------------------

/**
 * Set a metadata value.
 */
export async function setMeta(key: string, value: string | number): Promise<void> {
  await writeTx(STORE_METADATA, store =>
    store.put({ key, value } as QueueMeta)
  )
}

/**
 * Get a metadata value.
 */
export async function getMeta(key: string): Promise<string | number | null> {
  const result = await readTx<QueueMeta | undefined>(STORE_METADATA, store =>
    store.get(key)
  )
  return result?.value ?? null
}

// ---------------------------------------------------------------------------
// Aggregate Operations
// ---------------------------------------------------------------------------

/**
 * Get the total size of all pending items (events + snapshots) in bytes.
 */
export async function getTotalQueueSize(): Promise<number> {
  const db = await openDB()
  return new Promise<number>((resolve, reject) => {
    let totalSize = 0

    const tx = db.transaction([STORE_EVENTS, STORE_SNAPSHOTS], 'readonly')

    // Sum event sizes
    const eventStore = tx.objectStore(STORE_EVENTS)
    const eventCursor = eventStore.openCursor()

    eventCursor.onsuccess = (event) => {
      const cursor = (event.target as IDBRequest<IDBCursorWithValue | null>).result
      if (cursor) {
        const record = cursor.value as QueuedEvent
        if (record.status !== 'failed') {
          totalSize += record.sizeBytes
        }
        cursor.continue()
      }
    }

    // Sum snapshot sizes
    const snapshotStore = tx.objectStore(STORE_SNAPSHOTS)
    const snapshotCursor = snapshotStore.openCursor()

    snapshotCursor.onsuccess = (event) => {
      const cursor = (event.target as IDBRequest<IDBCursorWithValue | null>).result
      if (cursor) {
        const record = cursor.value as QueuedSnapshot
        if (record.status !== 'failed') {
          totalSize += record.sizeBytes
        }
        cursor.continue()
      }
    }

    tx.oncomplete = () => resolve(totalSize)
    tx.onerror = () => reject(tx.error)
  })
}

/**
 * Get the age of the oldest pending item in milliseconds.
 */
export async function getOldestPendingAge(): Promise<number> {
  const db = await openDB()
  return new Promise<number>((resolve, reject) => {
    let oldestTime = Infinity

    const tx = db.transaction([STORE_EVENTS, STORE_SNAPSHOTS], 'readonly')

    const eventStore = tx.objectStore(STORE_EVENTS)
    const eventIndex = eventStore.index('status')
    const eventReq = eventIndex.openCursor(IDBKeyRange.only('pending'), 'next')

    eventReq.onsuccess = (event) => {
      const cursor = (event.target as IDBRequest<IDBCursorWithValue | null>).result
      if (cursor) {
        const record = cursor.value as QueuedEvent
        oldestTime = Math.min(oldestTime, record.createdAt)
        // Only need the first one (oldest due to index order)
      }
    }

    const snapshotStore = tx.objectStore(STORE_SNAPSHOTS)
    const snapshotIndex = snapshotStore.index('status')
    const snapReq = snapshotIndex.openCursor(IDBKeyRange.only('pending'), 'next')

    snapReq.onsuccess = (event) => {
      const cursor = (event.target as IDBRequest<IDBCursorWithValue | null>).result
      if (cursor) {
        const record = cursor.value as QueuedSnapshot
        oldestTime = Math.min(oldestTime, record.capturedAt)
      }
    }

    tx.oncomplete = () => {
      resolve(oldestTime === Infinity ? 0 : Date.now() - oldestTime)
    }
    tx.onerror = () => reject(tx.error)
  })
}

// ---------------------------------------------------------------------------
// Eviction Utility (v3.1)
// ---------------------------------------------------------------------------

/**
 * Safely evict old LOW and NORMAL priority events to free up storage space.
 * CRITICAL and HIGH events (violations, screenshots) are NEVER evicted, 
 * preserving the legal evidence chain while preventing browser crashes.
 * 
 * @param count - Maximum number of events to evict
 * @returns Number of events actually evicted
 */
export async function evictOldEvents(count: number): Promise<number> {
  const db = await openDB()
  return new Promise<number>((resolve, reject) => {
    let evicted = 0
    const tx = db.transaction(STORE_EVENTS, 'readwrite')
    const store = tx.objectStore(STORE_EVENTS)
    const index = store.index('priority') // 'low' or 'normal'

    // We evict 'low' priority first.
    const lowReq = index.openCursor(IDBKeyRange.only('low'))

    // Arrays to collect IDs (since we can't reliably sort by createdAt via cursor easily when indexing by priority)
    // Actually, we can just delete from the cursor directly, which is close enough to 'oldest first' 
    // because add() usually appends in insertion order.

    lowReq.onsuccess = (event) => {
      const cursor = (event.target as IDBRequest<IDBCursorWithValue | null>).result
      if (cursor && evicted < count) {
        cursor.delete()
        evicted++
        cursor.continue()
      } else if (evicted < count) {
        // If we ran out of 'low' events, try 'normal'
        const normalReq = index.openCursor(IDBKeyRange.only('normal'))
        normalReq.onsuccess = (e) => {
          const nCursor = (e.target as IDBRequest<IDBCursorWithValue | null>).result
          if (nCursor && evicted < count) {
            nCursor.delete()
            evicted++
            nCursor.continue()
          }
        }
      }
    }

    tx.oncomplete = () => resolve(evicted)
    tx.onerror = () => reject(tx.error)
  })
}

// ---------------------------------------------------------------------------
// SHA-256 Utility
// ---------------------------------------------------------------------------

/**
 * Compute SHA-256 hash of an ArrayBuffer using Web Crypto API.
 * Returns hex-encoded string.
 */
export async function sha256(data: ArrayBuffer): Promise<string> {
  const hashBuffer = await crypto.subtle.digest('SHA-256', data)
  const hashArray = Array.from(new Uint8Array(hashBuffer))
  return hashArray.map(b => b.toString(16).padStart(2, '0')).join('')
}

/**
 * Compute HMAC-SHA256 of a string payload using Web Crypto API.
 * Used for tamper-evident signing of queued events in IndexedDB.
 * Returns hex-encoded string.
 *
 * @param payload - The string data to sign
 * @param key - The signing key (e.g., session token)
 */
export async function hmacSha256(payload: string, key: string): Promise<string> {
  const enc = new TextEncoder()
  const cryptoKey = await crypto.subtle.importKey(
    'raw',
    enc.encode(key),
    { name: 'HMAC', hash: 'SHA-256' },
    false,
    ['sign']
  )
  const sig = await crypto.subtle.sign('HMAC', cryptoKey, enc.encode(payload))
  return Array.from(new Uint8Array(sig)).map(b => b.toString(16).padStart(2, '0')).join('')
}
