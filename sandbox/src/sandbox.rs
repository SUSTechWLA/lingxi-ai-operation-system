use std::collections::HashMap;
use std::io::Read;
use std::os::unix::process::CommandExt;
use std::path::PathBuf;
use std::process::Command as StdCommand;
use std::process::Stdio;
use std::time::{Duration, Instant};

use tonic::{Request, Response, Status};
use tracing::info;

// ── Generated protobuf types ────────────────────────────────
tonic::include_proto!("sandbox");

pub use sandbox_service_server::SandboxServiceServer;

// ── Resource limits via setrlimit ────────────────────────────
fn apply_resource_limits(memory_bytes: u64, _cpu_shares: u64, disk_bytes: u64, max_pids: u32) {
    unsafe {
        if memory_bytes > 0 {
            let rlim = libc::rlimit {
                rlim_cur: memory_bytes as libc::rlim_t,
                rlim_max: memory_bytes as libc::rlim_t,
            };
            libc::setrlimit(libc::RLIMIT_AS, &rlim);
        }
        if disk_bytes > 0 {
            let rlim = libc::rlimit {
                rlim_cur: disk_bytes as libc::rlim_t,
                rlim_max: disk_bytes as libc::rlim_t,
            };
            libc::setrlimit(libc::RLIMIT_FSIZE, &rlim);
        }
        if max_pids > 0 {
            let rlim = libc::rlimit {
                rlim_cur: max_pids as libc::rlim_t,
                rlim_max: max_pids as libc::rlim_t,
            };
            libc::setrlimit(libc::RLIMIT_NPROC, &rlim);
        }
    }
}

// ── Sandbox service implementation ──────────────────────────
#[derive(Default)]
pub struct SandboxServiceImpl;

impl SandboxServiceImpl {
    fn sandbox_dir(task_id: &str, node_id: &str) -> std::io::Result<PathBuf> {
        let dir = std::env::temp_dir()
            .join("lingxi-sandbox")
            .join(format!("{}-{}", task_id, node_id));
        std::fs::create_dir_all(&dir)?;
        Ok(dir)
    }

    fn write_input_files(dir: &PathBuf, files: &HashMap<String, Vec<u8>>) -> std::io::Result<()> {
        for (name, content) in files {
            let path = dir.join(name);
            if let Some(parent) = path.parent() {
                std::fs::create_dir_all(parent)?;
            }
            std::fs::write(&path, content)?;
        }
        Ok(())
    }

    fn cleanup(dir: &PathBuf) {
        let _ = std::fs::remove_dir_all(dir);
    }
}

#[tonic::async_trait]
impl sandbox_service_server::SandboxService for SandboxServiceImpl {
    async fn execute(
        &self,
        request: Request<ExecuteRequest>,
    ) -> Result<Response<ExecuteResult>, Status> {
        let req = request.into_inner();
        let task_id = req.task_id.clone();
        let node_id = req.node_id.clone();

        info!(
            task_id = %task_id,
            node_id = %node_id,
            command = %req.command,
            "executing in sandbox"
        );

        // Create isolated sandbox directory
        let work_dir = Self::sandbox_dir(&task_id, &node_id)
            .map_err(|e| Status::internal(format!("failed to create sandbox dir: {}", e)))?;

        // Write input files
        if !req.input_files.is_empty() {
            Self::write_input_files(&work_dir, &req.input_files)
                .map_err(|e| Status::internal(format!("failed to write input files: {}", e)))?;
        }

        // Parse limits
        let mem_limit = req.limits.as_ref().map(|l| l.memory_bytes).unwrap_or(0);
        let cpu_shares = req.limits.as_ref().map(|l| l.cpu_shares).unwrap_or(0);
        let disk_limit = req.limits.as_ref().map(|l| l.disk_bytes).unwrap_or(0);
        let pid_limit = req.limits.as_ref().map(|l| l.max_pids).unwrap_or(0);

        let timeout_sec = if req.timeout_sec > 0 { req.timeout_sec } else { 30 };
        let env_map = req.env;
        let work_dir_clone = work_dir.clone();

        // Execute in blocking thread (setrlimit must run in child process)
        let start = Instant::now();
        let result = tokio::task::spawn_blocking(move || -> (i32, Vec<u8>, Vec<u8>, bool, String) {
            let mut cmd = StdCommand::new(&req.command);
            cmd.args(&req.args)
                .current_dir(&work_dir_clone)
                .envs(env_map.into_iter())
                .stdin(Stdio::null())
                .stdout(Stdio::piped())
                .stderr(Stdio::piped());

            unsafe {
                cmd.pre_exec(move || {
                    apply_resource_limits(mem_limit, cpu_shares, disk_limit, pid_limit);
                    Ok(())
                });
            }

            let mut child = match cmd.spawn() {
                Ok(c) => c,
                Err(e) => return (-1, vec![], vec![], false, format!("spawn failed: {}", e)),
            };

            // Drain stdout/stderr in background threads to prevent pipe deadlock
            let stdout_handle = child.stdout.take().map(|mut out| {
                std::thread::spawn(move || {
                    let mut buf = Vec::new();
                    let _ = out.read_to_end(&mut buf);
                    buf
                })
            });

            let stderr_handle = child.stderr.take().map(|mut err| {
                std::thread::spawn(move || {
                    let mut buf = Vec::new();
                    let _ = err.read_to_end(&mut buf);
                    buf
                })
            });

            // Poll for completion with timeout
            let now = Instant::now();
            let status = loop {
                if now.elapsed().as_secs() >= timeout_sec as u64 {
                    let _ = child.kill();
                    break (None, true);
                }
                match child.try_wait() {
                    Ok(Some(status)) => break (Some(status), false),
                    Ok(None) => std::thread::sleep(Duration::from_millis(50)),
                    Err(e) => return (-1, vec![], vec![], false, format!("wait failed: {}", e)),
                }
            };

            let stdout = stdout_handle
                .and_then(|h| h.join().ok())
                .unwrap_or_default();
            let stderr = stderr_handle
                .and_then(|h| h.join().ok())
                .unwrap_or_default();

            match status {
                (Some(s), timed_out) => (s.code().unwrap_or(-1), stdout, stderr, timed_out, String::new()),
                (None, timed_out) => (-1, stdout, stderr, timed_out, "process killed".into()),
            }
        }).await;

        let elapsed = start.elapsed();

        let (exit_code, stdout, stderr, timed_out, error_msg) = match result {
            Ok(r) => r,
            Err(e) => (-1, vec![], vec![], false, format!("task join error: {}", e)),
        };

        // Cleanup sandbox directory
        Self::cleanup(&work_dir);

        let resource_usage = ResourceUsage {
            memory_peak_bytes: 0,
            cpu_time_usec: elapsed.as_micros() as u64,
            user_time_usec: 0,
            system_time_usec: 0,
        };

        info!(
            task_id = %task_id,
            node_id = %node_id,
            exit_code = exit_code,
            duration_ms = elapsed.as_millis() as u64,
            timed_out = timed_out,
            "sandbox execution complete"
        );

        Ok(Response::new(ExecuteResult {
            exit_code: exit_code as i32,
            stdout,
            stderr,
            timed_out,
            resource_usage: Some(resource_usage),
            error: error_msg,
        }))
    }
}
