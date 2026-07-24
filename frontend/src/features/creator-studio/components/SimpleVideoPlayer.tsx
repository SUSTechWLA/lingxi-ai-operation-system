import { useEffect, useRef, useState, type KeyboardEvent } from 'react'

interface SimpleVideoPlayerProps {
  src: string
  title?: string
  downloadName?: string
  onError?: () => void
}

export default function SimpleVideoPlayer({ src, title = '成片预览', downloadName, onError }: SimpleVideoPlayerProps) {
  const playerRef = useRef<HTMLDivElement>(null)
  const videoRef = useRef<HTMLVideoElement>(null)
  const [isPlaying, setIsPlaying] = useState(false)
  const [currentTime, setCurrentTime] = useState(0)
  const [duration, setDuration] = useState(0)
  const [volume, setVolume] = useState(1)
  const [isMuted, setIsMuted] = useState(false)
  const [playbackRate, setPlaybackRate] = useState(1)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    setIsPlaying(false)
    setCurrentTime(0)
    setDuration(0)
    setFailed(false)
  }, [src])

  const togglePlayback = () => {
    const video = videoRef.current
    if (!video) return
    if (video.paused) {
      void video.play().catch(() => setIsPlaying(false))
    } else {
      video.pause()
    }
  }

  const seekTo = (nextTime: number) => {
    const video = videoRef.current
    if (!video) return
    const upperBound = Number.isFinite(video.duration) ? video.duration : duration
    video.currentTime = clamp(nextTime, 0, upperBound || 0)
    setCurrentTime(video.currentTime)
  }

  const toggleMute = () => {
    const video = videoRef.current
    if (!video) return
    video.muted = !video.muted
    setIsMuted(video.muted)
  }

  const changeVolume = (nextVolume: number) => {
    const video = videoRef.current
    if (!video) return
    video.volume = clamp(nextVolume, 0, 1)
    if (video.volume > 0) video.muted = false
    setVolume(video.volume)
    setIsMuted(video.muted)
  }

  const changePlaybackRate = (nextRate: number) => {
    const video = videoRef.current
    if (!video) return
    video.playbackRate = nextRate
    setPlaybackRate(video.playbackRate)
  }

  const enterFullscreen = () => {
    const request = playerRef.current?.requestFullscreen()
    if (request) void request.catch(() => undefined)
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
    } else if (event.key.toLowerCase() === 'f') {
      event.preventDefault()
      enterFullscreen()
    }
  }

  const handleError = () => {
    setFailed(true)
    setIsPlaying(false)
    onError?.()
  }

  if (failed) {
    return (
      <div className="simple-video-player is-error" role="group" aria-label={title}>
        <div className="simple-video-error" role="alert">
          <strong>当前视频无法播放</strong>
          <span>可以下载文件后使用系统播放器查看。</span>
          <a className="creator-secondary-button" href={src} download={downloadName || true} aria-label="下载视频">下载视频</a>
        </div>
      </div>
    )
  }

  return (
    <div
      ref={playerRef}
      className="simple-video-player"
      role="group"
      aria-label={title}
      tabIndex={0}
      onKeyDown={handleKeyboard}
    >
      <div className="simple-video-stage">
        <video
          ref={videoRef}
          className="artifact-video-preview"
          preload="metadata"
          src={src}
          onClick={togglePlayback}
          onLoadedMetadata={(event) => setDuration(Number.isFinite(event.currentTarget.duration) ? event.currentTarget.duration : 0)}
          onDurationChange={(event) => setDuration(Number.isFinite(event.currentTarget.duration) ? event.currentTarget.duration : 0)}
          onTimeUpdate={(event) => setCurrentTime(event.currentTarget.currentTime)}
          onPlay={() => setIsPlaying(true)}
          onPause={() => setIsPlaying(false)}
          onEnded={() => setIsPlaying(false)}
          onVolumeChange={(event) => {
            setVolume(event.currentTarget.volume)
            setIsMuted(event.currentTarget.muted)
          }}
          onRateChange={(event) => setPlaybackRate(event.currentTarget.playbackRate)}
          onError={handleError}
        >
          当前客户端无法播放视频。
        </video>
        {!isPlaying && <button type="button" className="simple-video-center-play" aria-label="播放" onClick={togglePlayback}><span aria-hidden="true">▶</span></button>}
      </div>

      <div className="simple-video-controls">
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
          onChange={(event) => seekTo(Number(event.target.value))}
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
          onChange={(event) => changeVolume(Number(event.target.value))}
        />
        <select className="simple-video-rate" value={playbackRate} aria-label="播放速度" onChange={(event) => changePlaybackRate(Number(event.target.value))}>
          <option value="0.75">0.75×</option>
          <option value="1">1×</option>
          <option value="1.25">1.25×</option>
          <option value="1.5">1.5×</option>
          <option value="2">2×</option>
        </select>
        <button type="button" className="simple-video-icon-button" aria-label="全屏" onClick={enterFullscreen}>
          <span aria-hidden="true">⛶</span>
        </button>
        <a className="simple-video-download" href={src} download={downloadName || true} aria-label="下载视频">下载</a>
      </div>
      <p className="simple-video-shortcuts">空格播放 · ←/→ 快退快进 5 秒 · M 静音 · F 全屏</p>
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
