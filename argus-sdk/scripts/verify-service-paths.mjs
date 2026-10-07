import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const scriptDir = dirname(fileURLToPath(import.meta.url));
const sdkRoot = resolve(scriptDir, '..');
const workspaceRoot = resolve(sdkRoot, '..');

const protoPath = resolve(workspaceRoot, 'argus-backend/api/proto/v1/event_collector.proto');
const sdkPathsPath = resolve(sdkRoot, 'src/core/service-paths.ts');

const proto = readFileSync(protoPath, 'utf8');
const sdkPaths = readFileSync(sdkPathsPath, 'utf8');

const packageMatch = proto.match(/^package\s+([a-zA-Z0-9_.]+);/m);
const serviceMatch = proto.match(/^service\s+([A-Za-z0-9_]+)\s*\{/m);
const methodMatches = [...proto.matchAll(/^\s*rpc\s+([A-Za-z0-9_]+)\s*\(/gm)];

if (!packageMatch || !serviceMatch || methodMatches.length === 0) {
  throw new Error(`Unable to parse EventCollector proto at ${protoPath}`);
}

const packageName = packageMatch[1];
const serviceName = serviceMatch[1];
const expectedPaths = methodMatches.map((match) => `/${packageName}.${serviceName}/${match[1]}`);

const missing = expectedPaths.filter((path) => !sdkPaths.includes(path));
const staleNamespace = /argus\.(v1|proctoring\.v1)\.EventCollector/.test(sdkPaths);

if (missing.length > 0 || staleNamespace) {
  console.error('EventCollector service path contract failed.');
  if (missing.length > 0) {
    console.error('Missing SDK paths:');
    for (const path of missing) console.error(`  - ${path}`);
  }
  if (staleNamespace) {
    console.error('SDK service paths contain a stale EventCollector namespace.');
  }
  process.exit(1);
}

console.log(`Verified ${expectedPaths.length} EventCollector gRPC-Web paths.`);

