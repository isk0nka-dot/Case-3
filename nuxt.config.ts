// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
  modules: [
    '@nuxt/eslint',
    '@nuxt/ui',
    '@pinia/nuxt'
  ],

  ssr: false,

  devtools: {
    enabled: true
  },

  app: {
    head: {
      title: 'Argus AI — AI Proctoring System',
      meta: [
        { charset: 'utf-8' },
        { name: 'viewport', content: 'width=device-width, initial-scale=1' },
        { name: 'description', content: 'Argus AI — AI-powered proctoring system with Dual-Cam 360° monitoring, Face ID verification, Gaze Tracking, and Deep Browser Lockdown.' },
        { name: 'theme-color', content: '#121820' },
        { name: 'apple-mobile-web-app-capable', content: 'yes' },
        { name: 'apple-mobile-web-app-status-bar-style', content: 'black-translucent' }
      ],
      link: [
        { rel: 'icon', type: 'image/svg+xml', href: '/favicon.svg' },
        { rel: 'icon', type: 'image/x-icon', href: '/favicon-v2.ico' },
        { rel: 'icon', type: 'image/png', sizes: '32x32', href: '/favicon-32x32-v2.png' },
        { rel: 'icon', type: 'image/png', sizes: '16x16', href: '/favicon-16x16-v2.png' },
        { rel: 'apple-touch-icon', sizes: '180x180', href: '/apple-touch-icon-v2.png' },
        { rel: 'manifest', href: '/site.webmanifest' }
      ]
    }
  },

  css: ['~/assets/css/main.css', '~/assets/css/low-spec.css'],

  colorMode: {
    preference: 'dark',
    fallback: 'dark',
    classSuffix: ''
  },

  // Runtime configuration for the backend API base URL.
  // Override via environment variable:
  //   NUXT_PUBLIC_API_BASE_URL=https://api.argus.ai
  // Backwards-compat: NUXT_PUBLIC_GRPC_URL is also accepted.
  runtimeConfig: {
    public: {
      // Nuxt auto-maps NUXT_PUBLIC_API_BASE_URL env var at runtime.
      // Backwards-compat: NUXT_PUBLIC_GRPC_URL is read via fallback below.
      apiBaseUrl: 'http://localhost:8080'
    }
  },

  routeRules: {
    '/': { ssr: true }
  },

  compatibilityDate: '2025-01-15',

  eslint: {
    config: {
      stylistic: {
        commaDangle: 'never',
        braceStyle: '1tbs'
      }
    }
  }
})
