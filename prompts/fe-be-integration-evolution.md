skill:
  name: fe-be-integration-evolution
  version: 1.1
  description: 自动完成前后端对接、实现最小可用功能并验证

context:
  assumptions:
    - 前端与后端在同一仓库
    - 前端为 Web（React/TypeScript + TailwindCSS + Zustand）
    - 后端为 API 服务（Go + Gin）
    - Vite proxy 转发 /api 请求到后端 8080 端口

loop: true
max_iterations: 10

steps:

  - id: scan_project
    name: 扫描项目结构
    action: analyze_repo
    output:
      - frontend_path
      - backend_path
      - api_style
      - tech_stack

  - id: detect_api
    name: 识别后端API
    action: analyze_backend_api
    input:
      backend_path: ${scan_project.backend_path}
    output:
      - endpoints
      - request_format
      - response_format

  - id: detect_frontend
    name: 识别前端调用逻辑
    action: analyze_frontend
    input:
      frontend_path: ${scan_project.frontend_path}
    output:
      - api_calls
      - pages
      - entry_points

  - id: choose_feature
    name: 选择最小功能
    action: decide_min_feature
    strategy:
      - 优先选择：
          - 内容发布（publishContent）
          - AI生成内容（aiGenerateContent）
          - AI润色文本（aiPolishText）
    output:
      - selected_api
      - selected_page

  - id: align_contract
    name: 对齐接口契约
    action: align_api_contract
    input:
      frontend_call: ${detect_frontend.api_calls}
      backend_api: ${detect_api.endpoints}
    output:
      - mismatch_fields
      - fix_plan
    rules:
      - 后端统一返回格式：{"code": int, "message": string, "data": object}
      - 前端统一解析 response.data.data

  - id: fix_backend
    name: 修复后端接口
    condition: mismatch_fields != empty
    action: modify_backend
    rules:
      - 保持接口简单
      - JSON格式统一
      - 返回标准结构：
          code: int
          message: string
          data: object
      - handler 添加 nil service 保护

  - id: fix_frontend
    name: 修复前端调用
    action: modify_frontend
    rules:
      - 使用统一请求方法（axios）
      - 确保URL正确（通过Vite proxy）
      - 处理loading/error
      - AI按钮传入type参数区分title/description

  - id: implement_flow
    name: 打通端到端流程
    action: implement_feature
    flow:
      - 用户输入标题/简介
      - 点击AI生成/润色 → 调用 /api/ai/generate 或 /api/ai/polish
      - 后端调用OpenAI API → 返回结果
      - 页面展示AI生成/润色后的内容
      - 用户选择平台 → 点击发布 → 调用 /api/publish
      - 后端创建DAG任务 → 返回taskId
      - 页面展示任务创建成功

  - id: run_backend
    name: 启动后端
    action: run_backend
    output:
      - backend_url

  - id: run_frontend
    name: 启动前端
    action: run_frontend
    output:
      - frontend_url

  - id: test_api
    name: API测试
    action: call_api
    input:
      endpoint: ${choose_feature.selected_api}
    output:
      - api_result

  - id: test_ui
    name: UI测试
    action: simulate_user
    steps:
      - 打开页面
      - 输入测试数据
      - 点击AI按钮
      - 点击发布按钮
      - 查看返回结果
    output:
      - ui_result

  - id: verify
    name: 验证结果
    action: evaluate
    rules:
      - API成功返回
      - 页面正确展示
    output:
      - success
      - issues

  - id: fix_loop
    name: 自动修复循环
    condition: success == false
    action: auto_fix
    strategy:
      - 优先修复接口字段
      - 修复URL错误
      - 修复跨域问题（后端已添加CORS中间件）
      - 修复JSON解析

  - id: output_result
    name: 输出结果
    action: summarize
    output:
      - 成功功能说明
      - 调用路径
      - API示例
      - 下一步优化建议

# 已完成的集成 (2026-04-25)

completed_integration:
  backend:
    - internal/publish/handler/handler.go - 3个API端点
    - internal/publish/service/service.go - 业务逻辑（OpenAI调用 + DAG提交）
    - cmd/lingxi-ai-os/main.go - 路由注册 + CORS中间件
  frontend:
    - src/services/api.ts - 统一API调用层（解析response.data.data）
    - src/utils/types.ts - 标准响应类型（ApiResponse<T>）
    - src/pages/PublishPage.tsx - 真实AI交互（生成/润色/发布）
    - src/components/TitleInput.tsx - AI润色传入type='title'
    - src/components/DescriptionInput.tsx - AI润色传入type='description'
    - src/components/AIHelperPanel.tsx - AI润色传入type='description'
  api_endpoints:
    POST /api/publish:
      request: {title, description, keywords, platforms[]}
      response: {code: 0, message: "success", data: {taskId, message}}
    POST /api/ai/generate:
      request: {prompt}
      response: {code: 0, message: "success", data: {title, description}}
    POST /api/ai/polish:
      request: {text, type: "title"|"description"}
      response: {code: 0, message: "success", data: {content}}
  tests:
    - internal/publish/handler/handler_test.go - 6个测试用例全部通过
    - 前端 npm run build 成功
    - Go go test ./... 全部通过
