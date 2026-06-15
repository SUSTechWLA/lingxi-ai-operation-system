#!/usr/bin/env python3
from pathlib import Path
import argparse
p=argparse.ArgumentParser()
p.add_argument('--id',required=True)
p.add_argument('--root',default='.lingxi-core-loop')
a=p.parse_args()
base=Path(a.root)/a.id
base.mkdir(parents=True,exist_ok=False)
for d in ['adr','reviews','bugs','evidence']:
    (base/d).mkdir()
content='schema_version: "1.0"\nloop_id: '+a.id+'\nmode: solution-design\nstatus: CREATED\ncore_write_enabled: false\nreview_mode: EXTERNAL_MANUAL\nsolution_outputs:\n  solution_blueprint: false\n  frontend_requirements: false\n  workflow_spec: false\n  api_and_event_contracts: false\n  external_capability_requirements: false\n'
(base/'loop.yaml').write_text(content,encoding='utf-8')
print(base)
