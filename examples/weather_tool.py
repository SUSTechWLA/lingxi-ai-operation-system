"""
灵犀AI OS - 多语言工具示例：天气查询工具（Python）
这是一个完整的多语言工具实现，遵循标准的HTTP接口协议
"""

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
from typing import Optional, Dict, Any, List
import uvicorn
import time

# ==================== 工具配置 ====================
app = FastAPI(title="Weather Query Tool", version="1.0.0")

# 工具元数据 - 完整定义，支持向量检索和LLM理解
TOOL_META = {
    "toolName": "weather_query",
    "toolVersion": "1.0.0",
    "description": "查询指定城市的实时天气信息，支持温度、湿度、风力等数据。"
                   "可以查询中国大陆主要城市的天气情况。",
    "author": "lingxi-ai",
    "tags": ["天气", "查询", "工具", "生活服务", "气象"],
    "inputSchema": {
        "type": "object",
        "properties": {
            "city": {
                "type": "string",
                "description": "要查询的城市名称，如'北京'、'上海'、'广州'、'深圳'"
            },
            "type": {
                "type": "string",
                "description": "查询类型，可选值：realtime(实时天气)/forecast(天气预报)",
                "default": "realtime"
            }
        },
        "required": ["city"]
    },
    "outputSchema": {
        "type": "object",
        "properties": {
            "city": {
                "type": "string",
                "description": "城市名称"
            },
            "temperature": {
                "type": "number",
                "description": "实时温度，单位摄氏度"
            },
            "humidity": {
                "type": "number",
                "description": "相对湿度，百分比"
            },
            "windSpeed": {
                "type": "number",
                "description": "风速，单位公里/小时"
            },
            "weather": {
                "type": "string",
                "description": "天气状况描述，如'晴'、'多云'、'阴'、'小雨'"
            },
            "queryTime": {
                "type": "string",
                "description": "查询时间"
            }
        }
    },
    "examples": [
        {
            "input": {"city": "北京"},
            "output": {
                "city": "北京",
                "temperature": 25,
                "humidity": 60,
                "windSpeed": 12,
                "weather": "晴",
                "queryTime": "2024-06-17 14:30:00"
            }
        },
        {
            "input": {"city": "上海", "type": "realtime"},
            "output": {
                "city": "上海",
                "temperature": 28,
                "humidity": 75,
                "windSpeed": 8,
                "weather": "多云",
                "queryTime": "2024-06-17 14:30:00"
            }
        }
    ],
    "timeout": 5000,
    "maxRetry": 3
}

# 模拟天气数据库
WEATHER_DATABASE = {
    "北京": {"temperature": 25, "humidity": 60, "windSpeed": 12, "weather": "晴"},
    "上海": {"temperature": 28, "humidity": 75, "windSpeed": 8, "weather": "多云"},
    "广州": {"temperature": 32, "humidity": 85, "windSpeed": 5, "weather": "小雨"},
    "深圳": {"temperature": 31, "humidity": 80, "windSpeed": 10, "weather": "阴"},
    "杭州": {"temperature": 27, "humidity": 70, "windSpeed": 6, "weather": "多云"},
    "成都": {"temperature": 24, "humidity": 65, "windSpeed": 4, "weather": "阴"},
    "武汉": {"temperature": 29, "humidity": 72, "windSpeed": 7, "weather": "晴"}
}

# ==================== 请求/响应模型 ====================
class ExecuteRequest(BaseModel):
    nodeId: str
    taskId: str
    traceId: str
    input: Dict[str, Any]

class StandardResponse(BaseModel):
    code: int
    message: str
    data: Optional[Any] = None

# ==================== 工具标准接口 ====================

@app.get("/tool/info", response_model=StandardResponse)
async def get_tool_info():
    """
    获取工具完整元数据（注册用）
    对应文档中的 /tool/info 接口
    """
    return StandardResponse(
        code=200,
        message="success",
        data=TOOL_META
    )

@app.post("/tool/execute", response_model=StandardResponse)
async def execute_tool(request: ExecuteRequest):
    """
    执行工具，接收输入参数，返回执行结果
    对应文档中的 /tool/execute 接口
    """
    try:
        tool_input = request.input
        city = tool_input.get("city")
        query_type = tool_input.get("type", "realtime")

        if not city:
            return StandardResponse(
                code=400,
                message="缺少必需参数：city",
                data={"errorCode": "MISSING_PARAMETER", "errorDetail": "city参数不能为空"}
            )

        # 查询天气数据
        weather_data = WEATHER_DATABASE.get(city)
        if not weather_data:
            return StandardResponse(
                code=404,
                message=f"找不到城市'{city}'的天气数据",
                data={"errorCode": "CITY_NOT_FOUND", "errorDetail": f"城市'{city}'不在支持列表中"}
            )

        # 构建结果
        result = {
            "city": city,
            "temperature": weather_data["temperature"],
            "humidity": weather_data["humidity"],
            "windSpeed": weather_data["windSpeed"],
            "weather": weather_data["weather"],
            "queryTime": time.strftime("%Y-%m-%d %H:%M:%S"),
            "type": query_type
        }

        return StandardResponse(
            code=200,
            message="success",
            data=result
        )

    except Exception as e:
        return StandardResponse(
            code=500,
            message=f"工具执行失败：{str(e)}",
            data={"errorCode": "EXECUTE_FAILED", "errorDetail": str(e)}
        )

@app.get("/tool/health", response_model=StandardResponse)
async def health_check():
    """
    健康检查，返回工具运行状态
    对应文档中的 /tool/health 接口
    """
    return StandardResponse(
        code=200,
        message="success",
        data={
            "status": "UP",
            "version": TOOL_META["toolVersion"],
            "timestamp": int(time.time() * 1000)
        }
    )

# ==================== 服务启动 ====================
if __name__ == "__main__":
    print("=" * 60)
    print("灵犀AI OS - 天气查询工具（Python示例）")
    print(f"工具名称：{TOOL_META['toolName']}")
    print(f"版本：{TOOL_META['toolVersion']}")
    print("=" * 60)
    print("\n可用接口：")
    print("  GET  /tool/info    - 获取工具元数据")
    print("  POST /tool/execute - 执行工具")
    print("  GET  /tool/health  - 健康检查")
    print("\n支持的城市：北京、上海、广州、深圳、杭州、成都、武汉")
    print("\n启动服务...")

    uvicorn.run(app, host="0.0.0.0", port=8090)
