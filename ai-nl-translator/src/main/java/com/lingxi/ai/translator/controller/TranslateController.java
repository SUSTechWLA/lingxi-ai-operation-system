package com.lingxi.ai.translator.controller;

import com.lingxi.ai.translator.model.DAG;
import com.lingxi.ai.translator.service.NlToDagService;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;

import java.util.HashMap;
import java.util.Map;

@RestController
@RequestMapping("/api")
public class TranslateController {

    @Autowired
    private NlToDagService nlToDagService;

    @ExceptionHandler(RuntimeException.class)
    public ResponseEntity<Map<String, Object>> handleRuntimeException(RuntimeException e) {
        Map<String, Object> error = new HashMap<>();
        error.put("error", e.getMessage());
        error.put("type", "ConfigurationError");
        if (e.getMessage().contains("API key")) {
            error.put("hint", "Please set OPENAI_API_KEY environment variable or update openai.api-key in application.yml");
            return ResponseEntity.status(503).body(error);
        }
        return ResponseEntity.status(500).body(error);
    }

    @PostMapping("/translate")
    public ResponseEntity<Map<String, Object>> translate(@RequestBody Map<String, String> request) {
        String prompt = request.get("prompt");
        DAG dag = nlToDagService.translateToDag(prompt);

        Map<String, Object> result = new HashMap<>();
        result.put("prompt", prompt);
        result.put("dag", dag);
        return ResponseEntity.ok(result);
    }

    @PostMapping("/translate-and-submit")
    public ResponseEntity<Map<String, Object>> translateAndSubmit(@RequestBody Map<String, String> request) {
        String prompt = request.get("prompt");
        Map<String, Object> result = nlToDagService.translateAndSubmit(prompt);
        return ResponseEntity.ok(result);
    }

    @GetMapping("/task/{taskId}")
    public ResponseEntity<Map<String, Object>> getTaskStatus(@PathVariable String taskId) {
        Map<String, Object> result = nlToDagService.getTaskStatus(taskId);
        return ResponseEntity.ok(result);
    }

    @GetMapping("/health")
    public ResponseEntity<Map<String, String>> health() {
        return ResponseEntity.ok(Map.of(
                "status", "UP",
                "service", "nl-translator"
        ));
    }
}
