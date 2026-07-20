# Default IP A-roll Assets

The repository ships one canonical sloth character source and one canonical warm-studio source under `ip-assets/main-ip/`.

## Canonical inventory

| Role | Path | Policy |
|---|---|---|
| Character source of truth | `models/main-ip-aroll-master-20260720.blend` | The only formal character master. |
| Runtime compatibility export | `models/main-ip-rigged.glb` | Derived export; never edited as a second source. |
| Studio source of truth | `scenes/warm-sloth-studio-20260720.blend` | Character-free, texture-free, and demo-audio-free. |
| Runtime profile | `character-profile.json` | Default MCP entry point. |
| Integrity manifest | `manifests/default-aroll-assets.json` | Pins the character and studio hashes. |
| Voice reference | `voice/reference/main_ip_voice_ref_v1.wav` | Pinned local production-voice reference. |
| Audit evidence | `reports/default-aroll-character-audit.json`, `reports/default-aroll-studio-audit.json` | Blender-side structural audits. |

The character master packs its four PBR images. The studio has zero image and zero sound dependencies. It keeps its architecture, furniture, native geometry, cameras, lights, spawn markers, and built-in-font brand copy without a PNG logo plane.

## Repository policy

- Every tracked path must use English ASCII names.
- Do not track static backgrounds, texture sidecars, turnarounds, previews, smoke renders, videos, temporary audio, Blender backups, or duplicate masters.
- Write generated evidence to ignored `tmp/` or `outputs/` directories.
- Do not edit the runtime GLB as an independent character version.
- Any canonical `.blend` change must update its audit report and manifest hash in the same pull request.
- Preserve the existing Armature, Shape Keys, UVs, vertex ordering, drivers, materials, and actions unless a separately approved asset-refinement task explicitly changes them.

## Validation

```bash
bash scripts/release-tree-check.sh
python3 -m unittest mcp.ip_avatar_3d.test_default_aroll_assets
python3 -m unittest mcp.ip_avatar_3d.test_warm_studio_contract
```

For Blender dependency checks, run `ip-assets/main-ip/scripts/audit_blend_dependencies.py` against both canonical `.blend` files. A release studio must report zero dependencies. Character image dependencies must be packed and must not appear in `missingExternal`.

Removed legacy assets remain recoverable from Git history and `archive/legacy-assets-2026-07`; they are intentionally absent from the release tree.
