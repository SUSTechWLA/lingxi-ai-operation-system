/**
 * Shared types for the HyperFrames Render Service.
 */

export type RenderQuality = "draft" | "standard" | "high";
export type RenderFormat = "mp4" | "webm" | "mov" | "png-sequence";
export type RenderStatus =
  | "queued"
  | "preparing"
  | "rendering"
  | "encoding"
  | "complete"
  | "failed";

export interface RenderRequest {
  projectDir: string;
  entry?: string;
  outputPath: string;
  fps?: number;
  quality?: RenderQuality;
  format?: RenderFormat;
  workers?: number;
  useGpu?: boolean;
}

export interface RenderResult {
  ok: boolean;
  jobId: string;
  outputPath?: string;
  durationMs?: number;
  error?: string;
}

export interface LintRequest {
  projectDir: string;
  entry?: string;
}

export interface LintResult {
  ok: boolean;
  errors: LintMessage[];
  warnings: LintMessage[];
  entry: string;
  durationMs: number;
}

export interface LintMessage {
  rule: string;
  message: string;
  file?: string;
  line?: number;
}

export interface SnapshotRequest {
  projectDir: string;
  entry?: string;
  times: number[];
  outputDir: string;
}

export interface SnapshotResult {
  ok: boolean;
  snapshots: SnapshotFile[];
}

export interface SnapshotFile {
  timeSec: number;
  path: string;
}

export interface HealthResult {
  ok: boolean;
  service: string;
  version: string;
  dependencies: {
    node: string;
    ffmpeg: boolean;
    chromium: boolean;
    producer: boolean;
  };
}

export interface RenderProgress {
  jobId: string;
  status: RenderStatus;
  progress: number; // 0-100
  message?: string;
}
