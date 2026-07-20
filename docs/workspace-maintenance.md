# Workspace Maintenance

Generated renders, diagnostics, temporary media, and local smoke-test folders do not belong in the source tree. Archive them in place with:

```bash
./scripts/workspace-archive.sh archive
```

The command moves known generated paths into `.workspace-archive/<timestamp>/payload/` and writes a tab-separated manifest. The archive directory is ignored by Git. Preview the operation with `--dry-run`, inspect snapshots with `list`, and restore a snapshot with:

```bash
./scripts/workspace-archive.sh restore --label <snapshot>
```

The canonical Tangying sloth assets under `ip-assets/main-ip/` are never part of the default archive target set. Only the versioned Blender master, runtime GLB export, texture-free studio, profile, manifest, voice reference, scripts, and integrity reports may be tracked there. Previews, renders, turnarounds, static backgrounds, and demo media belong in ignored `tmp/` or `outputs/` paths. Legacy tracked assets remain recoverable from Git history and the remote branch `archive/legacy-assets-2026-07`.
