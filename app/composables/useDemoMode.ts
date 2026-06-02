export function useDemoMode() {
  const config = useRuntimeConfig()

  const demoMode = computed(() => {
    const value: unknown = config.public.demoMode
    if (typeof value === 'boolean') return value
    if (typeof value === 'string') return value === 'true' || value === '1'
    return false
  })

  return { demoMode }
}
