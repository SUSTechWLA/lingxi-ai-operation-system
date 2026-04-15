package com.lingxi.ai.translator.controller;

import com.lingxi.ai.translator.model.DAG;
import com.lingxi.ai.translator.service.NlToDagService;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;

import java.util.HashMap;
import java.util.Map;

@RestController
@RequestMapping("/api")
public class TranslateController {

    @Autowired
    private NlToDagService nlToDagService;

    @Value("${orchestrator.url}")
    private String orchestratorUrl;

    // 仅翻译，返回 DAG
    @PostMapping("/translate")
    public ResponseEntity<Map<String, Object>> translate(@RequestBody Map<String, String> request) {
        String prompt = request.get("prompt");
        DAG dag = nlToDagService.translateToDag(prompt);

        Map<String, Object> result = new HashMap<>();
        result.put("prompt", prompt);
        result.put("dag", dag);
        return ResponseEntity.ok(result);
    }

    // 翻译并提交给 orchestrator
    @PostMapping("/translate-and-submit")
    public ResponseEntity<Map<String, Object>> translateAndSubmit(@RequestBody Map<String, String> request) {
        String prompt = request.get("prompt");
        Map<String, Object> result = nlToDagService.translateAndSubmit(prompt, orchestratorUrl);
        return ResponseEntity.ok(result);
    }
}
