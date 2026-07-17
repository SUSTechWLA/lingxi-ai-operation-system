package localtool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type CharacterAsset struct {
	RootDir           string
	CharacterID       string
	DisplayName       string
	Type              string
	Renderer          string
	DefaultExpression string
	DefaultPose       string
	Canvas            Resolution
	Anchor            CharacterAnchor
	Assets            map[string]string
	AssetPaths        map[string]string
	Capabilities      map[string]bool
	VoiceProfile      VoiceProfile
	ControlRig        map[string]interface{}
	Live2D            map[string]interface{}
}

type CharacterAnchor struct {
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Scale float64 `json:"scale"`
}

type characterAssetManifest struct {
	CharacterID       string                 `json:"characterId"`
	DisplayName       string                 `json:"displayName"`
	Type              string                 `json:"type"`
	Renderer          string                 `json:"renderer"`
	DefaultExpression string                 `json:"defaultExpression"`
	DefaultPose       string                 `json:"defaultPose"`
	Canvas            Resolution             `json:"canvas"`
	Anchor            CharacterAnchor        `json:"anchor"`
	Assets            map[string]string      `json:"assets"`
	Capabilities      map[string]bool        `json:"capabilities"`
	VoiceProfile      VoiceProfile           `json:"voiceProfile,omitempty"`
	ControlRig        map[string]interface{} `json:"controlRig,omitempty"`
	Live2D            map[string]interface{} `json:"live2d,omitempty"`
}

type IpAssetLoader struct {
	dataDir string
}

func NewIpAssetLoader(dataDir string) *IpAssetLoader {
	return &IpAssetLoader{dataDir: dataDir}
}

func (l *IpAssetLoader) Load(characterID string) (*CharacterAsset, error) {
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return nil, fmt.Errorf("characterId is required")
	}
	if err := validateLocalSegment(characterID); err != nil {
		return nil, fmt.Errorf("invalid characterId: %w", err)
	}

	root, err := l.findCharacterRoot(characterID)
	if err != nil {
		return nil, err
	}
	manifestPath := filepath.Join(root, "character.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read character.json: %w", err)
	}
	var manifest characterAssetManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse character.json: %w", err)
	}
	if strings.TrimSpace(manifest.CharacterID) == "" {
		return nil, fmt.Errorf("character.json missing characterId")
	}
	if manifest.CharacterID != characterID {
		return nil, fmt.Errorf("character.json characterId %q does not match requested %q", manifest.CharacterID, characterID)
	}
	if manifest.Renderer == "" {
		manifest.Renderer = "sprite2d"
	}
	if manifest.Renderer != "sprite2d" && manifest.Renderer != "svg2d" && manifest.Renderer != "live2d" {
		return nil, fmt.Errorf("unsupported character renderer %q", manifest.Renderer)
	}
	if manifest.Canvas.Width <= 0 {
		manifest.Canvas.Width = 1920
	}
	if manifest.Canvas.Height <= 0 {
		manifest.Canvas.Height = 1080
	}
	if manifest.Anchor.Scale <= 0 {
		manifest.Anchor.Scale = 1
	}

	required := requiredAssetsForRenderer(manifest.Renderer)
	assetPaths := map[string]string{}
	for _, key := range required {
		rel := strings.TrimSpace(manifest.Assets[key])
		if rel == "" {
			return nil, fmt.Errorf("character asset %q missing required asset %q", characterID, key)
		}
		path, err := resolveCharacterAssetPath(root, rel, characterID, key)
		if err != nil {
			return nil, err
		}
		assetPaths[key] = path
	}
	for key, rel := range manifest.Assets {
		if _, ok := assetPaths[key]; ok {
			continue
		}
		rel = strings.TrimSpace(rel)
		if rel == "" || strings.Contains(rel, "..") || filepath.IsAbs(rel) {
			continue
		}
		path := filepath.Join(root, filepath.FromSlash(rel))
		if info, err := os.Stat(path); err == nil && !info.IsDir() && info.Size() > 0 {
			assetPaths[key] = path
		}
	}
	if manifest.VoiceProfile.Persona == "" {
		manifest.VoiceProfile.Persona = manifest.CharacterID
	}
	if manifest.VoiceProfile.DisplayName == "" {
		manifest.VoiceProfile.DisplayName = manifest.DisplayName
	}
	if manifest.VoiceProfile.Provider == "" {
		manifest.VoiceProfile.Provider = "local_preview"
	}

	if rigPath := assetPaths["rig"]; rigPath != "" {
		var rig map[string]interface{}
		if data, err := os.ReadFile(rigPath); err == nil {
			if err := json.Unmarshal(data, &rig); err == nil {
				manifest.ControlRig = rig
			}
		}
	}

	return &CharacterAsset{
		RootDir:           root,
		CharacterID:       manifest.CharacterID,
		DisplayName:       manifest.DisplayName,
		Type:              manifest.Type,
		Renderer:          manifest.Renderer,
		DefaultExpression: manifest.DefaultExpression,
		DefaultPose:       manifest.DefaultPose,
		Canvas:            manifest.Canvas,
		Anchor:            manifest.Anchor,
		Assets:            manifest.Assets,
		AssetPaths:        assetPaths,
		Capabilities:      manifest.Capabilities,
		VoiceProfile:      manifest.VoiceProfile,
		ControlRig:        manifest.ControlRig,
		Live2D:            manifest.Live2D,
	}, nil
}

func requiredAssetsForRenderer(renderer string) []string {
	switch renderer {
	case "svg2d":
		return []string{"referenceSvg", "rig"}
	case "live2d":
		return []string{"model3Json"}
	default:
		return []string{
			"body",
			"head",
			"eyeOpen",
			"eyeHalf",
			"eyeClose",
			"mouthClosed",
			"mouthA",
			"mouthO",
			"mouthE",
			"mouthI",
			"mouthU",
		}
	}
}

func resolveCharacterAssetPath(root, rel, characterID, key string) (string, error) {
	if strings.Contains(rel, "..") || filepath.IsAbs(rel) {
		return "", fmt.Errorf("character asset %q has unsafe path for %q: %s", characterID, key, rel)
	}
	path := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("character asset %q missing file for %q: %s", characterID, key, path)
		}
		return "", fmt.Errorf("stat character asset %q for %q: %w", characterID, key, err)
	}
	if info.IsDir() || info.Size() == 0 {
		return "", fmt.Errorf("character asset %q file for %q is not readable: %s", characterID, key, path)
	}
	return path, nil
}

func (l *IpAssetLoader) findCharacterRoot(characterID string) (string, error) {
	candidates := []string{}
	if envRoot := strings.TrimSpace(os.Getenv("TANGYING_IP_CHARACTER_ROOT")); envRoot != "" {
		candidates = append(candidates, filepath.Join(envRoot, characterID))
	}
	if l.dataDir != "" {
		candidates = append(candidates, filepath.Join(l.dataDir, "assets", "characters", characterID))
	}
	if wd, err := os.Getwd(); err == nil {
		for _, root := range upwardAssetRoots(wd) {
			candidates = append(candidates, filepath.Join(root, characterID))
		}
	}
	if exe, err := os.Executable(); err == nil {
		for _, root := range upwardAssetRoots(filepath.Dir(exe)) {
			candidates = append(candidates, filepath.Join(root, characterID))
		}
	}
	seen := map[string]bool{}
	for _, candidate := range candidates {
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		if info, err := os.Stat(filepath.Join(candidate, "character.json")); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("character asset not found for %q; expected assets/characters/%s/character.json", characterID, characterID)
}

func upwardAssetRoots(start string) []string {
	var roots []string
	current := filepath.Clean(start)
	for i := 0; i < 8; i++ {
		roots = append(roots, filepath.Join(current, "assets", "characters"))
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return roots
}
