import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

const sessionPath = resolve(process.cwd(), 'src/core/session.ts');
const source = readFileSync(sessionPath, 'utf8');

assert.match(source, /EVENT_COLLECTOR_SERVICE_PATHS\.ingestBatch/);
assert.match(source, /EVENT_COLLECTOR_SERVICE_PATHS\.heartbeat/);
assert.match(source, /decodeResponse<Partial<IngestBatchResponse>>/);
assert.match(source, /decodeResponse<Partial<HeartbeatResponse>>/);
assert.match(source, /this\.eventBuffer\.unshift\(\.\.\.batch\)/, 'failed batches must be re-queued');
assert.match(source, /maxQueueSize/, 'SessionManager must use a bounded offline queue');
assert.match(source, /sessionStorage/, 'SessionManager should persist the queue within the browser tab session');
assert.match(source, /window\.addEventListener\('online'/, 'SessionManager should retry flush when the browser comes back online');
assert.doesNotMatch(source, /argus\.v1\.EventCollector/);
assert.doesNotMatch(source, /argus\.proctoring\.v1\.EventCollectorService/);

const transportCallCount = (source.match(/this\.transport\.call\(/g) ?? []).length;
assert.equal(transportCallCount, 2, 'SessionManager should use transport.call for batch and heartbeat only');

console.log('Verified SDK session transport path usage.');
