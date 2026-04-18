package com.lingxi.ai.worker.tool.builtin;

import com.lingxi.ai.worker.tool.Tool;
import com.lingxi.ai.worker.tool.ToolContext;
import com.lingxi.ai.worker.tool.ToolResult;
import com.lingxi.ai.worker.tool.ToolType;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;

import java.io.BufferedReader;
import java.io.InputStreamReader;
import java.time.Instant;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.concurrent.TimeUnit;

/**
 * 内置Bash执行工具
 * 直接在Worker进程内执行Shell命令，无网络开销
 */
@Slf4j
@Component
public class BashTool implements Tool {

    @Value("${bash.tool.allowed-commands:*}")
    private String allowedCommands;

    @Value("${bash.tool.timeout-seconds:60}")
    private int timeoutSeconds;

    @Override
    public String getName() {
        return "bash";
    }

    @Override
    public String getDescription() {
        return "执行Shell命令，支持标准输出和错误输出捕获";
    }

    @Override
    public ToolType getType() {
        return ToolType.CUSTOM;
    }

    @Override
    public ToolResult execute(Map<String, Object> parameters, ToolContext context) {
        Instant startTime = Instant.now();

        try {
            String command = (String) parameters.get("command");
            if (command == null || command.trim().isEmpty()) {
                return ToolResult.failure("Command is required", startTime, Instant.now());
            }

            log.info("Executing bash command: taskId={}, command={}", context.getTaskId(), maskCommand(command));

            ProcessBuilder processBuilder = new ProcessBuilder();
            processBuilder.command("bash", "-c", command);
            processBuilder.redirectErrorStream(true);

            Process process = processBuilder.start();

            // 读取输出
            BufferedReader reader = new BufferedReader(new InputStreamReader(process.getInputStream()));
            List<String> outputLines = new ArrayList<>();
            String line;
            while ((line = reader.readLine()) != null) {
                outputLines.add(line);
            }

            // 等待进程结束
            boolean finished = process.waitFor(timeoutSeconds, TimeUnit.SECONDS);
            if (!finished) {
                process.destroyForcibly();
                return ToolResult.failure("Command timed out after " + timeoutSeconds + " seconds",
                        startTime, Instant.now());
            }

            int exitCode = process.exitValue();
            String output = String.join("\n", outputLines);

            if (exitCode != 0) {
                log.warn("Bash command failed with exit code {}: {}", exitCode, maskCommand(command));
                return ToolResult.failure(
                        "Command failed with exit code " + exitCode + ": " + output,
                        startTime,
                        Instant.now()
                );
            }

            log.info("Bash command executed successfully: taskId={}", context.getTaskId());

            return ToolResult.success(
                    Map.of(
                            "exitCode", exitCode,
                            "output", output,
                            "command", maskCommand(command)
                    ),
                    startTime,
                    Instant.now()
            );

        } catch (Exception e) {
            log.error("Bash command execution failed", e);
            return ToolResult.failure("Execution failed: " + e.getMessage(), startTime, Instant.now());
        }
    }

    @Override
    public boolean validateParameters(Map<String, Object> parameters) {
        return parameters != null && parameters.containsKey("command");
    }

    /**
     * 简单的命令掩码，用于日志输出
     */
    private String maskCommand(String command) {
        if (command == null) return null;
        // 可以在这里添加敏感信息过滤逻辑
        return command;
    }
}
