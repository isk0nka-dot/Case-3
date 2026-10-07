// =============================================================================
// Argus AI — useKeyboardNav Composable
// =============================================================================
//
// Arrow key grid navigation, Enter for focus mode, Escape to unfocus.
// Awareness of grid column count for correct row wrapping.
//
// =============================================================================

import { type Ref, onMounted, onUnmounted } from 'vue'
import { useInspectorStore } from '~/stores/useInspectorStore'

export function useKeyboardNav(totalCells: Ref<number>) {
  const inspectorStore = useInspectorStore()

  function handleKeydown(e: KeyboardEvent) {
    // Don't capture when typing in inputs
    const tag = (e.target as HTMLElement)?.tagName
    if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return

    const cols = inspectorStore.gridCols()
    const total = totalCells.value
    const idx = inspectorStore.selectedCellIndex

    switch (e.key) {
      case 'ArrowRight': {
        e.preventDefault()
        const next = idx + 1
        inspectorStore.selectedCellIndex = next < total ? next : idx
        break
      }
      case 'ArrowLeft': {
        e.preventDefault()
        const prev = idx - 1
        inspectorStore.selectedCellIndex = prev >= 0 ? prev : idx
        break
      }
      case 'ArrowDown': {
        e.preventDefault()
        const below = idx + cols
        inspectorStore.selectedCellIndex = below < total ? below : idx
        break
      }
      case 'ArrowUp': {
        e.preventDefault()
        const above = idx - cols
        inspectorStore.selectedCellIndex = above >= 0 ? above : idx
        break
      }
      case 'Enter': {
        e.preventDefault()
        // Focus the currently selected session
        const sortedIds = inspectorStore.sortedSessionIds
        const targetId = sortedIds[idx]
        if (targetId) {
          inspectorStore.setFocus(targetId)
        }
        break
      }
      case 'Escape': {
        e.preventDefault()
        inspectorStore.setFocus(null)
        break
      }
    }
  }

  onMounted(() => {
    window.addEventListener('keydown', handleKeydown)
  })

  onUnmounted(() => {
    window.removeEventListener('keydown', handleKeydown)
  })
}
