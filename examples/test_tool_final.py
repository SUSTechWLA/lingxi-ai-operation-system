"""
灵犀AI OS - 最终测试工具
使用统一接口：/info, /run, /health
"""

from fastapi import FastAPI
from pydantic import BaseModel
from typing import Dict, Any
import uvicorn
import time

app = FastAPI(title="Final Test Tool", version="1.0.0")

# 工具元数据（直接返回，不包装）
TOOL_META = {
    "toolId": "final_test_tool",
    "toolName": "最终测试工具",
    "toolVersion": "1.0.0",
    "description": "用于验证Worker注册和执行流程的最终测试工具",
    "type": "CORE",
    "author": "lingxi-ai",
    "tags": ["测试", "工具"],
    "inputSchema": {
        "type": "object",
        "properties": {
            "action": {"type": "string", "description": "操作类型：echo/add/multiply"},
            "message": {"type": "string", "description": "echo消息"},
            "a": {"type": "number", "description": "操作数a"},
            "b": {"type": "number", "description": "操作数b"}
        },
        "required": ["action"]
    },
    "outputSchema": {
        "type": "object",
        "properties": {
            "result": {"type": "any", "description": "执行结果"},
            "timestamp": {"type": "string", "description": "执行时间"}
        }
    },
    "timeout": 5000,
    "maxRetry": 3
}

class ExecuteRequest(BaseModel):
    taskId: str
    nodeId: str
    input: Dict[str, Any]

@app.get("/info")
async def get_info():
    """获取工具元数据（直接返回）"""
    return TOOL_META

@app.post("/run")
async def run(request: ExecuteRequest):
    """执行工具"""
    try:
        action = request.input.get("action")

        if action == "echo":
            result = {"echo": request.input.get("message", "Hello from final test!")}
        elif action == "add":
            a = request.input.get("a", 0)
            b = request.input.get("b", 0)
            result = {"sum": a + b, "a": a, "b": b}
        elif action == "multiply":
            a = request.input.get("a", 1)
            b = request.input.get("b", 1)
            result = {"product": a * b, "a": a, "b": b}
        else:
            return {
                "code": 400,
                "message": "未知操作: " + str(action),
                "data": None
            }

        return {
            "code": 200,
            "message": "success",
            "data": {
                "result": result,
                "timestamp": time.strftime("%Y-%m-%d %H:%M:%S"),
                "taskId": request.taskId,
                "nodeId": request.nodeId
            }
        }
    except Exception as e:
        return {
            "code": 500,
            "message": str(e),
            "data": None
        }

@app.get("/health")
async def health():
    """健康检查"""
    return {
        "status": "UP",
        "version": "1.0.0",
        "timestamp": int(time.time() * 1000)
    }

if __name__ == "__main__":
    print("=" * 60)
    print("灵犀AI OS - 最终测试工具")
    print("=" * 60)
    print("\n标准接口：")
    print("  GET  /info   - 获取工具元数据")
    print("  POST /run    - 执行工具")
    print("  GET  /health - 健康检查")
    print("\n启动服务在端口 8092...")
    uvicorn.run(app, host="0.0.0.0", port=8092)
