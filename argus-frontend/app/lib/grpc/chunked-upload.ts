// =============================================================================
// Argus AI — Chunked Upload Client
// =============================================================================
//
// Splits binary evidence (JPEG snapshots, audio clips) into 256KB chunks
// and uploads them sequentially to the backend via multipart/form-data.
//
// Each chunk includes a per-chunk SHA-256 checksum for integrity verification.
// Failed chunks are retried with exponential backoff.
//
// Usage:
//   await uploadChunked(baseUrl, {
//     sessionId: 'sess-123',
//     fragmentId: 'frag-456',
//     data: jpegArrayBuffer,
//     contentType: 'image/jpeg',
//     sha256: totalHash,
//     orgId: 'org-1',
//     examId: 'exam-1',
//     studentId: 'student-1'
//   }, authHeaders)
//
// =============================================================================

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** Options for a chunked upload operation. */
export interface ChunkUploadOptions {
  /** Session this evidence belongs to. */
  sessionId: string
  /** Unique fragment identifier. */
  fragmentId: string
  /** Binary data to upload. */
  data: ArrayBuffer
  /** Size of each chunk in bytes. Default: 256KB (262144). */
  chunkSizeBytes?: number
  /** MIME content type (e.g., "image/jpeg"). */
  contentType: string
  /** SHA-256 hash of the complete data (hex string). */
  sha256: string
  /** Organization ID. */
  orgId: string
  /** Exam ID. */
  examId: string
  /** Student ID. */
  studentId: string
  /** Progress callback (uploaded bytes, total bytes). */
  onProgress?: (uploaded: number, total: number) => void
  /**
   * Inter-chunk pacing delay in milliseconds.
   * Prevents chunked uploads from consuming all available bandwidth,
   * leaving headroom for real-time event streaming.
   *
   * Recommended values by resilience tier:
   *   - Tier A: 0 (full speed)
   *   - Tier B: 200ms
   *   - Tier C: 500ms
   */
  paceDelayMs?: number
}

/** Response from a single chunk upload. */
export interface ChunkUploadResponse {
  fragmentId: string
  chunkIndex: number
  totalChunks: number
  complete: boolean
  receivedChunks: number
  message: string
  /** Present when complete === true */
  uri?: string
  /** Present when complete === true */
  sha256?: string
  /** Present when complete === true */
  sizeBytes?: number
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const DEFAULT_CHUNK_SIZE = 256 * 1024 // 256KB
const MAX_RETRIES_PER_CHUNK = 3
const RETRY_BASE_DELAY_MS = 500
const RETRY_MAX_DELAY_MS = 10_000

// ---------------------------------------------------------------------------
// SHA-256 Helper
// ---------------------------------------------------------------------------

async function sha256Chunk(data: ArrayBuffer): Promise<string> {
  const hashBuffer = await crypto.subtle.digest('SHA-256', data)
  const hashArray = Array.from(new Uint8Array(hashBuffer))
  return hashArray.map(b => b.toString(16).padStart(2, '0')).join('')
}

// ---------------------------------------------------------------------------
// Upload Functions
// ---------------------------------------------------------------------------

/**
 * Upload binary data to the server in chunks.
 *
 * Each chunk is sent as multipart/form-data with metadata fields.
 * The server reassembles chunks and stores the complete file in MinIO.
 *
 * @param baseUrl - Backend base URL (e.g., "http://localhost:8080")
 * @param options - Upload options including data and metadata
 * @param headers - Auth headers (e.g., { Authorization: "Bearer ..." })
 * @returns The final chunk response containing the S3 URI and hash
 */
export async function uploadChunked(
  baseUrl: string,
  options: ChunkUploadOptions,
  headers: Record<string, string>
): Promise<ChunkUploadResponse> {
  const chunkSize = options.chunkSizeBytes ?? DEFAULT_CHUNK_SIZE
  const totalChunks = Math.ceil(options.data.byteLength / chunkSize)

  if (totalChunks === 0) {
    throw new Error('Cannot upload empty data')
  }

  let lastResponse: ChunkUploadResponse | null = null

  for (let i = 0; i < totalChunks; i++) {
    const start = i * chunkSize
    const end = Math.min(start + chunkSize, options.data.byteLength)
    const chunk = options.data.slice(start, end)

    // Compute per-chunk SHA-256
    const chunkHash = await sha256Chunk(chunk)

    // Upload with retry
    lastResponse = await uploadSingleChunk(
      baseUrl,
      {
        sessionId: options.sessionId,
        fragmentId: options.fragmentId,
        chunkIndex: i,
        totalChunks,
        data: chunk,
        sha256Chunk: chunkHash,
        sha256Total: options.sha256,
        contentType: options.contentType,
        orgId: options.orgId,
        examId: options.examId,
        studentId: options.studentId
      },
      headers
    )

    // Report progress
    options.onProgress?.(end, options.data.byteLength)

    // Bandwidth pacing: inter-chunk delay to leave headroom for real-time events.
    // Only applied between chunks (not after the last one).
    if (options.paceDelayMs && options.paceDelayMs > 0 && i < totalChunks - 1) {
      await new Promise(resolve => setTimeout(resolve, options.paceDelayMs))
    }
  }

  if (!lastResponse) {
    throw new Error('No response received from chunk upload')
  }

  return lastResponse
}

/** Parameters for a single chunk upload. */
interface SingleChunkParams {
  sessionId: string
  fragmentId: string
  chunkIndex: number
  totalChunks: number
  data: ArrayBuffer
  sha256Chunk: string
  sha256Total: string
  contentType: string
  orgId: string
  examId: string
  studentId: string
}

/**
 * Upload a single chunk with exponential backoff retry.
 */
async function uploadSingleChunk(
  baseUrl: string,
  params: SingleChunkParams,
  headers: Record<string, string>
): Promise<ChunkUploadResponse> {
  let lastError: Error | null = null

  for (let attempt = 0; attempt <= MAX_RETRIES_PER_CHUNK; attempt++) {
    try {
      // Build multipart form data
      const formData = new FormData()
      formData.append('session_id', params.sessionId)
      formData.append('fragment_id', params.fragmentId)
      formData.append('chunk_index', String(params.chunkIndex))
      formData.append('total_chunks', String(params.totalChunks))
      formData.append('sha256_chunk', params.sha256Chunk)
      formData.append('sha256_total', params.sha256Total)
      formData.append('content_type', params.contentType)
      formData.append('org_id', params.orgId)
      formData.append('exam_id', params.examId)
      formData.append('student_id', params.studentId)
      formData.append('data', new Blob([params.data], { type: params.contentType }))

      const response = await fetch(`${baseUrl}/api/v1/ingest/chunk`, {
        method: 'POST',
        headers: {
          // Don't set Content-Type — let the browser set it with boundary
          ...headers
        },
        body: formData,
        credentials: 'include'
      })

      if (!response.ok) {
        throw new Error(`HTTP ${response.status}: ${response.statusText}`)
      }

      const result = await response.json() as ChunkUploadResponse

      return result
    } catch (err) {
      lastError = err as Error

      // Don't retry on the last attempt
      if (attempt < MAX_RETRIES_PER_CHUNK) {
        const delay = Math.min(
          RETRY_BASE_DELAY_MS * Math.pow(2, attempt) * (0.5 + Math.random() * 0.5),
          RETRY_MAX_DELAY_MS
        )
        await new Promise(resolve => setTimeout(resolve, delay))
      }
    }
  }

  throw new Error(
    `Chunk ${params.chunkIndex}/${params.totalChunks} failed after ${MAX_RETRIES_PER_CHUNK + 1} attempts: ${lastError?.message}`
  )
}

/**
 * Check the upload status of a fragment.
 */
export async function getChunkStatus(
  baseUrl: string,
  fragmentId: string,
  headers: Record<string, string>
): Promise<ChunkUploadResponse> {
  const response = await fetch(`${baseUrl}/api/v1/ingest/chunk/${fragmentId}/status`, {
    method: 'GET',
    headers,
    credentials: 'include'
  })

  if (!response.ok) {
    throw new Error(`HTTP ${response.status}: ${response.statusText}`)
  }

  return await response.json() as ChunkUploadResponse
}
