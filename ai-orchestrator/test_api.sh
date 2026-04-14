#!/bin/bash

echo "测试创建任务API..."
curl -v -X POST http://localhost:8080/api/task \
  -H "Content-Type: application/json" \
  -d '{"prompt":"写文章并生成摘要"}'

echo "\n测试查询任务API..."
curl -v http://localhost:8080/api/task/test-task-id