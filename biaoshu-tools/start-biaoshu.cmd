@echo off
set PYTHONIOENCODING=utf-8
if "%OPENAI_BASE_URL%"=="" set OPENAI_BASE_URL=https://api.deepseek.com
if "%OPENAI_MODEL%"=="" set OPENAI_MODEL=deepseek-v4-pro
python app.py --port 9001
