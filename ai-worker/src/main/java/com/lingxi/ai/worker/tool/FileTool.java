package com.lingxi.ai.worker.tool;

import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;

import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.time.Instant;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.stream.Collectors;
import java.util.stream.Stream;

/**
 * 文件工具 - 支持本地文件读写
 */
@Slf4j
@Component
public class FileTool implements Tool {

    private final Path basePath;
    private final Set<Path> allowedPaths;

    public FileTool(
            @Value("${file.tool.base-path:/tmp/ai-worker-files}") String basePathStr,
            @Value("${file.tool.allowed-paths:/tmp,/var/tmp}") List<String> allowedPathsStr) {
        this.basePath = Paths.get(basePathStr).toAbsolutePath().normalize();
        this.allowedPaths = allowedPathsStr.stream()
                .map(p -> Paths.get(p).toAbsolutePath().normalize())
                .collect(Collectors.toSet());
        this.allowedPaths.add(this.basePath);

        try {
            Files.createDirectories(this.basePath);
        } catch (IOException e) {
            log.warn("Could not create base directory: {}", this.basePath, e);
        }
    }

    @Override
    public String getName() {
        return "file";
    }

    @Override
    public String getDescription() {
        return "文件工具，支持读取、写入、列出文件";
    }

    @Override
    public ToolType getType() {
        return ToolType.FILE;
    }

    @Override
    public ToolResult execute(Map<String, Object> parameters, ToolContext context) {
        Instant startTime = Instant.now();
        log.info("Executing File tool for task: {}, node: {}", context.getTaskId(), context.getNodeId());

        try {
            String operation = getParameter(parameters, "operation", String.class, "read");

            Map<String, Object> resultData;
            switch (operation.toLowerCase()) {
                case "read" -> resultData = readFile(parameters);
                case "write" -> resultData = writeFile(parameters);
                case "list" -> resultData = listFiles(parameters);
                case "delete" -> resultData = deleteFile(parameters);
                default -> throw new IllegalArgumentException("Unsupported operation: " + operation);
            }

            log.info("File tool executed successfully: operation={}", operation);
            return ToolResult.success(resultData, startTime, Instant.now());

        } catch (Exception e) {
            log.error("File tool execution failed for task: {}", context.getTaskId(), e);
            return ToolResult.failure(e.getMessage(), startTime, Instant.now());
        }
    }

    private Map<String, Object> readFile(Map<String, Object> parameters) throws IOException {
        String pathStr = getParameter(parameters, "path", String.class);
        if (pathStr == null) {
            throw new IllegalArgumentException("path is required for read operation");
        }

        Path path = resolveAndValidatePath(pathStr);
        String content = Files.readString(path);

        Map<String, Object> result = new HashMap<>();
        result.put("operation", "read");
        result.put("path", path.toString());
        result.put("content", content);
        result.put("size", Files.size(path));
        result.put("lastModified", Files.getLastModifiedTime(path).toMillis());
        return result;
    }

    private Map<String, Object> writeFile(Map<String, Object> parameters) throws IOException {
        String pathStr = getParameter(parameters, "path", String.class);
        String content = getParameter(parameters, "content", String.class, "");
        Boolean append = getParameter(parameters, "append", Boolean.class, false);

        if (pathStr == null) {
            throw new IllegalArgumentException("path is required for write operation");
        }

        Path path = resolveAndValidatePath(pathStr);
        Path parentDir = path.getParent();
        if (parentDir != null) {
            Files.createDirectories(parentDir);
        }

        if (append) {
            Files.writeString(path, content, java.nio.file.StandardOpenOption.CREATE, java.nio.file.StandardOpenOption.APPEND);
        } else {
            Files.writeString(path, content);
        }

        Map<String, Object> result = new HashMap<>();
        result.put("operation", "write");
        result.put("path", path.toString());
        result.put("size", Files.size(path));
        result.put("append", append);
        return result;
    }

    private Map<String, Object> listFiles(Map<String, Object> parameters) throws IOException {
        String pathStr = getParameter(parameters, "path", String.class, ".");
        Boolean recursive = getParameter(parameters, "recursive", Boolean.class, false);

        Path path = resolveAndValidatePath(pathStr);

        List<Map<String, Object>> files;
        try (Stream<Path> stream = recursive ? Files.walk(path) : Files.list(path)) {
            files = stream
                    .filter(Files::isRegularFile)
                    .map(p -> {
                        Map<String, Object> fileInfo = new HashMap<>();
                        try {
                            fileInfo.put("path", path.relativize(p).toString());
                            fileInfo.put("size", Files.size(p));
                            fileInfo.put("lastModified", Files.getLastModifiedTime(p).toMillis());
                        } catch (IOException e) {
                            fileInfo.put("error", e.getMessage());
                        }
                        return fileInfo;
                    })
                    .collect(Collectors.toList());
        }

        Map<String, Object> result = new HashMap<>();
        result.put("operation", "list");
        result.put("path", path.toString());
        result.put("count", files.size());
        result.put("files", files);
        return result;
    }

    private Map<String, Object> deleteFile(Map<String, Object> parameters) throws IOException {
        String pathStr = getParameter(parameters, "path", String.class);
        if (pathStr == null) {
            throw new IllegalArgumentException("path is required for delete operation");
        }

        Path path = resolveAndValidatePath(pathStr);
        boolean existed = Files.exists(path);
        if (existed) {
            Files.delete(path);
        }

        Map<String, Object> result = new HashMap<>();
        result.put("operation", "delete");
        result.put("path", path.toString());
        result.put("existed", existed);
        return result;
    }

    private Path resolveAndValidatePath(String pathStr) {
        Path path = basePath.resolve(pathStr).toAbsolutePath().normalize();

        boolean allowed = allowedPaths.stream()
                .anyMatch(allowedPath -> path.startsWith(allowedPath));

        if (!allowed) {
            throw new SecurityException("Access to path '" + path + "' is not allowed");
        }

        return path;
    }

    @Override
    public boolean validateParameters(Map<String, Object> parameters) {
        if (parameters == null) {
            return false;
        }
        String operation = getParameter(parameters, "operation", String.class, "read");
        if ("read".equals(operation) || "write".equals(operation) || "delete".equals(operation)) {
            return parameters.containsKey("path") && parameters.get("path") instanceof String;
        }
        return true;
    }

    @SuppressWarnings("unchecked")
    private <T> T getParameter(Map<String, Object> parameters, String key, Class<T> type) {
        Object value = parameters.get(key);
        if (type.isInstance(value)) {
            return (T) value;
        }
        return null;
    }

    @SuppressWarnings("unchecked")
    private <T> T getParameter(Map<String, Object> parameters, String key, Class<T> type, T defaultValue) {
        Object value = parameters.get(key);
        if (type.isInstance(value)) {
            return (T) value;
        }
        return defaultValue;
    }
}
