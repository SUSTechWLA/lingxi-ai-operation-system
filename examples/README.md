# 灵犀AI OS - 多语言工具示例

本目录包含灵犀AI OS Worker层的多语言工具示例。

## 文件说明

| 文件 | 说明 |
|------|------|
| `weather_tool.py` | 完整的天气查询工具示例（生产级） |
| `test_tool_final.py` | 简化的测试工具（用于验证注册流程） |
| `requirements.txt` | Python依赖 |

**注意**: 自动化测试脚本已移动到 `../scripts/final_test.sh`

## 快速开始

### 1. 安装依赖
```bash
pip install -r requirements.txt
```

### 2. 启动测试工具
```bash
# 方式1：启动简化测试工具（端口8092）
python test_tool_final.py

# 方式2：启动天气查询工具（端口8090）
python weather_tool.py
```

### 3. 启动Worker
```bash
cd ../ai-worker
mvn spring-boot:run
```

### 4. 运行测试脚本
```bash
../scripts/final_test.sh
```

## 工具接口规范

所有工具必须实现以下3个HTTP接口：

| 接口 | 方法 | 说明 |
|------|------|------|
| `/info` | GET | 获取工具元数据 |
| `/run` | POST | 执行工具 |
| `/health` | GET | 健康检查 |

### 响应格式

```json
{
  "code": 200,
  "message": "success",
  "data": {}
}
```
