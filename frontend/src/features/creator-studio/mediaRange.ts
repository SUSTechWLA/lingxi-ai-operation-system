import { createTimeSelection } from './logic'
import type { ArtifactSelection } from './types'

export type TimeSelection = Extract<ArtifactSelection, { kind: 'time' }>

export interface MediaRangeDraft {
  startMs?: number
  endMs?: number
}

interface MediaRangeState {
  selection: TimeSelection | null
  draft: MediaRangeDraft
}

interface MediaReviewPlaybackState {
  playbackAvailable: boolean
  selection: TimeSelection | null
  pending: boolean
}

interface MediaTransportState {
  currentTime: number
  duration: number
  volume: number
  isMuted: boolean
  playbackRate: number
  isPlaying: boolean
}

export function secondsToIntegerMilliseconds(seconds: number): number {
  if (!Number.isFinite(seconds)) return 0
  return Math.max(0, Math.round(seconds * 1000))
}

function integerMilliseconds(milliseconds: number): number {
  if (!Number.isFinite(milliseconds)) return 0
  return Math.max(0, Math.round(milliseconds))
}

export function updateMediaRangeBoundary(
  selection: TimeSelection | null,
  draft: MediaRangeDraft,
  boundary: 'start' | 'end',
  currentTimeMs: number,
): MediaRangeState {
  const playheadMs = integerMilliseconds(currentTimeMs)
  const counterpart = boundary === 'start'
    ? draft.endMs ?? selection?.endMs
    : draft.startMs ?? selection?.startMs

  if (counterpart === undefined) {
    return {
      selection: null,
      draft: boundary === 'start' ? { startMs: playheadMs } : { endMs: playheadMs },
    }
  }

  return {
    selection: boundary === 'start'
      ? createTimeSelection(playheadMs, counterpart)
      : createTimeSelection(counterpart, playheadMs),
    draft: {},
  }
}

export function clearMediaRange(): MediaRangeState {
  return { selection: null, draft: {} }
}

export function formatMediaTimecode(milliseconds: number): string {
  const normalized = integerMilliseconds(milliseconds)
  const totalSeconds = Math.floor(normalized / 1000)
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60
  const fraction = normalized % 1000
  const minuteTime = `${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}.${String(fraction).padStart(3, '0')}`
  return hours > 0 ? `${String(hours).padStart(2, '0')}:${minuteTime}` : minuteTime
}

export function mediaReviewAfterPlaybackFailure(state: MediaReviewPlaybackState): MediaReviewPlaybackState {
  return {
    ...state,
    playbackAvailable: false,
    selection: null,
    pending: false,
  }
}

export function audioPlaybackFailure(retries: number): { canRetry: boolean; playbackAvailable: false } {
  return {
    canRetry: retries < 2,
    playbackAvailable: false,
  }
}

export function resetMediaTransportState(): MediaTransportState {
  return {
    currentTime: 0,
    duration: 0,
    volume: 1,
    isMuted: false,
    playbackRate: 1,
    isPlaying: false,
  }
}

export function creatorRevisionInputsLocked(working: boolean, hasPendingConfirmation: boolean): boolean {
  return working || hasPendingConfirmation
}
