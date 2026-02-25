// =============================================================================
// Argus AI — Origin Private File System (OPFS) Adapter
// =============================================================================
//
// OPFS provides a high-performance, quota-managed filesystem for large binary
// blobs (JPEG snapshots > 256KB). Keeping blobs in OPFS instead of IndexedDB:
//   - Avoids IDB blob serialization overhead (2-5x faster for >256KB)
//   - Eliminates IDB transaction size limits on some browsers
//   - Separates binary evidence from structured metadata (IDB stays fast)
//
// All files are stored under the `argus-snapshots/` subdirectory of the
// origin's private file system root.
//
// Graceful degradation: if OPFS is unavailable (older browsers, insecure
// context), `isOPFSAvailable()` returns false and callers fall back to
// inline IndexedDB blobs.
//
// =============================================================================

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

/** OPFS subdirectory for snapshot blobs. */
const OPFS_DIR = 'argus-snapshots'

/**
 * Blobs larger than this threshold are routed to OPFS instead of IndexedDB.
 * 256KB is the empirical crossover where OPFS outperforms IDB blob storage.
 */
export const OPFS_BLOB_THRESHOLD = 256 * 1024 // 256KB

// ---------------------------------------------------------------------------
// Capability Check
// ---------------------------------------------------------------------------

/** Cached capability result to avoid repeated async checks. */
let _opfsAvailable: boolean | null = null

// Storage telemetry counters.
let _opfsWriteCount = 0
let _idbFallbackCount = 0
let _lastError: string | null = null

/**
 * Check if the Origin Private File System is available in this browser.
 *
 * Returns true if `navigator.storage.getDirectory()` resolves successfully.
 * The result is cached after the first check.
 */
export async function isOPFSAvailable(): Promise<boolean> {
  if (_opfsAvailable !== null) return _opfsAvailable

  try {
    if (typeof navigator === 'undefined' || !navigator.storage?.getDirectory) {
      _opfsAvailable = false
      return false
    }

    // Probe: try to get the root directory handle.
    await navigator.storage.getDirectory()
    _opfsAvailable = true
    return true
  } catch {
    _opfsAvailable = false
    return false
  }
}

// ---------------------------------------------------------------------------
// Directory Helper
// ---------------------------------------------------------------------------

/**
 * Get (or create) the `argus-snapshots/` subdirectory handle.
 */
async function getSnapshotDir(): Promise<FileSystemDirectoryHandle> {
  const root = await navigator.storage.getDirectory()
  return root.getDirectoryHandle(OPFS_DIR, { create: true })
}

// ---------------------------------------------------------------------------
// CRUD Operations
// ---------------------------------------------------------------------------

/** Result of a blob write operation, indicating which storage was used. */
export interface WriteBlobResult {
  /** The storage key. */
  key: string
  /** Which storage backend was used. */
  storage: 'opfs' | 'idb-fallback'
  /** Error message if OPFS failed and IDB fallback was used. */
  error?: string
}

/**
 * Write a binary blob to OPFS under the given key.
 *
 * If OPFS fails (quota exceeded, Safari lock conflict, permission error),
 * the function does NOT throw. Instead it:
 *   1. Emits an `argus:storage-fallback` CustomEvent for telemetry
 *   2. Sets `_opfsAvailable = false` to prevent repeated failures
 *   3. Returns `{ storage: 'idb-fallback' }` so the caller stores the blob
 *      inline in IndexedDB
 *
 * The `probeOPFSHealth()` function (called every 60s by the offline queue)
 * will re-enable OPFS once it recovers.
 *
 * @param key   - Unique identifier for this blob (e.g., `snapshot-{id}`).
 * @param data  - The binary data to persist.
 * @returns Write result indicating storage backend used.
 */
export async function writeBlob(key: string, data: ArrayBuffer): Promise<WriteBlobResult> {
  try {
    const dir = await getSnapshotDir()
    const fileHandle = await dir.getFileHandle(key, { create: true })

    // Use the synchronous access handle for maximum write performance.
    // Falls back to writable stream if createSyncAccessHandle is not available.
    if ('createSyncAccessHandle' in fileHandle) {
      try {
        const accessHandle = await (fileHandle as any).createSyncAccessHandle()
        try {
          accessHandle.write(new Uint8Array(data), { at: 0 })
          accessHandle.truncate(data.byteLength)
          accessHandle.flush()
        } finally {
          accessHandle.close()
        }
        _opfsWriteCount++
        return { key, storage: 'opfs' }
      } catch {
        // Fall through to writable stream approach
      }
    }

    // Fallback: use WritableStream API (works in main thread).
    const writable = await fileHandle.createWritable()
    try {
      await writable.write(data)
    } finally {
      await writable.close()
    }

    _opfsWriteCount++
    return { key, storage: 'opfs' }
  } catch (err) {
    // OPFS completely failed — signal IDB fallback
    const errorMessage = err instanceof Error ? err.message : String(err)
    _lastError = errorMessage
    _idbFallbackCount++

    // Disable OPFS to prevent repeated failures on subsequent writes
    _opfsAvailable = false

    // Emit telemetry event for the offline queue / session composable
    if (typeof window !== 'undefined') {
      window.dispatchEvent(new CustomEvent('argus:storage-fallback', {
        detail: { key, from: 'opfs', to: 'idb', error: errorMessage }
      }))
    }

    console.warn('[argus:opfs] OPFS write failed, signaling IDB fallback', {
      key,
      error: errorMessage
    })

    return { key, storage: 'idb-fallback', error: errorMessage }
  }
}

/**
 * Read a binary blob from OPFS by key.
 *
 * @param key - The key used when writing the blob.
 * @returns The blob data, or null if the file does not exist.
 */
export async function readBlob(key: string): Promise<ArrayBuffer | null> {
  try {
    const dir = await getSnapshotDir()
    const fileHandle = await dir.getFileHandle(key)
    const file = await fileHandle.getFile()
    return await file.arrayBuffer()
  } catch {
    // File not found or read error — return null for graceful degradation.
    return null
  }
}

/**
 * Delete a blob from OPFS by key.
 * Called after successful upload to free disk space.
 *
 * @param key - The key used when writing the blob.
 */
export async function deleteBlob(key: string): Promise<void> {
  try {
    const dir = await getSnapshotDir()
    await dir.removeEntry(key)
  } catch {
    // File already deleted or never existed — ignore silently.
  }
}

/**
 * Calculate the total size of all files in the OPFS snapshot directory.
 * Used for storage quota monitoring.
 *
 * @returns Total size in bytes across all OPFS snapshot files.
 */
export async function getOPFSTotalSize(): Promise<number> {
  try {
    const dir = await getSnapshotDir()
    let totalSize = 0

    // Iterate over all files in the directory.
    for await (const [, handle] of (dir as any).entries()) {
      if (handle.kind === 'file') {
        const file = await (handle as FileSystemFileHandle).getFile()
        totalSize += file.size
      }
    }

    return totalSize
  } catch {
    return 0
  }
}

// ---------------------------------------------------------------------------
// OPFS Health Probe & Diagnostics
// ---------------------------------------------------------------------------

/**
 * Probe OPFS health by writing, reading, and deleting a tiny test blob.
 *
 * Called periodically (every 60s) by the offline queue to detect OPFS
 * recovery after a failure. On success, sets `_opfsAvailable = true`
 * so subsequent writes route to OPFS instead of IDB fallback.
 *
 * @returns true if OPFS is healthy and available.
 */
export async function probeOPFSHealth(): Promise<boolean> {
  try {
    if (typeof navigator === 'undefined' || !navigator.storage?.getDirectory) {
      _opfsAvailable = false
      return false
    }

    const testKey = '__argus_opfs_probe__'
    const testData = new Uint8Array([0x41, 0x52, 0x47, 0x55, 0x53]) // "ARGUS"

    const dir = await getSnapshotDir()

    // Write
    const fileHandle = await dir.getFileHandle(testKey, { create: true })
    const writable = await fileHandle.createWritable()
    await writable.write(testData)
    await writable.close()

    // Read & verify
    const file = await fileHandle.getFile()
    const readData = new Uint8Array(await file.arrayBuffer())
    if (readData.length !== testData.length || readData[0] !== 0x41) {
      throw new Error('OPFS probe: read-back verification failed')
    }

    // Cleanup
    await dir.removeEntry(testKey)

    // OPFS is healthy — re-enable
    const wasDisabled = _opfsAvailable === false
    _opfsAvailable = true

    if (wasDisabled) {
      console.info('[argus:opfs] OPFS recovered — switching back from IDB fallback')
    }

    return true
  } catch {
    _opfsAvailable = false
    return false
  }
}

/** Storage diagnostics for the OPFS layer. */
export interface OPFSStorageStats {
  /** Total successful OPFS writes. */
  opfsWrites: number
  /** Total writes that fell back to IDB. */
  idbFallbackWrites: number
  /** Whether OPFS is currently available. */
  opfsAvailable: boolean | null
  /** Last error message (if any). */
  lastError: string | null
}

/**
 * Get OPFS storage diagnostics.
 * Returns write counts, availability status, and last error for telemetry.
 */
export function getStorageStats(): OPFSStorageStats {
  return {
    opfsWrites: _opfsWriteCount,
    idbFallbackWrites: _idbFallbackCount,
    opfsAvailable: _opfsAvailable,
    lastError: _lastError
  }
}
