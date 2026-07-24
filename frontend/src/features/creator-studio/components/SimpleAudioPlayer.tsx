import { useEffect, useRef, useState, type KeyboardEvent } from 'react'
import { secondsToIntegerMilliseconds } from '../mediaRange'
import type { MediaPlaybackState } from './SimpleVideoPlayer'

interface SimpleAudioPlayerProps {
  src: string
  title?: string
  downloadName?: string
  onError?: () => void
  onPlaybackStateChange?: (state: MediaPlaybackState) => void
}

export default function SimpleAudioPlayer({
  src,
  title = '语音预览',
  downloadName,
  onError,
  onPlaybackStateChange,
}: SimpleAudioPlayerProps) {
  const audioRef = useRef<HTMLAudioElement>(null)
  const [isPlaying, setIsPlaying] = useState(false)
  const [currentTime, setCurrentTime] = useState(0)
  const [duration, setDuration] = useState(0)
  const [volume, setVolume] = useState(1)
  const [isMuted, setIsMuted] = useState(false)
  const [playbackRate, setPlaybackRate] = useState(1)
  const [failed, setFailed] = useState(false)
  const [retries, setRetries] = useState(0)

  useEffect(() => {
    setIsPlaying(false)
    setCurrentTime(0)
    setDuration(0)
    setFailed(false)
    setRetries(0)
  }, [src])

  useEffect(() => {
    onPlaybackStateChange?.({
      currentTimeMs: secondsToIntegerMilliseconds(currentTime),
      durationMs: secondsToIntegerMilliseconds(duration),
      isPlaying,
    })
  }, [currentTime, duration, isPlaying, onPlaybackStateChange])

  const togglePlayback = () => {
    const audio = audioRef.current
    if (!audio) return
    if (audio.paused) {
      void audio.play().catch(() => setIsPlaying(false))
    } else {
      audio.pause()
    }
  }

  const seekTo = (nextTime: number) => {
    const audio = audioRef.current
    if (!audio) return
    const upperBound = Number.isFinite(audio.duration) ? audio.duration : duration
    audio.currentTime = clamp(nextTime, 0, upperBound || 0)
    setCurrentTime(audio.currentTime)
  }

  const toggleMute = () => {
    const audio = audioRef.current
    if (!audio) return
    audio.muted = !audio.muted
    setIsMuted(audio.muted)
  }

  const changeVolume = (nextVolume: number) => {
    const audio = audioRef.current
    if (!audio) return
    audio.volume = clamp(nextVolume, 0, 1)
    if (audio.volume > 0) audio.muted = false
    setVolume(audio.volume)
    setIsMuted(audio.muted)
  }

  const changePlaybackRate = (nextRate: number) => {
    const audio = audioRef.current
    if (!audio) return
    audio.playbackRate = nextRate
    setPlaybackRate(audio.playbackRate)
  }

  const handleKeyboard = (event: KeyboardEvent<HTMLDivElement>) => {
    const target = event.target
    if (target instanceof HTMLInputElement || target instanceof HTMLSelectElement || target instanceof HTMLButtonElement || target instanceof HTMLAnchorElement) return
    if (event.code === 'Space') {
      event.preventDefault()
      togglePlayback()
    } else if (event.key === 'ArrowLeft') {
      event.preventDefault()
      seekTo(currentTime - 5)
    } else if (event.key === 'ArrowRight') {
      event.preventDefault()
      seekTo(currentTime + 5)
    } else if (event.key.toLowerCase() === 'm') {
      event.preventDefault()
      toggleMute()
    }
  }

  const handleError = () => {
    setFailed(true)
    setIsPlaying(false)
    if (retries >= 2) onError?.()
  }

  if (failed) {
    return (
      <div className="simple-audio-player is-error" role="group" aria-label={title}>
        <div className="simple-audio-error" role="alert">
          <strong>当前音频无法播放</strong>
          <span>可以重新加载预览，或下载后使用系统播放器收听。</span>
          <div>
            {retries < 2 && <button type="button" className="creator-secondary-button" onClick={() => {
              setRetries(value => value + 1)
              setFailed(false)
            }}>重新加载预览</button>}
            <a className="creator-text-button" href={src} download={downloadName || true} aria-label="下载音频">下载音频</a>
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="simple-audio-player" role="group" aria-label={title} tabIndex={0} onKeyDown={handleKeyboard}>
      <audio
        key={`${src}:${retries}`}
        ref={audioRef}
        preload="metadata"
        src={src}
        onLoadedMetadata={event => setDuration(Number.isFinite(event.currentTarget.duration) ? event.currentTarget.duration : 0)}
        onDurationChange={event => setDuration(Number.isFinite(event.currentTarget.duration) ? event.currentTarget.duration : 0)}
        onTimeUpdate={event => setCurrentTime(event.currentTarget.currentTime)}
        onPlay={() => setIsPlaying(true)}
        onPause={() => setIsPlaying(false)}
        onEnded={() => setIsPlaying(false)}
        onVolumeChange={event => {
          setVolume(event.currentTarget.volume)
          setIsMuted(event.currentTarget.muted)
        }}
        onRateChange={event => setPlaybackRate(event.currentTarget.playbackRate)}
        onError={handleError}
      >
        当前客户端无法播放音频。
      </audio>
      <div className="simple-audio-heading">
        <span aria-hidden="true">{isPlaying ? '◉' : '◌'}</span>
        <div><strong>{title}</strong><small>{isPlaying ? '正在播放' : '移动播放头可选择需要修改的范围'}</small></div>
      </div>
      <div className="simple-video-controls simple-audio-controls">
        <button type="button" className="simple-video-icon-button" aria-label={isPlaying ? '暂停' : '播放'} onClick={togglePlayback}>
          <span aria-hidden="true">{isPlaying ? '❚❚' : '▶'}</span>
        </button>
        <span className="simple-video-time" aria-live="off">{formatTime(currentTime)} / {formatTime(duration)}</span>
        <input
          className="simple-video-seek"
          type="range"
          min="0"
          max={duration || 0}
          step="0.05"
          value={Math.min(currentTime, duration || 0)}
          aria-label="进度"
          onChange={event => seekTo(Number(event.target.value))}
        />
        <button type="button" className="simple-video-icon-button" aria-label={isMuted ? '取消静音' : '静音'} onClick={toggleMute}>
          <span aria-hidden="true">{isMuted || volume === 0 ? '⌁' : '◖'}</span>
        </button>
        <input
          className="simple-video-volume"
          type="range"
          min="0"
          max="1"
          step="0.05"
          value={isMuted ? 0 : volume}
          aria-label="音量"
          onChange={event => changeVolume(Number(event.target.value))}
        />
        <select className="simple-video-rate" value={playbackRate} aria-label="播放速度" onChange={event => changePlaybackRate(Number(event.target.value))}>
          <option value="0.75">0.75×</option>
          <option value="1">1×</option>
          <option value="1.25">1.25×</option>
          <option value="1.5">1.5×</option>
          <option value="2">2×</option>
        </select>
        <a className="simple-video-download" href={src} download={downloadName || true} aria-label="下载音频">下载</a>
      </div>
      <p className="simple-video-shortcuts">空格播放 · ←/→ 快退快进 5 秒 · M 静音</p>
    </div>
  )
}

function formatTime(value: number): string {
  if (!Number.isFinite(value) || value < 0) return '00:00'
  const totalSeconds = Math.floor(value)
  const seconds = String(totalSeconds % 60).padStart(2, '0')
  const minutes = Math.floor(totalSeconds / 60)
  if (minutes < 60) return `${String(minutes).padStart(2, '0')}:${seconds}`
  const hours = Math.floor(minutes / 60)
  return `${hours}:${String(minutes % 60).padStart(2, '0')}:${seconds}`
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value))
}
