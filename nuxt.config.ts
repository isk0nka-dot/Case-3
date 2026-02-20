// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
  modules: [
    '@nuxt/eslint',
    '@nuxt/ui',
    '@pinia/nuxt'
  ],

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
        { rel: 'icon', type: 'image/x-icon', href: '/favicon.ico' },
        { rel: 'apple-touch-icon', href: '/favicon.svg' }
      ]
    }
  },

  css: ['~/assets/css/main.css', '~/assets/css/low-spec.css'],

  colorMode: {
    preference: 'dark',
    fallback: 'dark',
    classSuffix: ''
  },

  routeRules: {
    '/': { ssr: true }
  },

  ssr: false,

  // Runtime configuration for the backend API base URL.
  // Override via environment variable:
  //   NUXT_PUBLIC_API_BASE_URL=https://api.argus.ai
  // Backwards-compat: NUXT_PUBLIC_GRPC_URL is also accepted.
  runtimeConfig: {
    public: {
      apiBaseUrl: process.env.NUXT_PUBLIC_API_BASE_URL
        || process.env.NUXT_PUBLIC_GRPC_URL
        || 'http://localhost:8080'
    }
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
