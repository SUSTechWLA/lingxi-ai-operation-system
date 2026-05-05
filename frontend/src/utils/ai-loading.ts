let _abort: (() => void) | null = null

export const setAIAbort = (fn: (() => void) | null) => {
  _abort = fn
}

export const cancelAI = () => {
  _abort?.()
  _abort = null
}
