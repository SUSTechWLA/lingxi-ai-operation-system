"""
biaoshu-writer 工具 HTTP 服务
将 Python 脚本包装为 HTTP 端点，供 cloud-backend 外部工具调用。

启动: python app.py --port 9001
"""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
import tempfile
import traceback
from pathlib import Path

from flask import Flask, request, jsonify

# ─── 配置 ───────────────────────────────────────────
SCRIPTS_DIR = Path(
    os.environ.get(
        "BIAOSHU_SCRIPTS_DIR",
        str(Path.home() / ".agents/skills/biaoshu-writer-pro-5.4.0/scripts"),
    )
)
OUTPUT_DIR = Path(os.environ.get("BIAOSHU_OUTPUT_DIR", str(Path(__file__).parent / "output")))
ALLOWED_BID_EXTENSIONS = {".txt", ".docx", ".pdf", ".xlsx", ".xls"}

app = Flask(__name__)


# ─── 辅助函数 ─────────────────────────────────────────

def run_script(script_name: str, args: list[str], timeout: int = 120) -> dict:
    """运行 Python 脚本并返回 JSON 结果"""
    script_path = SCRIPTS_DIR / script_name
    if not script_path.exists():
        return {"success": False, "error": f"脚本不存在: {script_path}"}

    try:
        result = subprocess.run(
            ["python", str(script_path)] + args,
            capture_output=True,
            text=True,
            timeout=timeout,
            cwd=str(SCRIPTS_DIR),
        )
        stdout = result.stdout.strip()
        stderr = result.stderr.strip()

        if result.returncode != 0:
            return {"success": False, "error": stderr or stdout or f"exit code {result.returncode}"}

        return {"success": True, "data": {"stdout": stdout, "stderr": stderr}}
    except subprocess.TimeoutExpired:
        return {"success": False, "error": f"脚本执行超时 ({timeout}s)"}
    except Exception as e:
        return {"success": False, "error": str(e)}


def extract_payload() -> dict:
    """从请求体中提取 params"""
    body = request.get_json(force=True) or {}
    return body.get("params", body)


def require_param(params: dict, key: str) -> str | None:
    """缺少必填参数时返回错误消息，否则返回 None"""
    if key not in params or not params[key]:
        return jsonify({"success": False, "error": f"缺少必填参数: {key}"}), 400
    return None


# ─── 健康检查 ─────────────────────────────────────────

@app.route("/health", methods=["GET"])
def health():
    scripts = {}
    for name in [
        "parse_bid_files.py",
        "check_chapter_words.py",
        "convert_to_word.py",
        "merge_chapters.py",
        "retrieve.py",
        "chapter_chunker.py",
        "embed_chunks.py",
    ]:
        scripts[name.replace(".py", "")] = (SCRIPTS_DIR / name).exists()
    return jsonify({
        "success": True,
        "data": {
            "status": "UP",
            "scripts_dir": str(SCRIPTS_DIR),
            "output_dir": str(OUTPUT_DIR),
            "scripts": scripts,
        },
    })


# ─── 工具端点 ─────────────────────────────────────────

@app.route("/tools/parse_bid_files", methods=["POST"])
def tool_parse_bid_files():
    """解析招标文件（txt/pdf/docx/xlsx），输出产物到项目目录"""
    params = extract_payload()
    err = require_param(params, "file_path")
    if err:
        return err
    file_path = params["file_path"]
    if not os.path.exists(file_path):
        return jsonify({"success": False, "error": f"文件不存在: {file_path}"}), 400
    ext = os.path.splitext(file_path)[1].lower()
    if ext not in ALLOWED_BID_EXTENSIONS:
        return jsonify({
            "success": False,
            "error": (
                f"Unsupported file format: {ext or '(none)'}. "
                "Supported formats: .txt, .docx, .pdf, .xlsx, .xls"
            ),
        }), 400

    result = run_script("parse_bid_files.py", [file_path], timeout=120)
    stdout = str(result.get("data", {}).get("stdout", "")).strip()
    if result.get("success") and (stdout.startswith("[错误]") or stdout.startswith("[Error]")):
        return jsonify({"success": False, "error": stdout}), 400

    # 保存阶段产物：00_招标文件解析报告.md
    if result.get("success") and params.get("output_dir"):
        project_dir = os.path.join(OUTPUT_DIR, str(params["output_dir"]))
        os.makedirs(project_dir, exist_ok=True)
        report_path = os.path.join(project_dir, "00_招标文件解析报告.md")
        with open(report_path, "w", encoding="utf-8") as f:
            f.write(f"# 招标文件解析报告\n\n")
            f.write(f"**源文件**: {file_path}\n\n")
            f.write(f"---\n\n")
            f.write(result["data"]["stdout"])
        result["data"]["report_path"] = report_path

    return jsonify(result)


@app.route("/tools/check_chapter_words", methods=["POST"])
def tool_check_chapter_words():
    """章节字数检查"""
    params = extract_payload()

    # 支持两种模式：传入 markdown 内容或章节目录路径
    if "content" in params:
        # 写入临时文件
        with tempfile.NamedTemporaryFile(
            mode="w", suffix=".md", encoding="utf-8", delete=False
        ) as f:
            f.write(params["content"])
            tmp_path = f.name
        try:
            result = run_script("check_chapter_words.py", [tmp_path], timeout=30)
        finally:
            os.unlink(tmp_path)
    elif "file_path" in params:
        result = run_script("check_chapter_words.py", [params["file_path"]], timeout=30)
    else:
        return jsonify({"success": False, "error": "需要 content 或 file_path 参数"}), 400

    return jsonify(result)


@app.route("/tools/check_all_chapters_words", methods=["POST"])
def tool_check_all_chapters_words():
    """检查章节目录下所有文件的字数"""
    params = extract_payload()
    err = require_param(params, "chapter_dir")
    if err:
        return err

    chapter_dir = params["chapter_dir"]
    score_map = params.get("score_map", {})  # {"第一章": 5, "第二章": 4, ...}
    total_pages = params.get("total_pages", 300)
    total_score = params.get("total_score", 100)

    if not os.path.isdir(chapter_dir):
        return jsonify({"success": False, "error": f"目录不存在: {chapter_dir}"}), 400

    import re

    files = sorted(
        [f for f in os.listdir(chapter_dir) if f.endswith(".md")],
        key=lambda x: int(x.split("_")[0]),
    )

    def count_chinese_chars(text):
        chinese = re.findall(r"[\u4e00-\u9fff]", text)
        return len(chinese)

    results = []
    overall_pass = True
    for fname in files:
        fpath = os.path.join(chapter_dir, fname)
        with open(fpath, encoding="utf-8") as f:
            content = f.read()
        word_count = count_chinese_chars(content)

        # 计算目标字数
        chapter_name = Path(fname).stem
        score = score_map.get(chapter_name, 5) if score_map else 5
        target = int(score * (total_pages / total_score) * 780)
        low = int(target * 0.75)
        high = int(target * 1.25)
        passed = low <= word_count <= high

        if not passed:
            overall_pass = False

        results.append({
            "file": fname,
            "word_count": word_count,
            "target_words": target,
            "range": f"{low:,} ~ {high:,}",
            "passed": passed,
        })

    return jsonify({
        "success": True,
        "data": {
            "overall_pass": overall_pass,
            "chapters": results,
        },
    })


@app.route("/tools/convert_to_word", methods=["POST"])
def tool_convert_to_word():
    """Markdown → Word"""
    params = extract_payload()

    # 支持 content 字符串模式
    if "content" in params:
        md_content = params["content"]
        tmp_path = None
        try:
            with tempfile.NamedTemporaryFile(
                mode="w", suffix=".md", encoding="utf-8", delete=False
            ) as f:
                f.write(md_content)
                tmp_path = f.name
            project_name = params.get("project_name", "技术标")
            output_name = f"{project_name}_技术标.docx"
            output_path = str(OUTPUT_DIR / project_name / output_name)
            os.makedirs(os.path.dirname(output_path), exist_ok=True)

            result = run_script("convert_to_word.py", [tmp_path, output_path], timeout=60)
            if result["success"]:
                result["data"]["output_path"] = output_path
                result["data"]["output_name"] = output_name
        finally:
            if tmp_path:
                os.unlink(tmp_path)
    elif "input_file" in params:
        input_file = params["input_file"]
        output_file = params.get("output_file", input_file.replace(".md", ".docx"))
        os.makedirs(os.path.dirname(output_file), exist_ok=True)
        result = run_script("convert_to_word.py", [input_file, output_file], timeout=60)
        if result["success"]:
            result["data"]["output_path"] = output_file
    else:
        return jsonify({"success": False, "error": "需要 content 或 input_file 参数"}), 400

    return jsonify(result)


@app.route("/tools/merge_chapters", methods=["POST"])
def tool_merge_chapters():
    """合并章节目录下所有 md 文件"""
    params = extract_payload()
    err = require_param(params, "chapter_dir")
    if err:
        return err

    chapter_dir = params["chapter_dir"]
    output = params.get("output_file", os.path.join(chapter_dir, "..", "merged.md"))
    output = os.path.abspath(output)

    result = run_script("merge_chapters.py", [chapter_dir, output], timeout=30)
    if result["success"]:
        result["data"]["output_path"] = output
    return jsonify(result)


@app.route("/tools/rag_retrieve", methods=["POST"])
def tool_rag_retrieve():
    """知识库 RAG 检索"""
    params = extract_payload()
    err = require_param(params, "query")
    if err:
        return err

    query = params["query"]
    k = params.get("k", 5)
    config = params.get("config", f"{SCRIPTS_DIR}/../embedding_config.json")
    persist_dir = params.get("persist_dir", f"{SCRIPTS_DIR}/../vector_db/biaoshu")
    collection = params.get("collection", "biaoshu_chunks")

    result = run_script(
        "retrieve.py",
        [
            query,
            "--config", config,
            "--persist-dir", persist_dir,
            "--collection", collection,
            "-k", str(k),
        ],
        timeout=60,
    )
    return jsonify(result)


@app.route("/tools/chapter_chunker", methods=["POST"])
def tool_chapter_chunker():
    """将 Markdown 技术标按章节切分为 RAG 切片"""
    params = extract_payload()
    err = require_param(params, "input_file")
    if err:
        return err

    input_file = params["input_file"]
    output = params.get("output_file", input_file.replace(".md", ".jsonl"))
    chunk_size = params.get("chunk_size", 1500)
    chunk_overlap = params.get("chunk_overlap", 200)

    result = run_script(
        "chapter_chunker.py",
        [
            input_file,
            "--output", output,
            "--chunk-size", str(chunk_size),
            "--chunk-overlap", str(chunk_overlap),
        ],
        timeout=60,
    )
    if result["success"]:
        result["data"]["output_path"] = output
    return jsonify(result)


@app.route("/tools/embed_chunks", methods=["POST"])
def tool_embed_chunks():
    """将 RAG 切片写入 Chroma 向量库"""
    params = extract_payload()
    err = require_param(params, "input_file")
    if err:
        return err

    input_file = params["input_file"]
    config = params.get("config", f"{SCRIPTS_DIR}/../embedding_config.json")
    persist_dir = params.get("persist_dir", f"{SCRIPTS_DIR}/../vector_db/biaoshu")
    collection = params.get("collection", "biaoshu_chunks")

    result = run_script(
        "embed_chunks.py",
        [
            input_file,
            "--config", config,
            "--persist-dir", persist_dir,
            "--collection", collection,
        ],
        timeout=120,
    )
    return jsonify(result)


# ─── 主入口 ───────────────────────────────────────────

if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="biaoshu-writer 工具 HTTP 服务")
    parser.add_argument("--port", type=int, default=9001, help="监听端口")
    parser.add_argument("--host", default="127.0.0.1", help="监听地址")
    parser.add_argument("--debug", action="store_true", help="调试模式")
    args = parser.parse_args()

    print(f"biaoshu-writer 工具服务启动: http://{args.host}:{args.port}")
    print(f"  脚本目录: {SCRIPTS_DIR}")
    print(f"  输出目录: {OUTPUT_DIR}")
    print(f"  健康检查: http://{args.host}:{args.port}/health")
    app.run(host=args.host, port=args.port, debug=args.debug)
