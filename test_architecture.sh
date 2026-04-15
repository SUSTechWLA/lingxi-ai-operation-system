#!/bin/bash

echo "========================================"
echo " 灵犀AI OS 架构调整验证脚本"
echo "========================================"
echo ""

# 检查是否在正确的目录
if [ ! -d "ai-orchestrator" ] || [ ! -d "nl-translator" ]; then
    echo "错误：请在项目根目录运行此脚本"
    exit 1
fi

echo "✅ 项目结构验证通过"
echo ""
echo "模块目录："
echo "  - ai-orchestrator/   (任务编排层)"
echo "  - nl-translator/      (自然语言翻译层)"
echo ""

# 检查关键文件
echo "检查关键文件..."

check_file() {
    if [ -f "$1" ]; then
        echo "  ✅ $1"
    else
        echo "  ❌ $1 (缺失)"
    fi
}

echo ""
echo "ai-orchestrator 模块："
check_file "ai-orchestrator/src/main/java/com/lingxi/ai/orchestrator/controller/TaskController.java"
check_file "ai-orchestrator/src/main/java/com/lingxi/ai/orchestrator/service/OrchestratorService.java"

echo ""
echo "nl-translator 模块："
check_file "nl-translator/pom.xml"
check_file "nl-translator/src/main/java/com/lingxi/ai/translator/NlTranslatorApplication.java"
check_file "nl-translator/src/main/java/com/lingxi/ai/translator/controller/TranslateController.java"
check_file "nl-translator/src/main/java/com/lingxi/ai/translator/service/OpenAiClientService.java"
check_file "nl-translator/src/main/java/com/lingxi/ai/translator/service/NlToDagService.java"
check_file "nl-translator/src/main/resources/application.yml"

echo ""
echo "文档文件："
check_file "README.md"
check_file "ORCHESTRATOR_GUIDE.md"
check_file "NL_TRANSLATOR_GUIDE.md"

echo ""
echo "========================================"
echo " 架构调整完成！"
echo "========================================"
echo ""
echo "下一步操作："
echo ""
echo "1. 启动依赖服务："
echo "   cd ai-orchestrator"
echo "   docker-compose up -d redis redpanda"
echo ""
echo "2. 启动 Orchestrator："
echo "   cd ai-orchestrator"
echo "   mvn spring-boot:run"
echo ""
echo "3. 启动 NL Translator（需要 OpenAI API Key）："
echo "   cd nl-translator"
echo "   export OPENAI_API_KEY=your-api-key-here"
echo "   mvn spring-boot:run"
echo ""
echo "4. 测试 Orchestrator API："
echo "   curl -X POST http://localhost:8080/api/node \\"
echo "     -H 'Content-Type: application/json' \\"
echo "     -d '{\"nodes\":[{\"nodeId\":\"1\",\"type\":\"LLM\",\"task\":\"write_article\",\"deps\":[],\"status\":\"PENDING\"},{\"nodeId\":\"2\",\"type\":\"LLM\",\"task\":\"summarize\",\"deps\":[\"1\"],\"status\":\"PENDING\"}]}'"
echo ""
echo "5. 测试 NL Translator API："
echo "   curl -X POST http://localhost:8081/api/translate \\"
echo "     -H 'Content-Type: application/json' \\"
echo "     -d '{\"prompt\":\"写文章并生成摘要\"}'"
echo ""
