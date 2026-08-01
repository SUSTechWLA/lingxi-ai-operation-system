"""Discard only unsaved Gate 1 partial state by reopening the authoritative source.

This script does not write or overwrite any Blender file. The pre-Gate checkpoint
already exists on disk. It simply reloads the unchanged authoritative source so
the corrected Gate 1 script can run from a clean state.
"""

from pathlib import Path

import bpy


SOURCE = Path("/Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip/models/checkpoints/main-ip-aroll-master_20260718_114057_eye_redesign_final.blend")

if not SOURCE.exists():
    raise FileNotFoundError(SOURCE)

bpy.ops.wm.open_mainfile(filepath=str(SOURCE), load_ui=True)
