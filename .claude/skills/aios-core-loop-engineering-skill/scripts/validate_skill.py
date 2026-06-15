#!/usr/bin/env python3
from pathlib import Path
import json,sys
root=Path(__file__).resolve().parents[1]
required=['SKILL.md','README.md','templates/loop.template.yaml','schemas/loop.schema.json']
missing=[x for x in required if not (root/x).exists()]
for p in (root/'schemas').glob('*.json'):
    json.loads(p.read_text(encoding='utf-8'))
if missing:
    print('missing:',missing);sys.exit(1)
print('skill validation passed')
