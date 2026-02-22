// =============================================================================
// Argus AI — ESLint Rule: no-duplicate-utils
// =============================================================================
//
// Prevents re-declaration of utility functions that are already centralized in
// composables (useFormatters, useStatusHelpers, useColors).
//
// This rule enforces the "Utility Genocide" policy: all shared logic MUST live
// in composable singletons. Local redefinitions are rejected at lint time.
//
// Two detection modes:
//   1. Exact name match — function name matches a known centralized utility.
//   2. Fuzzy signature match — function body is structurally similar to a known
//      utility (switch/case on status, Intl.DateTimeFormat, Math.floor + Date.now
//      patterns, byte-size formatting, etc.).
//
// Severity: error (build-blocking)
// =============================================================================

/** @type {import('eslint').Rule.RuleModule} */
export default {
  meta: {
    type: 'problem',
    docs: {
      description: 'Disallow local re-declaration of centralized utility functions',
      recommended: true,
    },
    messages: {
      duplicateName:
        'Function "{{name}}" is a centralized utility in {{source}}. Import it instead of redefining locally.',
      fuzzyMatch:
        'Function "{{name}}" matches the pattern of centralized utility "{{pattern}}" ({{source}}). Use the composable import instead.',
    },
    schema: [],
  },

  create(context) {
    // -----------------------------------------------------------------
    // Registry of protected function names → composable source
    // -----------------------------------------------------------------
    const PROTECTED_NAMES = new Map([
      // useFormatters
      ['formatDate', 'useFormatters'],
      ['formatTime', 'useFormatters'],
      ['formatDateTime', 'useFormatters'],
      ['formatDateTimeFull', 'useFormatters'],
      ['formatTimeShort', 'useFormatters'],
      ['formatVideoTimestamp', 'useFormatters'],
      ['formatTimeAgo', 'useFormatters'],
      ['formatTimeAgoMs', 'useFormatters'],
      ['formatStartTime', 'useFormatters'],
      ['formatFileSize', 'useFormatters'],
      ['formatCompactNumber', 'useFormatters'],

      // useStatusHelpers
      ['sessionStatusLabel', 'useStatusHelpers'],
      ['sessionStatusColor', 'useStatusHelpers'],
      ['sessionStatusIcon', 'useStatusHelpers'],
      ['sessionStatusBg', 'useStatusHelpers'],
      ['appealStatusLabel', 'useStatusHelpers'],
      ['appealStatusColor', 'useStatusHelpers'],
      ['appealStatusIcon', 'useStatusHelpers'],
      ['appealStatusBg', 'useStatusHelpers'],
      ['isAppealTerminal', 'useStatusHelpers'],
      ['severityColor', 'useStatusHelpers'],
      ['severityBg', 'useStatusHelpers'],
      ['severityLabel', 'useStatusHelpers'],
      ['eventIcon', 'useStatusHelpers'],
      ['isBehavioralEvent', 'useStatusHelpers'],
      ['isAudioEvent', 'useStatusHelpers'],
      ['isVisionAIEvent', 'useStatusHelpers'],
      ['sourceLabel', 'useStatusHelpers'],
      ['sourceIcon', 'useStatusHelpers'],
      ['sourceColor', 'useStatusHelpers'],
      ['isSideEvent', 'useStatusHelpers'],
      ['integrityColor', 'useStatusHelpers'],
      ['integrityGradient', 'useStatusHelpers'],
      ['exportStatusLabel', 'useStatusHelpers'],
      ['exportStatusColor', 'useStatusHelpers'],
      ['exportStatusIcon', 'useStatusHelpers'],
      ['noiseLevelColor', 'useStatusHelpers'],
      ['noiseLevelLabel', 'useStatusHelpers'],
      ['reviewDecisionLabel', 'useStatusHelpers'],
      ['reviewDecisionColor', 'useStatusHelpers'],
      ['reviewDecisionBg', 'useStatusHelpers'],
      ['reviewDecisionIcon', 'useStatusHelpers'],

      // useColors
      ['makeAccentColor', 'useColors'],
    ])

    // -----------------------------------------------------------------
    // Fuzzy pattern signatures — AST body heuristics
    // -----------------------------------------------------------------
    const FUZZY_PATTERNS = [
      {
        pattern: 'date-formatter',
        source: 'useFormatters',
        // Detects: new Intl.DateTimeFormat or toLocaleDateString('ru
        test: (src) =>
          /Intl\.DateTimeFormat/.test(src) ||
          /toLocaleDateString\s*\(\s*['"]ru/.test(src) ||
          /toLocaleString\s*\(\s*['"]ru/.test(src),
      },
      {
        pattern: 'time-ago',
        source: 'useFormatters',
        // Detects: Date.now() - ... / 1000 with "назад" string
        test: (src) =>
          /Date\.now\(\)/.test(src) && /назад/.test(src),
      },
      {
        pattern: 'file-size',
        source: 'useFormatters',
        // Detects: bytes / 1024 patterns with KB/MB/GB
        test: (src) =>
          /1024/.test(src) && /(KB|MB|GB)/.test(src),
      },
      {
        pattern: 'compact-number',
        source: 'useFormatters',
        // Detects: n >= 1_000_000 or n / 1000000 with M/K suffix
        test: (src) =>
          (/1[_,]?000[_,]?000/.test(src) || /1e6/.test(src)) &&
          /['"](M|K)['"]/.test(src),
      },
      {
        pattern: 'status-switch',
        source: 'useStatusHelpers',
        // Detects: switch with case 'reviewed'/'pending'/'confirmed'/'dismissed'/'escalated'
        test: (src) =>
          /switch\s*\(/.test(src) &&
          (
            (/['"]reviewed['"]/.test(src) && /['"]pending['"]/.test(src)) ||
            (/['"]confirmed['"]/.test(src) && /['"]dismissed['"]/.test(src)) ||
            (/['"]submitted['"]/.test(src) && /['"]under_review['"]/.test(src)) ||
            (/['"]critical['"]/.test(src) && /['"]warning['"]/.test(src) && /var\(--argus/.test(src))
          ),
      },
      {
        pattern: 'noise-level',
        source: 'useStatusHelpers',
        // Detects: dB threshold checks with argus color variables
        test: (src) =>
          />\s*(35|40|45|50|55|60)/.test(src) &&
          (/var\(--argus/.test(src) || /Высокий|Средний|Тихо/.test(src)),
      },
      {
        pattern: 'integrity-color',
        source: 'useStatusHelpers',
        // Detects: score < 50 / score < 70 with argus color vars
        test: (src) =>
          /<\s*(50|70)/.test(src) && /var\(--argus/.test(src),
      },
    ]

    // -----------------------------------------------------------------
    // Helpers
    // -----------------------------------------------------------------

    /**
     * Get the raw source text of a function body node.
     */
    function getFunctionBodySource(node) {
      const body = node.body || node.value?.body
      if (!body) return ''
      return context.sourceCode.getText(body)
    }

    /**
     * Check if the file is a composable definition (allow self-definition).
     */
    function isComposableFile() {
      const filename = context.filename || context.getFilename()
      return /composables\/use[A-Z]/.test(filename)
    }

    /**
     * Core check: run name + fuzzy matching on a function declaration.
     */
    function checkFunction(node, name) {
      if (!name || isComposableFile()) return

      // 1. Exact name match
      if (PROTECTED_NAMES.has(name)) {
        context.report({
          node,
          messageId: 'duplicateName',
          data: { name, source: PROTECTED_NAMES.get(name) },
        })
        return
      }

      // 2. Fuzzy body match (only if the function has a body)
      const src = getFunctionBodySource(node)
      if (src.length < 20) return // Trivial functions — skip

      for (const fp of FUZZY_PATTERNS) {
        if (fp.test(src)) {
          context.report({
            node,
            messageId: 'fuzzyMatch',
            data: { name, pattern: fp.pattern, source: fp.source },
          })
          return
        }
      }
    }

    // -----------------------------------------------------------------
    // Visitor
    // -----------------------------------------------------------------

    return {
      // function formatDate(...) { }
      FunctionDeclaration(node) {
        checkFunction(node, node.id?.name)
      },

      // const formatDate = (...) => { }
      // const formatDate = function(...) { }
      VariableDeclarator(node) {
        if (
          node.id?.type === 'Identifier' &&
          (node.init?.type === 'ArrowFunctionExpression' ||
            node.init?.type === 'FunctionExpression')
        ) {
          checkFunction(node.init, node.id.name)
        }
      },

      // { formatDate(...) { } } inside object
      'Property > FunctionExpression'(node) {
        const parent = node.parent
        if (parent?.type === 'Property' && parent.key?.type === 'Identifier') {
          checkFunction(node, parent.key.name)
        }
      },

      // Methods in object literal: { formatDate() { } }
      'Property[method=true]'(node) {
        if (node.key?.type === 'Identifier' && node.value?.type === 'FunctionExpression') {
          checkFunction(node.value, node.key.name)
        }
      },
    }
  },
}
