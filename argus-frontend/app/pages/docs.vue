<script setup lang="ts">
// =============================================================================
// Argus AI — Public Developer Documentation
// =============================================================================
// Public page (no auth required). Stripe/Vercel-style documentation.
// Language: English (developer-facing).
// =============================================================================

const colorMode = useColorMode()
const isDark = computed(() => colorMode.value === 'dark')

function toggleTheme() {
  colorMode.preference = isDark.value ? 'light' : 'dark'
}

// --- Sections ---
interface DocSection {
  id: string
  title: string
  icon: string
  keywords: string[]
}

const sections: DocSection[] = [
  { id: 'getting-started', title: 'Getting Started', icon: 'i-lucide-rocket', keywords: ['quickstart', 'overview', 'introduction', 'setup'] },
  { id: 'authentication', title: 'Authentication', icon: 'i-lucide-lock', keywords: ['jwt', 'api key', 'token', 'bearer', 'auth', 'login'] },
  { id: 'session-flow', title: 'Session Flow', icon: 'i-lucide-workflow', keywords: ['session', 'lifecycle', 'create', 'start', 'end', 'proctoring'] },
  { id: 'idempotency', title: 'Idempotency & Resilience', icon: 'i-lucide-shield-check', keywords: ['idempotency', 'retry', 'resilience', 'adaptive', 'cpu', 'fallback', 'circuit breaker'] },
  { id: 'webhooks', title: 'Webhooks', icon: 'i-lucide-webhook', keywords: ['webhook', 'event', 'notification', 'callback', 'violation', 'payload'] },
  { id: 'error-handling', title: 'Error Handling', icon: 'i-lucide-alert-triangle', keywords: ['error', 'code', 'retry', 'degradation', 'tier', 'graceful'] },
  { id: 'widget-integration', title: 'Widget Integration', icon: 'i-lucide-layout-template', keywords: ['widget', 'frontend', 'embed', 'preexamcheck', 'hard gate', 'camera', 'face'] },
  { id: 'api-reference', title: 'API Reference', icon: 'i-lucide-file-code', keywords: ['swagger', 'openapi', 'rest', 'grpc', 'endpoint'] }
]

// --- Scrollspy ---
const activeSection = ref('getting-started')
const sectionRefs = ref<Record<string, HTMLElement | null>>({})

function setSectionRef(id: string, el: HTMLElement | null) {
  sectionRefs.value[id] = el
}

onMounted(() => {
  const observer = new IntersectionObserver(
    (entries) => {
      for (const entry of entries) {
        if (entry.isIntersecting) {
          activeSection.value = entry.target.id
        }
      }
    },
    { rootMargin: '-80px 0px -60% 0px', threshold: 0.1 }
  )

  // Small delay to let DOM render
  setTimeout(() => {
    for (const section of sections) {
      const el = sectionRefs.value[section.id]
      if (el) observer.observe(el)
    }
  }, 100)

  onUnmounted(() => observer.disconnect())
})

function scrollToSection(id: string) {
  const el = sectionRefs.value[id]
  if (el) {
    el.scrollIntoView({ behavior: 'smooth', block: 'start' })
    activeSection.value = id
  }
  showMobileNav.value = false
}

// --- Search ---
const searchQuery = ref('')
const filteredSections = computed(() => {
  if (!searchQuery.value.trim()) return sections
  const q = searchQuery.value.toLowerCase()
  return sections.filter(s =>
    s.title.toLowerCase().includes(q)
    || s.keywords.some(k => k.includes(q))
  )
})

// --- Mobile nav ---
const showMobileNav = ref(false)

// --- Code Snippets ---
const createSessionSnippets = [
  {
    label: 'cURL',
    language: 'bash',
    code: `curl -X POST https://api.argus.ai/api/v1/sessions \\
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \\
  -H "Content-Type: application/json" \\
  -H "X-Idempotency-Key: $(uuidgen)" \\
  -d '{
    "exam_id": "MATH-401-FINAL",
    "student_id": "STU-2026-4421",
    "org_id": "org-university",
    "config": {
      "face_verification": true,
      "gaze_tracking": true,
      "side_camera": false
    }
  }'`
  },
  {
    label: 'Node.js',
    language: 'javascript',
    code: `import { randomUUID } from 'crypto';

const response = await fetch('https://api.argus.ai/api/v1/sessions', {
  method: 'POST',
  headers: {
    'Authorization': \`Bearer \${jwt}\`,
    'Content-Type': 'application/json',
    'X-Idempotency-Key': randomUUID()
  },
  body: JSON.stringify({
    exam_id: 'MATH-401-FINAL',
    student_id: 'STU-2026-4421',
    org_id: 'org-university',
    config: {
      face_verification: true,
      gaze_tracking: true,
      side_camera: false
    }
  })
});

const session = await response.json();
console.log(session.session_id);`
  }
]

const widgetSnippets = [
  {
    label: 'HTML / JS',
    language: 'html',
    code: `<!-- 1. Include the Argus Widget script -->
<script src="https://cdn.argus.ai/widget/v1/argus.min.js"><\/script>

<!-- 2. Create a container -->
<div id="argus-proctoring"></div>

<!-- 3. Initialize the widget -->
<script>
  const widget = new ArgusProctoring({
    container: '#argus-proctoring',
    sessionId: 'SESSION_ID_FROM_API',
    token: 'STUDENT_JWT_TOKEN',
    apiUrl: 'https://api.argus.ai',
    locale: 'en',
    onVerified: (result) => {
      // Student passed PreExamCheck — unlock exam content
      console.log('Ready:', result.networkMode);
      document.getElementById('exam-content').style.display = 'block';
    },
    onViolation: (event) => {
      console.warn('Violation:', event.type, event.severity);
    },
    onSessionEnd: (summary) => {
      console.log('Integrity score:', summary.integrityScore);
    }
  });

  widget.mount();
<\/script>`
  },
  {
    label: 'React',
    language: 'jsx',
    code: `import { ArgusProctoring } from '@argus-ai/react';

function ExamPage({ sessionId, token }) {
  const [isReady, setIsReady] = useState(false);

  return (
    <div>
      <ArgusProctoring
        sessionId={sessionId}
        token={token}
        apiUrl="https://api.argus.ai"
        onVerified={(result) => setIsReady(true)}
        onViolation={(event) => {
          console.warn('Violation:', event.type);
        }}
      />
      {isReady && <ExamContent />}
    </div>
  );
}`
  }
]

const webhookSnippets = [
  {
    label: 'Payload',
    language: 'json',
    code: `{
  "event": "violation.detected",
  "timestamp": "2026-02-15T14:23:45.123Z",
  "session_id": "sess_abc123",
  "exam_id": "MATH-401-FINAL",
  "student_id": "STU-2026-4421",
  "org_id": "org-university",
  "data": {
    "violation_type": "PHONE_DETECTED",
    "severity": "CRITICAL",
    "confidence": 0.94,
    "evidence_fragment_id": "frag_xyz789",
    "ai_model_version": "argus-v3.2.1"
  }
}`
  },
  {
    label: 'Node.js Handler',
    language: 'javascript',
    code: `const express = require('express');
const crypto = require('crypto');
const app = express();

app.post('/webhooks/argus', express.json(), (req, res) => {
  // 1. Verify webhook signature
  const signature = req.headers['x-argus-signature'];
  const computed = crypto
    .createHmac('sha256', WEBHOOK_SECRET)
    .update(JSON.stringify(req.body))
    .digest('hex');

  if (signature !== computed) {
    return res.status(401).json({ error: 'Invalid signature' });
  }

  // 2. Process the event
  const { event, session_id, data } = req.body;

  switch (event) {
    case 'violation.detected':
      notifyProctor(session_id, data);
      break;
    case 'session.ended':
      finalizeExam(session_id);
      break;
    case 'integrity.finalized':
      updateGradebook(session_id, data.integrity_score);
      break;
  }

  // 3. Return 200 quickly (process async)
  res.status(200).json({ received: true });
});`
  }
]

const authSnippets = [
  {
    label: 'cURL',
    language: 'bash',
    code: `# Authenticate with API key to get a JWT token
curl -X POST https://api.argus.ai/api/v1/auth/login \\
  -H "Content-Type: application/json" \\
  -d '{
    "key_id": "argus_live_7f8a9b2c3d4e5f6g",
    "secret": "sk_live_your_secret_key_here"
  }'

# Response:
# {
#   "token": "eyJhbGciOiJIUzI1NiIs...",
#   "expires_at": "2026-02-16T14:00:00Z",
#   "org_id": "org-university"
# }`
  },
  {
    label: 'Node.js',
    language: 'javascript',
    code: `const response = await fetch('https://api.argus.ai/api/v1/auth/login', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    key_id: 'argus_live_7f8a9b2c3d4e5f6g',
    secret: process.env.ARGUS_SECRET_KEY
  })
});

const { token, expires_at } = await response.json();
// Use this token in the Authorization header for all API calls
// Authorization: Bearer \${token}`
  }
]
</script>

<template>
  <div
    class="min-h-screen"
    style="background: var(--argus-bg-deep);"
  >
    <!-- ============================== -->
    <!--  TOP NAVIGATION BAR            -->
    <!-- ============================== -->
    <nav
      class="sticky top-0 z-50 border-b backdrop-blur-xl"
      :style="{
        background: isDark ? 'rgba(11,15,20,0.85)' : 'rgba(240,244,250,0.85)',
        borderColor: 'var(--argus-border)'
      }"
    >
      <div class="max-w-[1400px] mx-auto px-4 sm:px-6 lg:px-8">
        <div class="flex items-center justify-between h-16">
          <!-- Left: Logo + Title -->
          <div class="flex items-center gap-3">
            <NuxtLink
              to="/"
              class="flex items-center gap-2.5"
            >
              <div
                class="flex items-center justify-center size-8 rounded-lg"
                style="background: linear-gradient(135deg, var(--argus-accent), var(--argus-brand-purple));"
              >
                <UIcon
                  name="i-lucide-scan-eye"
                  class="size-4 text-white"
                />
              </div>
              <span
                class="text-sm font-bold tracking-tight"
                style="color: var(--argus-text);"
              >
                Argus AI
              </span>
            </NuxtLink>
            <span
              class="text-xs px-2 py-0.5 rounded-md font-medium"
              style="background: var(--argus-bg-elevated); color: var(--argus-text-dimmed);"
            >
              Documentation
            </span>
          </div>

          <!-- Center: Search -->
          <div class="hidden sm:flex items-center flex-1 max-w-md mx-8">
            <div class="relative w-full">
              <UIcon
                name="i-lucide-search"
                class="absolute left-3 top-1/2 -translate-y-1/2 size-4"
                style="color: var(--argus-text-dimmed);"
              />
              <input
                v-model="searchQuery"
                type="text"
                placeholder="Search documentation..."
                class="w-full pl-10 pr-4 py-2 rounded-lg border text-sm outline-none transition-all"
                :style="{
                  background: 'var(--argus-bg-card)',
                  borderColor: searchQuery ? 'var(--argus-accent)' : 'var(--argus-border)',
                  color: 'var(--argus-text)'
                }"
              >
            </div>
          </div>

          <!-- Right: Theme + Dashboard link -->
          <div class="flex items-center gap-3">
            <button
              class="flex items-center justify-center size-8 rounded-lg transition-all cursor-pointer"
              style="color: var(--argus-text-dimmed);"
              @click="toggleTheme"
            >
              <UIcon
                :name="isDark ? 'i-lucide-sun' : 'i-lucide-moon'"
                class="size-4"
              />
            </button>
            <NuxtLink
              to="/dashboard"
              class="hidden sm:flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium transition-all"
              style="background: var(--argus-bg-elevated); color: var(--argus-text-muted);"
            >
              <UIcon
                name="i-lucide-layout-dashboard"
                class="size-3.5"
              />
              Dashboard
            </NuxtLink>

            <!-- Mobile menu toggle -->
            <button
              class="lg:hidden flex items-center justify-center size-8 rounded-lg cursor-pointer"
              style="color: var(--argus-text-dimmed);"
              @click="showMobileNav = !showMobileNav"
            >
              <UIcon
                :name="showMobileNav ? 'i-lucide-x' : 'i-lucide-menu'"
                class="size-5"
              />
            </button>
          </div>
        </div>
      </div>
    </nav>

    <!-- ============================== -->
    <!--  MAIN LAYOUT                   -->
    <!-- ============================== -->
    <div class="max-w-[1400px] mx-auto flex">
      <!-- LEFT SIDEBAR (Navigation) -->
      <aside
        class="hidden lg:block w-[240px] shrink-0 border-r sticky top-16 h-[calc(100vh-4rem)] overflow-y-auto py-6 px-4"
        :style="{ borderColor: 'var(--argus-border)' }"
      >
        <div class="space-y-1">
          <button
            v-for="section in filteredSections"
            :key="section.id"
            class="w-full flex items-center gap-2.5 px-3 py-2 rounded-lg text-left transition-all cursor-pointer text-sm"
            :style="{
              background: activeSection === section.id ? 'rgba(56,189,248,0.08)' : 'transparent',
              color: activeSection === section.id ? 'var(--argus-accent)' : 'var(--argus-text-muted)',
              fontWeight: activeSection === section.id ? '600' : '400'
            }"
            @click="scrollToSection(section.id)"
          >
            <UIcon
              :name="section.icon"
              class="size-4 shrink-0"
            />
            {{ section.title }}
          </button>
        </div>

        <!-- Docs version badge -->
        <div class="mt-8 px-3">
          <div
            class="text-[10px] font-medium uppercase tracking-wider"
            style="color: var(--argus-text-dimmed);"
          >
            API Version
          </div>
          <div
            class="text-xs font-semibold mt-1"
            style="color: var(--argus-text-muted);"
          >
            v1.0.0 — Stable
          </div>
        </div>
      </aside>

      <!-- Mobile Navigation Overlay -->
      <Transition name="modal">
        <div
          v-if="showMobileNav"
          class="lg:hidden fixed inset-0 z-40 pt-16"
          style="background: var(--argus-bg-deep);"
        >
          <div class="p-4 space-y-1">
            <!-- Mobile search -->
            <div class="relative mb-4">
              <UIcon
                name="i-lucide-search"
                class="absolute left-3 top-1/2 -translate-y-1/2 size-4"
                style="color: var(--argus-text-dimmed);"
              />
              <input
                v-model="searchQuery"
                type="text"
                placeholder="Search documentation..."
                class="w-full pl-10 pr-4 py-2.5 rounded-lg border text-sm outline-none"
                :style="{
                  background: 'var(--argus-bg-card)',
                  borderColor: 'var(--argus-border)',
                  color: 'var(--argus-text)'
                }"
              >
            </div>
            <button
              v-for="section in filteredSections"
              :key="section.id"
              class="w-full flex items-center gap-3 px-4 py-3 rounded-lg text-left transition-all cursor-pointer"
              :style="{
                background: activeSection === section.id ? 'rgba(56,189,248,0.08)' : 'transparent',
                color: activeSection === section.id ? 'var(--argus-accent)' : 'var(--argus-text-muted)'
              }"
              @click="scrollToSection(section.id)"
            >
              <UIcon
                :name="section.icon"
                class="size-5"
              />
              <span class="text-sm font-medium">{{ section.title }}</span>
            </button>
          </div>
        </div>
      </Transition>

      <!-- CONTENT AREA -->
      <main class="flex-1 min-w-0 px-6 sm:px-8 lg:px-12 py-10">
        <!-- ================================================ -->
        <!-- SECTION: Getting Started                         -->
        <!-- ================================================ -->
        <section
          id="getting-started"
          :ref="(el) => setSectionRef('getting-started', el as HTMLElement)"
          class="mb-16 scroll-mt-20"
        >
          <h1
            class="text-3xl font-bold mb-2"
            style="color: var(--argus-text);"
          >
            Argus AI Documentation
          </h1>
          <p
            class="text-lg mb-6"
            style="color: var(--argus-text-muted);"
          >
            Integrate AI-powered exam proctoring into your platform in minutes.
          </p>

          <!-- Hero highlight cards -->
          <div class="grid sm:grid-cols-3 gap-4 mb-8">
            <div
              class="rounded-xl border p-4"
              :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
            >
              <UIcon
                name="i-lucide-zap"
                class="size-5 mb-2"
                style="color: var(--argus-accent);"
              />
              <h3
                class="text-sm font-semibold mb-1"
                style="color: var(--argus-text);"
              >
                Quick Integration
              </h3>
              <p
                class="text-xs"
                style="color: var(--argus-text-dimmed);"
              >
                4-step setup. Create a session, embed the widget, receive events.
              </p>
            </div>
            <div
              class="rounded-xl border p-4"
              :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
            >
              <UIcon
                name="i-lucide-shield-check"
                class="size-5 mb-2"
                style="color: var(--argus-success);"
              />
              <h3
                class="text-sm font-semibold mb-1"
                style="color: var(--argus-text);"
              >
                Indestructible
              </h3>
              <p
                class="text-xs"
                style="color: var(--argus-text-dimmed);"
              >
                Idempotent requests, adaptive CPU sampling, automatic failover.
              </p>
            </div>
            <div
              class="rounded-xl border p-4"
              :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
            >
              <UIcon
                name="i-lucide-brain"
                class="size-5 mb-2"
                style="color: var(--argus-brand-purple);"
              />
              <h3
                class="text-sm font-semibold mb-1"
                style="color: var(--argus-text);"
              >
                40+ AI Detectors
              </h3>
              <p
                class="text-xs"
                style="color: var(--argus-text-dimmed);"
              >
                Face, gaze, audio, object detection, deepfake analysis, liveness checks.
              </p>
            </div>
          </div>

          <!-- Integration Flow -->
          <h2
            class="text-xl font-semibold mb-4"
            style="color: var(--argus-text);"
          >
            Integration Flow
          </h2>
          <DocFlowDiagram @navigate="scrollToSection" />

          <div
            class="rounded-xl border p-4 mt-6"
            :style="{ background: 'rgba(56,189,248,0.05)', borderColor: 'rgba(56,189,248,0.15)' }"
          >
            <div class="flex items-start gap-3">
              <UIcon
                name="i-lucide-lightbulb"
                class="size-5 shrink-0 mt-0.5"
                style="color: var(--argus-accent);"
              />
              <div>
                <h4
                  class="text-sm font-semibold mb-1"
                  style="color: var(--argus-accent);"
                >
                  Base URL
                </h4>
                <p
                  class="text-xs"
                  style="color: var(--argus-text-muted);"
                >
                  All API requests use <code
                    class="px-1.5 py-0.5 rounded text-xs"
                    style="background: var(--argus-bg-elevated); color: var(--argus-accent);"
                  >https://api.argus.ai/api/v1</code>
                  — Replace with your self-hosted URL if applicable.
                </p>
              </div>
            </div>
          </div>
        </section>

        <!-- ================================================ -->
        <!-- SECTION: Authentication                          -->
        <!-- ================================================ -->
        <section
          id="authentication"
          :ref="(el) => setSectionRef('authentication', el as HTMLElement)"
          class="mb-16 scroll-mt-20"
        >
          <div class="flex items-center gap-3 mb-4">
            <div
              class="flex items-center justify-center size-8 rounded-lg"
              style="background: rgba(56,189,248,0.1);"
            >
              <UIcon
                name="i-lucide-lock"
                class="size-4"
                style="color: var(--argus-accent);"
              />
            </div>
            <h2
              class="text-xl font-semibold"
              style="color: var(--argus-text);"
            >
              Authentication
            </h2>
          </div>

          <p
            class="text-sm leading-relaxed mb-4"
            style="color: var(--argus-text-muted);"
          >
            Argus uses API key-based authentication. Each organization receives a key pair (Key ID + Secret). Exchange them for a short-lived JWT token (24h expiry) that authenticates all subsequent requests.
          </p>

          <h3
            class="text-base font-semibold mb-3 mt-6"
            style="color: var(--argus-text);"
          >
            Obtain a Token
          </h3>
          <DocCodeBlock :tabs="authSnippets" />

          <h3
            class="text-base font-semibold mb-3 mt-8"
            style="color: var(--argus-text);"
          >
            Security Model
          </h3>
          <div class="space-y-3">
            <div
              v-for="item in [
                { icon: 'i-lucide-key', title: 'API Keys', desc: 'Key ID is public, Secret is shown once at creation. Store secrets in environment variables.' },
                { icon: 'i-lucide-clock', title: 'Token Expiry', desc: 'JWT tokens expire after 24 hours. The server tolerates 30 seconds of clock skew.' },
                { icon: 'i-lucide-gauge', title: 'Rate Limits', desc: 'Default: 100 RPS per API key. Enterprise plans support custom limits.' },
                { icon: 'i-lucide-shield', title: 'RBAC', desc: 'Four roles: super_admin, org_admin, proctor, viewer. API keys inherit org-level permissions.' }
              ]"
              :key="item.title"
              class="flex items-start gap-3 p-3 rounded-lg"
              :style="{ background: 'var(--argus-bg-card)' }"
            >
              <UIcon
                :name="item.icon"
                class="size-4 shrink-0 mt-0.5"
                style="color: var(--argus-accent);"
              />
              <div>
                <h4
                  class="text-sm font-medium"
                  style="color: var(--argus-text);"
                >
                  {{ item.title }}
                </h4>
                <p
                  class="text-xs mt-0.5"
                  style="color: var(--argus-text-dimmed);"
                >
                  {{ item.desc }}
                </p>
              </div>
            </div>
          </div>
        </section>

        <!-- ================================================ -->
        <!-- SECTION: Session Flow                            -->
        <!-- ================================================ -->
        <section
          id="session-flow"
          :ref="(el) => setSectionRef('session-flow', el as HTMLElement)"
          class="mb-16 scroll-mt-20"
        >
          <div class="flex items-center gap-3 mb-4">
            <div
              class="flex items-center justify-center size-8 rounded-lg"
              style="background: rgba(52,211,153,0.1);"
            >
              <UIcon
                name="i-lucide-workflow"
                class="size-4"
                style="color: var(--argus-success);"
              />
            </div>
            <h2
              class="text-xl font-semibold"
              style="color: var(--argus-text);"
            >
              Session Flow
            </h2>
          </div>

          <p
            class="text-sm leading-relaxed mb-4"
            style="color: var(--argus-text-muted);"
          >
            A proctoring session is the central entity. It tracks one student taking one exam with a unique session ID. The full lifecycle is: create, verify hardware (PreExamCheck), monitor, and finalize.
          </p>

          <h3
            class="text-base font-semibold mb-3"
            style="color: var(--argus-text);"
          >
            Create a Session
          </h3>
          <DocCodeBlock :tabs="createSessionSnippets" />

          <h3
            class="text-base font-semibold mb-3 mt-8"
            style="color: var(--argus-text);"
          >
            Session Lifecycle
          </h3>
          <div class="space-y-2">
            <div
              v-for="(step, i) in [
                { phase: 'Created', desc: 'Session ID allocated. No data flowing yet.', color: 'var(--argus-text-dimmed)' },
                { phase: 'Verifying', desc: 'PreExamCheck runs: camera, face enrollment, storage, network probe.', color: 'var(--argus-warning)' },
                { phase: 'Active', desc: 'AI inference running. Events streamed via gRPC. Heartbeats every 30s.', color: 'var(--argus-success)' },
                { phase: 'Cooldown', desc: 'Student submitted exam. Final events flushed. Evidence uploads complete.', color: 'var(--argus-accent)' },
                { phase: 'Finalized', desc: 'Integrity score computed. Webhook dispatched. Session archived.', color: 'var(--argus-brand-purple)' }
              ]"
              :key="step.phase"
              class="flex items-start gap-3 p-3 rounded-lg"
              :style="{ background: 'var(--argus-bg-card)' }"
            >
              <div
                class="flex items-center justify-center size-6 rounded-full shrink-0 mt-0.5 text-[10px] font-bold text-white"
                :style="{ background: step.color }"
              >
                {{ i + 1 }}
              </div>
              <div>
                <h4
                  class="text-sm font-medium"
                  style="color: var(--argus-text);"
                >
                  {{ step.phase }}
                </h4>
                <p
                  class="text-xs mt-0.5"
                  style="color: var(--argus-text-dimmed);"
                >
                  {{ step.desc }}
                </p>
              </div>
            </div>
          </div>
        </section>

        <!-- ================================================ -->
        <!-- SECTION: Idempotency & Resilience                -->
        <!-- ================================================ -->
        <section
          id="idempotency"
          :ref="(el) => setSectionRef('idempotency', el as HTMLElement)"
          class="mb-16 scroll-mt-20"
        >
          <div class="flex items-center gap-3 mb-4">
            <div
              class="flex items-center justify-center size-8 rounded-lg"
              style="background: rgba(52,211,153,0.1);"
            >
              <UIcon
                name="i-lucide-shield-check"
                class="size-4"
                style="color: var(--argus-success);"
              />
            </div>
            <h2
              class="text-xl font-semibold"
              style="color: var(--argus-text);"
            >
              Idempotency & Resilience
            </h2>
          </div>

          <div
            class="rounded-xl border p-4 mb-6"
            :style="{ background: 'rgba(52,211,153,0.05)', borderColor: 'rgba(52,211,153,0.15)' }"
          >
            <p
              class="text-sm font-medium"
              style="color: var(--argus-success);"
            >
              Argus is built to be indestructible. Every request is safe to retry. Every failure has an automatic recovery path.
            </p>
          </div>

          <h3
            class="text-base font-semibold mb-3"
            style="color: var(--argus-text);"
          >
            Idempotency Keys
          </h3>
          <p
            class="text-sm leading-relaxed mb-4"
            style="color: var(--argus-text-muted);"
          >
            Include an <code
              class="px-1.5 py-0.5 rounded text-xs"
              style="background: var(--argus-bg-elevated); color: var(--argus-accent);"
            >X-Idempotency-Key</code> header with every mutating request. The server caches responses for 5 minutes — retries with the same key return the cached result without re-processing. Use a UUID v4 generated once per logical operation.
          </p>

          <h3
            class="text-base font-semibold mb-3 mt-8"
            style="color: var(--argus-text);"
          >
            Adaptive Quality Tiers
          </h3>
          <p
            class="text-sm leading-relaxed mb-3"
            style="color: var(--argus-text-muted);"
          >
            The client automatically adjusts AI inference quality based on device capabilities and network conditions:
          </p>

          <div class="grid sm:grid-cols-3 gap-3">
            <div
              v-for="tier in [
                { name: 'Tier A', score: '≥ 70', specs: '720p 15fps, 10Hz AI inference, 500ms event flush', color: 'var(--argus-success)' },
                { name: 'Tier B', score: '40–70', specs: '240p 10fps, 5Hz AI inference, 2s event flush', color: 'var(--argus-warning)' },
                { name: 'Tier C', score: '< 40', specs: 'AI disabled, 1fps/30s snapshots, 5s flush', color: 'var(--argus-error)' }
              ]"
              :key="tier.name"
              class="rounded-xl border p-4"
              :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
            >
              <div class="flex items-center gap-2 mb-2">
                <span
                  class="text-xs font-bold px-2 py-0.5 rounded-full"
                  :style="{ background: `${tier.color}15`, color: tier.color }"
                >
                  {{ tier.name }}
                </span>
                <span
                  class="text-xs"
                  style="color: var(--argus-text-dimmed);"
                >Score {{ tier.score }}</span>
              </div>
              <p
                class="text-xs leading-relaxed"
                style="color: var(--argus-text-muted);"
              >
                {{ tier.specs }}
              </p>
            </div>
          </div>

          <h3
            class="text-base font-semibold mb-3 mt-8"
            style="color: var(--argus-text);"
          >
            Resilience Features
          </h3>
          <div class="space-y-2">
            <div
              v-for="feature in [
                { icon: 'i-lucide-cpu', title: 'Adaptive CPU Sampling', desc: 'AI inference latency feeds the health governor. Sustained high CPU (>100ms for 3+ frames) automatically downgrades tier to relieve pressure.' },
                { icon: 'i-lucide-hard-drive', title: 'OPFS Dual-Write Fallback', desc: 'Evidence blobs write to OPFS (fast) with automatic IndexedDB fallback. 60-second health probe detects OPFS recovery.' },
                { icon: 'i-lucide-wifi-off', title: 'Offline Queue', desc: 'Events queue in IndexedDB when offline (up to 500MB). Automatic drain on reconnection with idempotency-safe retries.' },
                { icon: 'i-lucide-rotate-ccw', title: 'Circuit Breakers', desc: 'Server-side gobreaker protects Kafka and ClickHouse. Half-open probing for automatic recovery. Admin panic button for manual reset.' },
                { icon: 'i-lucide-clock', title: 'Clock Synchronization', desc: 'NTP-style offset computed from heartbeat responses. Running median of 5 samples rejects RTT outliers.' }
              ]"
              :key="feature.title"
              class="flex items-start gap-3 p-3 rounded-lg"
              :style="{ background: 'var(--argus-bg-card)' }"
            >
              <UIcon
                :name="feature.icon"
                class="size-4 shrink-0 mt-0.5"
                style="color: var(--argus-accent);"
              />
              <div>
                <h4
                  class="text-sm font-medium"
                  style="color: var(--argus-text);"
                >
                  {{ feature.title }}
                </h4>
                <p
                  class="text-xs mt-0.5"
                  style="color: var(--argus-text-dimmed);"
                >
                  {{ feature.desc }}
                </p>
              </div>
            </div>
          </div>
        </section>

        <!-- ================================================ -->
        <!-- SECTION: Webhooks                                -->
        <!-- ================================================ -->
        <section
          id="webhooks"
          :ref="(el) => setSectionRef('webhooks', el as HTMLElement)"
          class="mb-16 scroll-mt-20"
        >
          <div class="flex items-center gap-3 mb-4">
            <div
              class="flex items-center justify-center size-8 rounded-lg"
              style="background: rgba(251,191,36,0.1);"
            >
              <UIcon
                name="i-lucide-webhook"
                class="size-4"
                style="color: var(--argus-warning);"
              />
            </div>
            <h2
              class="text-xl font-semibold"
              style="color: var(--argus-text);"
            >
              Webhooks
            </h2>
          </div>

          <p
            class="text-sm leading-relaxed mb-4"
            style="color: var(--argus-text-muted);"
          >
            Configure webhook URLs in the Integration Dashboard to receive real-time event notifications. Each delivery includes a HMAC-SHA256 signature for verification.
          </p>

          <h3
            class="text-base font-semibold mb-3"
            style="color: var(--argus-text);"
          >
            Event Types
          </h3>
          <div
            class="rounded-xl border overflow-hidden"
            :style="{ borderColor: 'var(--argus-border)' }"
          >
            <table class="w-full text-sm">
              <thead>
                <tr :style="{ background: 'var(--argus-bg-elevated)' }">
                  <th
                    class="text-left px-4 py-2.5 text-xs font-semibold"
                    style="color: var(--argus-text-dimmed);"
                  >
                    Event
                  </th>
                  <th
                    class="text-left px-4 py-2.5 text-xs font-semibold"
                    style="color: var(--argus-text-dimmed);"
                  >
                    Description
                  </th>
                  <th
                    class="text-left px-4 py-2.5 text-xs font-semibold"
                    style="color: var(--argus-text-dimmed);"
                  >
                    Timing
                  </th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="evt in [
                    { name: 'violation.detected', desc: 'A proctoring violation was detected by AI.', timing: 'Real-time' },
                    { name: 'session.started', desc: 'Student passed PreExamCheck and began exam.', timing: 'Once per session' },
                    { name: 'session.ended', desc: 'Student submitted or session was terminated.', timing: 'Once per session' },
                    { name: 'integrity.finalized', desc: 'Final integrity score computed for the session.', timing: 'After session end' },
                    { name: 'alert.critical', desc: 'Critical violation requiring immediate attention.', timing: 'Real-time' }
                  ]"
                  :key="evt.name"
                  class="border-t"
                  :style="{ borderColor: 'var(--argus-border)' }"
                >
                  <td class="px-4 py-2.5">
                    <code
                      class="text-xs px-1.5 py-0.5 rounded"
                      style="background: var(--argus-bg-elevated); color: var(--argus-accent);"
                    >{{ evt.name }}</code>
                  </td>
                  <td
                    class="px-4 py-2.5 text-xs"
                    style="color: var(--argus-text-muted);"
                  >
                    {{ evt.desc }}
                  </td>
                  <td
                    class="px-4 py-2.5 text-xs"
                    style="color: var(--argus-text-dimmed);"
                  >
                    {{ evt.timing }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <h3
            class="text-base font-semibold mb-3 mt-8"
            style="color: var(--argus-text);"
          >
            Payload & Handler Example
          </h3>
          <DocCodeBlock :tabs="webhookSnippets" />

          <h3
            class="text-base font-semibold mb-3 mt-8"
            style="color: var(--argus-text);"
          >
            Delivery Guarantees
          </h3>
          <div class="space-y-2">
            <div
              v-for="item in [
                { icon: 'i-lucide-repeat', title: 'Retry Policy', desc: 'Failed deliveries are retried 3 times with exponential backoff (1s, 5s, 30s).' },
                { icon: 'i-lucide-check-circle', title: 'Acknowledgment', desc: 'Return HTTP 200 within 10 seconds to acknowledge receipt. Non-2xx triggers retry.' },
                { icon: 'i-lucide-fingerprint', title: 'Signature Verification', desc: 'Every payload is signed with your webhook secret using HMAC-SHA256. Always verify before processing.' }
              ]"
              :key="item.title"
              class="flex items-start gap-3 p-3 rounded-lg"
              :style="{ background: 'var(--argus-bg-card)' }"
            >
              <UIcon
                :name="item.icon"
                class="size-4 shrink-0 mt-0.5"
                style="color: var(--argus-accent);"
              />
              <div>
                <h4
                  class="text-sm font-medium"
                  style="color: var(--argus-text);"
                >
                  {{ item.title }}
                </h4>
                <p
                  class="text-xs mt-0.5"
                  style="color: var(--argus-text-dimmed);"
                >
                  {{ item.desc }}
                </p>
              </div>
            </div>
          </div>
        </section>

        <!-- ================================================ -->
        <!-- SECTION: Error Handling                          -->
        <!-- ================================================ -->
        <section
          id="error-handling"
          :ref="(el) => setSectionRef('error-handling', el as HTMLElement)"
          class="mb-16 scroll-mt-20"
        >
          <div class="flex items-center gap-3 mb-4">
            <div
              class="flex items-center justify-center size-8 rounded-lg"
              style="background: rgba(248,113,113,0.1);"
            >
              <UIcon
                name="i-lucide-alert-triangle"
                class="size-4"
                style="color: var(--argus-error);"
              />
            </div>
            <h2
              class="text-xl font-semibold"
              style="color: var(--argus-text);"
            >
              Error Handling
            </h2>
          </div>

          <p
            class="text-sm leading-relaxed mb-4"
            style="color: var(--argus-text-muted);"
          >
            Argus returns standard HTTP status codes. All error responses include a structured JSON body with machine-readable error codes.
          </p>

          <div
            class="rounded-xl border overflow-hidden"
            :style="{ borderColor: 'var(--argus-border)' }"
          >
            <table class="w-full text-sm">
              <thead>
                <tr :style="{ background: 'var(--argus-bg-elevated)' }">
                  <th
                    class="text-left px-4 py-2.5 text-xs font-semibold"
                    style="color: var(--argus-text-dimmed);"
                  >
                    Code
                  </th>
                  <th
                    class="text-left px-4 py-2.5 text-xs font-semibold"
                    style="color: var(--argus-text-dimmed);"
                  >
                    Meaning
                  </th>
                  <th
                    class="text-left px-4 py-2.5 text-xs font-semibold"
                    style="color: var(--argus-text-dimmed);"
                  >
                    Action
                  </th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="err in [
                    { code: '400', meaning: 'Bad Request', action: 'Fix request payload. Check required fields.' },
                    { code: '401', meaning: 'Unauthorized', action: 'Token expired or invalid. Re-authenticate.' },
                    { code: '403', meaning: 'Forbidden', action: 'Insufficient permissions. Check API key scope.' },
                    { code: '404', meaning: 'Not Found', action: 'Resource does not exist. Check IDs.' },
                    { code: '409', meaning: 'Conflict (Idempotent)', action: 'Duplicate request detected. Original result returned.' },
                    { code: '429', meaning: 'Rate Limited', action: 'Too many requests. Respect Retry-After header.' },
                    { code: '500', meaning: 'Server Error', action: 'Retry with exponential backoff. Include X-Idempotency-Key.' },
                    { code: '503', meaning: 'Service Degraded', action: 'Circuit breaker open. Retry after 30 seconds.' }
                  ]"
                  :key="err.code"
                  class="border-t"
                  :style="{ borderColor: 'var(--argus-border)' }"
                >
                  <td class="px-4 py-2.5">
                    <code
                      class="text-xs font-bold px-1.5 py-0.5 rounded"
                      :style="{
                        background: parseInt(err.code) >= 500 ? 'rgba(248,113,113,0.1)' : parseInt(err.code) >= 400 ? 'rgba(251,191,36,0.1)' : 'var(--argus-bg-elevated)',
                        color: parseInt(err.code) >= 500 ? 'var(--argus-error)' : parseInt(err.code) >= 400 ? 'var(--argus-warning)' : 'var(--argus-text-muted)'
                      }"
                    >
                      {{ err.code }}
                    </code>
                  </td>
                  <td
                    class="px-4 py-2.5 text-xs font-medium"
                    style="color: var(--argus-text);"
                  >
                    {{ err.meaning }}
                  </td>
                  <td
                    class="px-4 py-2.5 text-xs"
                    style="color: var(--argus-text-dimmed);"
                  >
                    {{ err.action }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>

        <!-- ================================================ -->
        <!-- SECTION: Widget Integration                      -->
        <!-- ================================================ -->
        <section
          id="widget-integration"
          :ref="(el) => setSectionRef('widget-integration', el as HTMLElement)"
          class="mb-16 scroll-mt-20"
        >
          <div class="flex items-center gap-3 mb-4">
            <div
              class="flex items-center justify-center size-8 rounded-lg"
              style="background: rgba(162,89,255,0.1);"
            >
              <UIcon
                name="i-lucide-layout-template"
                class="size-4"
                style="color: var(--argus-brand-purple);"
              />
            </div>
            <h2
              class="text-xl font-semibold"
              style="color: var(--argus-text);"
            >
              Widget Integration
            </h2>
          </div>

          <p
            class="text-sm leading-relaxed mb-4"
            style="color: var(--argus-text-muted);"
          >
            The Argus Widget is a drop-in JavaScript component that handles the entire proctoring UI. It manages camera access, face verification, AI inference, and event streaming — your application just needs to respond to callbacks.
          </p>

          <DocCodeBlock :tabs="widgetSnippets" />

          <h3
            class="text-base font-semibold mb-3 mt-8"
            style="color: var(--argus-text);"
          >
            Hard Gate: PreExamCheck
          </h3>
          <p
            class="text-sm leading-relaxed mb-4"
            style="color: var(--argus-text-muted);"
          >
            Before the exam begins, the widget launches an unclosable verification modal (PreExamCheck) that validates the student's device through 4 sequential stages. The exam content remains locked until all checks pass.
          </p>

          <!-- 4-stage hard gate diagram -->
          <div class="grid sm:grid-cols-2 lg:grid-cols-4 gap-3">
            <div
              v-for="stage in [
                { num: 1, icon: 'i-lucide-camera', title: 'Media Permissions', desc: 'Requests camera + microphone access via getUserMedia. No exam without consent.', color: 'var(--argus-accent)' },
                { num: 2, icon: 'i-lucide-scan-face', title: 'Face Enrollment', desc: 'Captures reference face via MediaPipe. Requires quality ≥0.7 for 2+ seconds.', color: 'var(--argus-success)' },
                { num: 3, icon: 'i-lucide-hard-drive', title: 'Storage Audit', desc: 'Checks navigator.storage.estimate() for ≥500MB available for evidence caching.', color: 'var(--argus-warning)' },
                { num: 4, icon: 'i-lucide-wifi', title: 'Network Probe', desc: '3x ping to /healthz. Measures RTT/jitter. Sets mode: online, degraded, or offline.', color: 'var(--argus-brand-purple)' }
              ]"
              :key="stage.num"
              class="rounded-xl border p-4"
              :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
            >
              <div class="flex items-center gap-2 mb-2">
                <div
                  class="flex items-center justify-center size-6 rounded-full text-[10px] font-bold text-white"
                  :style="{ background: stage.color }"
                >
                  {{ stage.num }}
                </div>
                <UIcon
                  :name="stage.icon"
                  class="size-4"
                  :style="{ color: stage.color }"
                />
              </div>
              <h4
                class="text-sm font-medium mb-1"
                style="color: var(--argus-text);"
              >
                {{ stage.title }}
              </h4>
              <p
                class="text-xs leading-relaxed"
                style="color: var(--argus-text-dimmed);"
              >
                {{ stage.desc }}
              </p>
            </div>
          </div>

          <div
            class="rounded-xl border p-4 mt-6"
            :style="{ background: 'rgba(162,89,255,0.05)', borderColor: 'rgba(162,89,255,0.15)' }"
          >
            <div class="flex items-start gap-3">
              <UIcon
                name="i-lucide-code"
                class="size-5 shrink-0 mt-0.5"
                style="color: var(--argus-brand-purple);"
              />
              <div>
                <h4
                  class="text-sm font-semibold mb-1"
                  style="color: var(--argus-brand-purple);"
                >
                  Verified Event Payload
                </h4>
                <p
                  class="text-xs"
                  style="color: var(--argus-text-muted);"
                >
                  When all 4 stages pass, the widget fires <code
                    class="px-1 py-0.5 rounded text-xs"
                    style="background: var(--argus-bg-elevated);"
                  >onVerified</code> with:
                  <code
                    class="px-1 py-0.5 rounded text-xs"
                    style="background: var(--argus-bg-elevated);"
                  >{ referenceFace: Blob, mediaStream: MediaStream, networkMode: 'online' | 'degraded' | 'offline' }</code>
                </p>
              </div>
            </div>
          </div>
        </section>

        <!-- ================================================ -->
        <!-- SECTION: API Reference                           -->
        <!-- ================================================ -->
        <section
          id="api-reference"
          :ref="(el) => setSectionRef('api-reference', el as HTMLElement)"
          class="mb-16 scroll-mt-20"
        >
          <div class="flex items-center gap-3 mb-4">
            <div
              class="flex items-center justify-center size-8 rounded-lg"
              style="background: rgba(56,189,248,0.1);"
            >
              <UIcon
                name="i-lucide-file-code"
                class="size-4"
                style="color: var(--argus-accent);"
              />
            </div>
            <h2
              class="text-xl font-semibold"
              style="color: var(--argus-text);"
            >
              API Reference
            </h2>
          </div>

          <p
            class="text-sm leading-relaxed mb-6"
            style="color: var(--argus-text-muted);"
          >
            The complete API specification is available via Swagger UI. Explore endpoints, test requests, and view response schemas interactively.
          </p>

          <div class="grid sm:grid-cols-2 gap-4">
            <a
              href="/api/openapi.yaml"
              target="_blank"
              class="rounded-xl border p-5 transition-all block group"
              :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
            >
              <div class="flex items-center gap-3 mb-2">
                <UIcon
                  name="i-lucide-external-link"
                  class="size-5"
                  style="color: var(--argus-accent);"
                />
                <h3
                  class="text-sm font-semibold"
                  style="color: var(--argus-text);"
                >Swagger UI</h3>
              </div>
              <p
                class="text-xs"
                style="color: var(--argus-text-dimmed);"
              >Interactive API explorer with live request testing.</p>
            </a>
            <a
              href="/api/openapi.yaml"
              target="_blank"
              class="rounded-xl border p-5 transition-all block"
              :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
            >
              <div class="flex items-center gap-3 mb-2">
                <UIcon
                  name="i-lucide-download"
                  class="size-5"
                  style="color: var(--argus-success);"
                />
                <h3
                  class="text-sm font-semibold"
                  style="color: var(--argus-text);"
                >OpenAPI Spec</h3>
              </div>
              <p
                class="text-xs"
                style="color: var(--argus-text-dimmed);"
              >Download the YAML spec for code generation or Postman import.</p>
            </a>
          </div>
        </section>

        <!-- ================================================ -->
        <!-- FOOTER                                           -->
        <!-- ================================================ -->
        <footer
          class="border-t pt-8 mt-16"
          :style="{ borderColor: 'var(--argus-border)' }"
        >
          <div class="flex items-center justify-between">
            <p
              class="text-xs"
              style="color: var(--argus-text-dimmed);"
            >
              Argus AI Documentation — v1.0.0
            </p>
            <div class="flex items-center gap-4">
              <NuxtLink
                to="/integrations"
                class="text-xs font-medium"
                style="color: var(--argus-accent);"
              >
                Integration Dashboard
              </NuxtLink>
              <NuxtLink
                to="/dashboard"
                class="text-xs font-medium"
                style="color: var(--argus-text-muted);"
              >
                Admin Panel
              </NuxtLink>
            </div>
          </div>
        </footer>
      </main>

      <!-- RIGHT SIDEBAR (Table of Contents) -->
      <aside
        class="hidden xl:block w-[200px] shrink-0 sticky top-16 h-[calc(100vh-4rem)] overflow-y-auto py-6 px-4"
      >
        <div
          class="text-[10px] font-semibold uppercase tracking-wider mb-3"
          style="color: var(--argus-text-dimmed);"
        >
          On this page
        </div>
        <div class="space-y-1">
          <button
            v-for="section in sections"
            :key="section.id"
            class="w-full text-left px-2 py-1 rounded text-xs transition-all cursor-pointer"
            :style="{
              color: activeSection === section.id ? 'var(--argus-accent)' : 'var(--argus-text-dimmed)',
              fontWeight: activeSection === section.id ? '600' : '400',
              borderLeft: activeSection === section.id ? '2px solid var(--argus-accent)' : '2px solid transparent'
            }"
            @click="scrollToSection(section.id)"
          >
            {{ section.title }}
          </button>
        </div>
      </aside>
    </div>
  </div>
</template>
