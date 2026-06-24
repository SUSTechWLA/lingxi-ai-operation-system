# Local Commands Contract

EdgeRun CloseLoop 本地命令协议 — 定义 cloud-backend 和 local-backend 之间每个 local command 的语义、payload、output 和约束。

## 约定

- `local://` URI 由 **local-backend PathGuard** 解析为本地文件系统路径
- cloud-backend 不访问本机文件系统，只通过 LocalJob 下发语义命令
- 所有 executor 必须通过 PathGuard 校验输入输出路径

## 命令清单

| Command | Payload Schema | Output Schema | 路径校验 | 用户确认 | 允许重试 |
|---------|---------------|---------------|---------|---------|---------|
| `HYPERFRAMES_PROJECT_GENERATE` | projectId, topic, script, shotList, videoPrompts, style, publishCopy | projectDir, entry, files, summary | ✅ (output dir) | after_artifact | ✅ |
| `HYPERFRAMES_RENDER` | projectId, projectDir, entry, outputPath, fps, quality, format | outputRef, artifacts, metrics, renderJobId | ✅ (input + output) | before_execute | ✅ |
| `HYPERFRAMES_LINT` | projectDir, entry | errors, warnings | ✅ (input) | ❌ | ✅ |
| `HYPERFRAMES_SNAPSHOT` | projectDir, entry, outputPath | snapshotRef | ✅ (input + output) | ❌ | ✅ |
| `FFMPEG_PROBE` | input | media (durationSec, width, height, videoCodec, audioCodec, fps, bitrate, hasAudio) | ✅ (input) | ❌ | ✅ |
| `FFMPEG_CLIP_EXTRACT` | input, startSec, durationSec, output | outputRef | ✅ (input + output) | ❌ | ✅ |
| `FFMPEG_ASSEMBLE` | inputs[], output, transition | outputRef | ✅ (input + output) | ❌ | ✅ |
| `AUDIO_EXTRACT` | input, output, format | outputRef | ✅ (input + output) | ❌ | ✅ |
| `AUDIO_NORMALIZE` | input, output, targetLUFS | outputRef | ✅ (input + output) | ❌ | ✅ |
| `ASR_TRANSCRIBE` | input, language, model | transcript, segments | ✅ (input) | ❌ | ✅ |
| `ARTIFACT_PACKAGE` | projectId, include[], output | outputRef, sizeBytes, fileCount, files, artifacts | ✅ (input + output) | ❌ | ✅ |
| `LOCAL_FILE_IMPORT` | sourcePath, projectId, targetName | storageRef, mimeType, sizeBytes | ✅ (source + target) | ✅ | ❌ |
| `LOCAL_MEDIA_INDEX` | projectId, directory | files[], totalSizeBytes | ✅ (directory) | ❌ | ✅ |

## 安全约束

### PathGuard 规则

**允许的 local:// 前缀：**
- `local://projects/` → `<DataDir>/projects/`
- `local://artifacts/` → `<DataDir>/artifacts/`
- `local://cache/` → `<DataDir>/cache/`
- `local://logs/` → `<DataDir>/logs/`

**禁止：**
- `../` 路径遍历
- `~/.ssh`、`~/.aws` 等敏感目录
- 系统目录 (`/etc/`、`/System/`、`/private/etc/`、`/var/root/`)
- 未授权的绝对路径

### Runner 认证

所有 local-backend → cloud-backend 请求携带：
```
Authorization: Bearer <token>
X-Device-ID: <device_id>
X-Runner-ID: <runner_id>
X-Runner-Session-ID: <session_id>
```

cloud-backend 校验：
1. runner 存在且状态不为 REVOKED
2. runner.user_id == 认证 user_id
3. runner.device_id == X-Device-ID
4. runner.session_id == X-Runner-Session-ID

### Job 访问控制

- complete/fail 只能由 claim 该 job 的 runner 上报
- 已 COMPLETED/FAILED 的 job 不可修改（幂等）
- progress 上报需要 X-Runner-ID 匹配 job.runner_id

## 断网恢复 (Pending Report)

1. 本地执行完成后，先写 `pending_reports/<jobId>.<complete|fail>.json`
2. 尝试上报 cloud
3. 上报成功后删除 pending report
4. 上报失败则保留
5. 下次 heartbeat 成功后重试所有 pending reports
6. cloud complete/fail 幂等：重复上报相同 jobId 返回已有结果，不重复推进 DAG
