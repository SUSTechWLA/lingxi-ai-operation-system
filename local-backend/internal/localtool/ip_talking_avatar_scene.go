package localtool

import "path/filepath"

type AvatarSceneBuilder struct{}

func NewAvatarSceneBuilder() *AvatarSceneBuilder {
	return &AvatarSceneBuilder{}
}

func (b *AvatarSceneBuilder) Build(input LocalIpTalkingAvatarRenderInput, asset *CharacterAsset, durationSec float64, audioAnalysisPath, lipPath, motionPath string) AvatarScene {
	return AvatarScene{
		CharacterID:          asset.CharacterID,
		DurationSec:          roundFloat(durationSec, 4),
		FPS:                  input.FPS,
		Resolution:           input.Resolution,
		Assets:               asset.AssetPaths,
		LipSyncTimelinePath:  lipPath,
		MotionTimelinePath:   motionPath,
		AudioAnalysisPath:    audioAnalysisPath,
		Style:                input.Style,
		Renderer:             input.RenderMode,
		CharacterDisplayName: asset.DisplayName,
		CharacterAssetRoot:   asset.RootDir,
		VoiceProfile:         effectiveVoiceProfile(input, asset),
		HyperGenControl:      hyperGenControlForAsset(asset),
		GenerationMetadata: map[string]interface{}{
			"schemaVersion":    "local-ip-talking-avatar-scene/v1",
			"generatedAt":      nowRFC3339(),
			"renderMode":       input.RenderMode,
			"interactionLevel": input.InteractionLevel,
			"notAIGC":          true,
		},
	}
}

func WriteAvatarScene(outputDir string, scene AvatarScene) (string, error) {
	path := filepath.Join(outputDir, "avatar_scene.json")
	return path, writeJSONFile(path, scene)
}

func hyperGenControlForAsset(asset *CharacterAsset) map[string]interface{} {
	control := map[string]interface{}{
		"renderer":       asset.Renderer,
		"characterId":    asset.CharacterID,
		"displayName":    asset.DisplayName,
		"referenceSvg":   asset.AssetPaths["referenceSvg"],
		"rigPath":        asset.AssetPaths["rig"],
		"controlVersion": "hypergen-ip-puppet/v1",
		"partIds": []string{
			"root",
			"body",
			"head",
			"faceScreen",
			"eyeLeft",
			"eyeRight",
			"mouth",
			"leftArm",
			"rightArm",
			"props",
		},
		"motionChannels": []string{
			"translateX",
			"translateY",
			"scale",
			"bodyFloat",
			"headTilt",
			"blink",
			"mouthOpen",
			"gesture",
			"glowPulse",
		},
		"recommendedTimeline": []map[string]interface{}{
			{"timeSec": 0, "channel": "bodyFloat", "value": "idle_breath"},
			{"timeSec": 0, "channel": "mouthOpen", "value": "bind_lip_sync_timeline"},
			{"timeSec": 0, "channel": "gesture", "value": "bind_motion_timeline"},
		},
	}
	if asset.ControlRig != nil {
		control["rig"] = asset.ControlRig
	}
	if asset.Renderer == "live2d" {
		control["live2d"] = asset.Live2D
		control["note"] = "Live2D assets require a Cubism renderer bridge before local preview rendering."
	}
	return control
}
