# Task 11 Report: Final A-roll Master Preparation

## Outcome

Task 11 prepared and published the main-IP A-roll master through the production
`server.prepare_character_master` entrypoint with `qualityTier=aroll_close`.
The build ran from the profile's source FBX into a temporary staging directory;
the three canonical outputs were copied to same-filesystem temporary names,
byte-compared, and renamed only after Blend, GLB, QA, and focused Blender
validation passed.

The saved master now contains deterministic standalone `Camera_Medium` and
`Camera_Wide` objects required by Task 10 QA. They remain outside
`IP_Character_Master`, so appending the reusable character collection does not
also append or conflict with runtime studio cameras.

No production voice, substitute voice, final 2K reel, or placeholder production
artifact was generated.

## Production Build

Inputs:

- source FBX: `ip形象/main_ip/turnaround/带骨骼3d模型.fbx`;
- profile: `ip形象/main_ip/character-profile.json`;
- quality tier: `aroll_close`;
- staging directory: `tmp/ip_avatar_3d/task11-staging-final.RttcAZ`.

The canonical builder command completed with `status=ready`, `success=true`,
and all three staged artifacts present:

```bash
python3 -c "import json, sys; sys.path.insert(0, 'mcp/ip_avatar_3d'); import server; result = server.prepare_character_master(sourceModel='ip形象/main_ip/turnaround/带骨骼3d模型.fbx', characterProfilePath='ip形象/main_ip/character-profile.json', outputDir='tmp/ip_avatar_3d/task11-staging-final.RttcAZ', qualityTier='aroll_close', dryRun=False); print('TASK11_BUILD_RESULT=' + json.dumps(result, ensure_ascii=False, sort_keys=True))"
```

Publication refused to run if any canonical destination already existed. It
then used `install`, `cmp -s`, and same-directory `mv` operations to publish:

- `outputs/MainIP_Sloth_Aroll_Master.blend`;
- `outputs/MainIP_Sloth_Aroll_Rigged.glb`;
- `outputs/MainIP_Sloth_Aroll_Rig_Report.json`.

## Master Validation

The production rig report and direct saved-Blend inspection agree on:

- exactly one scene/master Armature named `Armature` with 52 bones;
- all 44 required semantic roles resolved, including all six middle-finger
  roles;
- 18 required finger bones, all marked deform, with three segments per digit;
- 24 hand joint support loops;
- zero unweighted skinned vertices and at most four deform influences;
- all 16 canonical `Aroll_*` Actions;
- all profile-required mouth Shape Keys plus `Eye_Squint.L/R`;
- master collection `IP_Character_Master` with `ip_aroll_master_version=1`;
- standalone `Camera_Medium` and `Camera_Wide`, with no cameras inside the
  reusable master collection.

The direct Blend audit counted 7,818 vertices across six skinned meshes,
`unweightedVertexCount=0`, and `maxVertexInfluences=4`. It validated the saved
collection and the full Task 10 scene contract, rather than accepting report
metadata alone.

Facial capability is reported without escalation: `trueLipTopology=true` is
backed by the integrated source-mouth seam, while `trueEyelidTopology=false`,
`sourceEyelidShapeKeys=false`, `sourceSquintShapeKeys=true`, and
`blinkCapability=squint_only`. No full blink is claimed.

## GLB Reimport

The canonical GLB was reimported into a Blender 5.1.2 factory-startup scene
with:

```bash
/Applications/Blender.app/Contents/MacOS/Blender -b --factory-startup --python-expr "import bpy,json; bpy.ops.import_scene.gltf(filepath='outputs/MainIP_Sloth_Aroll_Rigged.glb'); report=json.load(open('outputs/MainIP_Sloth_Aroll_Rig_Report.json')); armatures=[obj for obj in bpy.context.scene.objects if obj.type=='ARMATURE']; assert len(armatures)==1; armature=armatures[0]; bones={bone.name for bone in armature.data.bones}; expected=set(report['boneMap'].values()); fingers={name for role,name in report['boneMap'].items() if role.startswith('finger_')}; shapes={key.name for obj in bpy.context.scene.objects if obj.type=='MESH' and obj.data.shape_keys for key in obj.data.shape_keys.key_blocks}; actions={action.name for action in bpy.data.actions}; aroll={name for name in report['actions'] if name.startswith('Aroll_')}; skinned=sum(obj.type=='MESH' and any(mod.type=='ARMATURE' and mod.object==armature for mod in obj.modifiers) for obj in bpy.context.scene.objects); assert expected<=bones and len(fingers)==18 and fingers<=bones and set(report['mouthShapeKeys'])<=shapes and len(aroll)==16 and aroll<=actions and skinned>0; print('TASK11_GLB_VALIDATION='+json.dumps({'armatures':len(armatures),'bones':len(bones),'fingerBones':len(fingers&bones),'shapeKeys':len(shapes),'actions':len(actions),'arollActions':len(aroll&actions),'skinnedMeshes':skinned},sort_keys=True))"
```

It printed:

```text
TASK11_GLB_VALIDATION={"actions": 48, "armatures": 1, "arollActions": 16, "bones": 52, "fingerBones": 18, "shapeKeys": 26, "skinnedMeshes": 6}
```

All semantic bones and required Shape Keys are included in the assertions. The
staged file was also byte-compared with the published canonical GLB before the
atomic rename.

## Low-resolution QA

The final command ran against the canonical published Blend:

```bash
/Applications/Blender.app/Contents/MacOS/Blender -b outputs/MainIP_Sloth_Aroll_Master.blend --python mcp/ip_avatar_3d/render_aroll_master_qa.py -- tmp/ip_avatar_3d/aroll_master_qa
```

`tmp/ip_avatar_3d/aroll_master_qa/qa-report.json` reports `status=ready` for all
52 required files: six close hand poses, six per-digit L/R roll samples, eight
face crops, and medium/wide samples for all 16 A-roll Actions. Each sample
contains the action, camera, relative path, bone rotations, fingertip
displacement ratios, framing, silhouette metrics, and frame MD5.

Acceptance metrics:

- open/fist alpha-mask difference: `0.016211` (minimum `0.012`);
- right finger-roll phases: `0.007681`, `0.016797`;
- left finger-roll phases: `0.006604`, `0.013472`;
- silhouette coverage range: `0.055855` to `0.313003`;
- adjacent framemd5 groups checked: hand 6, right roll 3, left roll 3,
  face 8, actions 32;
- face sample: `face/squint.png`, `blinkCapability=squint_only`,
  `fullBlinkClaimed=false`.

## Tests

The camera regression was first run before implementation and failed on the
intended missing `Camera_Medium` assertion. After implementation, the focused
hand/QA command passed all 10 direct-runner checks:

```bash
/Applications/Blender.app/Contents/MacOS/Blender -b --python mcp/ip_avatar_3d/test_blender_hand_refinement.py
```

The broader focused character-rig command passed all 24 direct-runner checks:

```bash
/Applications/Blender.app/Contents/MacOS/Blender -b --python mcp/ip_avatar_3d/test_blender_character_rig.py
```

This includes source-rig preservation, hand topology and weighting, source-mesh
facial retopology, A-roll animation, squint-only truth, and GLB round-trip
coverage. Blender emitted only existing `Material.use_nodes` Blender 6.0
deprecation warnings.

## Published SHA256

| Artifact | Bytes | SHA256 |
| --- | ---: | --- |
| `outputs/MainIP_Sloth_Aroll_Master.blend` | 25,345,859 | `766471fa955ac71800635025c0d68d7f347cb968d7fff444d76fe15c37611b07` |
| `outputs/MainIP_Sloth_Aroll_Rigged.glb` | 32,263,768 | `02fb467c879d9ed48afb6861cf0d68c29a59d947ea13f409d211334a62a5f8e3` |
| `outputs/MainIP_Sloth_Aroll_Rig_Report.json` | 22,247 | `99d8a1a9615544cb74265d48d1fb71e67db5e3155fef24d51aedf4f6af530de8` |
| `tmp/ip_avatar_3d/aroll_master_qa/qa-report.json` | 238,082 | `c97c2b4626e948d7920aa15fe067270832dfb114de30e182e04a3cd377bd475a` |

Generated Blend, GLB, rig-report, and low-resolution QA evidence remain
untracked and are not part of the code commit.

## Deferred Work

Production voice integration is owned by the separate voice task and was not
read, edited, substituted, or consumed here. Final 2K action, hand, and face
reels remain intentionally deferred until that provider voice is ready. The
real low-resolution QA evidence above is the only rendered Task 11 evidence;
no fake production reel was created.
