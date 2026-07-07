package localtool

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
)

type Sprite2DRenderer struct {
	ffmpegPath string
}

type spriteLayers struct {
	body        image.Image
	head        image.Image
	hair        image.Image
	leftArm     image.Image
	rightArm    image.Image
	eyeOpen     image.Image
	eyeHalf     image.Image
	eyeClose    image.Image
	mouthClosed image.Image
	mouthA      image.Image
	mouthO      image.Image
	mouthE      image.Image
	mouthI      image.Image
	mouthU      image.Image
}

type renderMotionState struct {
	bodyDy       float64
	headDy       float64
	scalePulse   float64
	rightArmDx   float64
	rightArmDy   float64
	blinkPhase   float64
	nodStrength  float64
	gesturePhase float64
}

func NewSprite2DRenderer() *Sprite2DRenderer {
	return &Sprite2DRenderer{}
}

func (r *Sprite2DRenderer) Render(ctx context.Context, outputDir string, input LocalIpTalkingAvatarRenderInput, asset *CharacterAsset, lips []LipSyncFrame, motions []MotionEvent, durationSec float64) (string, string, error) {
	if asset == nil {
		return "", "", fmt.Errorf("character asset is required")
	}
	if asset.Renderer != "sprite2d" {
		return "", "", fmt.Errorf("renderMode %q is reserved but not implemented in this version; use sprite2d", asset.Renderer)
	}
	layers, err := loadSpriteLayers(asset)
	if err != nil {
		return "", "", err
	}
	framesDir := filepath.Join(outputDir, "frames")
	if err := os.RemoveAll(framesDir); err != nil {
		return "", "", fmt.Errorf("clean frames dir: %w", err)
	}
	if err := os.MkdirAll(framesDir, 0o755); err != nil {
		return "", "", fmt.Errorf("create frames dir: %w", err)
	}
	frameCount := int(math.Ceil(durationSec * float64(input.FPS)))
	if frameCount < 1 {
		frameCount = 1
	}
	for frame := 0; frame < frameCount; frame++ {
		t := float64(frame) / float64(input.FPS)
		img := image.NewRGBA(image.Rect(0, 0, input.Resolution.Width, input.Resolution.Height))
		renderSpriteFrame(img, input, asset, layers, lipAt(lips, frame), motionAt(motions, t), t)
		framePath := filepath.Join(framesDir, fmt.Sprintf("frame_%06d.png", frame+1))
		if err := writePNG(framePath, img); err != nil {
			return "", "", err
		}
	}
	avatarPath, err := r.encodeAvatarLayer(ctx, outputDir, input.FPS)
	if err != nil {
		return "", "", err
	}
	return framesDir, avatarPath, nil
}

func loadSpriteLayers(asset *CharacterAsset) (*spriteLayers, error) {
	load := func(key string) (image.Image, error) {
		path := asset.AssetPaths[key]
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open sprite layer %s: %w", key, err)
		}
		defer f.Close()
		img, err := png.Decode(f)
		if err != nil {
			return nil, fmt.Errorf("decode sprite layer %s as PNG: %w", key, err)
		}
		return img, nil
	}
	var err error
	l := &spriteLayers{}
	if l.body, err = load("body"); err != nil {
		return nil, err
	}
	if l.head, err = load("head"); err != nil {
		return nil, err
	}
	if l.eyeOpen, err = load("eyeOpen"); err != nil {
		return nil, err
	}
	if l.eyeHalf, err = load("eyeHalf"); err != nil {
		return nil, err
	}
	if l.eyeClose, err = load("eyeClose"); err != nil {
		return nil, err
	}
	if l.mouthClosed, err = load("mouthClosed"); err != nil {
		return nil, err
	}
	if l.mouthA, err = load("mouthA"); err != nil {
		return nil, err
	}
	if l.mouthO, err = load("mouthO"); err != nil {
		return nil, err
	}
	if l.mouthE, err = load("mouthE"); err != nil {
		return nil, err
	}
	if l.mouthI, err = load("mouthI"); err != nil {
		return nil, err
	}
	if l.mouthU, err = load("mouthU"); err != nil {
		return nil, err
	}
	if path := asset.AssetPaths["hair"]; path != "" {
		l.hair, _ = load("hair")
	}
	if path := asset.AssetPaths["leftArm"]; path != "" {
		l.leftArm, _ = load("leftArm")
	}
	if path := asset.AssetPaths["rightArm"]; path != "" {
		l.rightArm, _ = load("rightArm")
	}
	return l, nil
}

func renderSpriteFrame(dst *image.RGBA, input LocalIpTalkingAvatarRenderInput, asset *CharacterAsset, layers *spriteLayers, lip LipSyncFrame, motion renderMotionState, t float64) {
	baseScale := math.Min(float64(input.Resolution.Width)/float64(asset.Canvas.Width), float64(input.Resolution.Height)/float64(asset.Canvas.Height))
	styleScale := input.Style.Scale
	if styleScale <= 0 {
		styleScale = 1
	}
	scale := baseScale * asset.Anchor.Scale * styleScale * (1 + motion.scalePulse)
	x, y := spriteOrigin(input, asset, scale)
	bodyDy := motion.bodyDy
	headDy := motion.headDy

	drawLayer(dst, layers.body, x, y+bodyDy, scale)
	if layers.leftArm != nil {
		drawLayer(dst, layers.leftArm, x, y+bodyDy, scale)
	}
	if layers.rightArm != nil {
		drawLayer(dst, layers.rightArm, x+motion.rightArmDx, y+bodyDy+motion.rightArmDy, scale)
	}
	drawLayer(dst, layers.head, x, y+bodyDy+headDy, scale)
	if layers.hair != nil {
		drawLayer(dst, layers.hair, x, y+bodyDy+headDy, scale)
	}
	switch {
	case motion.blinkPhase > 0.68:
		drawLayer(dst, layers.eyeClose, x, y+bodyDy+headDy, scale)
	case motion.blinkPhase > 0.24:
		drawLayer(dst, layers.eyeHalf, x, y+bodyDy+headDy, scale)
	default:
		drawLayer(dst, layers.eyeOpen, x, y+bodyDy+headDy, scale)
	}
	drawLayer(dst, mouthLayerFor(layers, lip.Mouth), x, y+bodyDy+headDy, scale)
	_ = t
}

func spriteOrigin(input LocalIpTalkingAvatarRenderInput, asset *CharacterAsset, scale float64) (float64, float64) {
	width := float64(asset.Canvas.Width) * scale
	height := float64(asset.Canvas.Height) * scale
	marginX := float64(input.Resolution.Width) * 0.04
	switch input.Style.Position {
	case "left_bottom":
		return marginX, float64(input.Resolution.Height) - height
	case "right_bottom":
		return float64(input.Resolution.Width) - width - marginX, float64(input.Resolution.Height) - height
	case "center":
		return (float64(input.Resolution.Width) - width) / 2, (float64(input.Resolution.Height) - height) / 2
	default:
		return (float64(input.Resolution.Width) - width) / 2, float64(input.Resolution.Height) - height
	}
}

func drawLayer(dst *image.RGBA, src image.Image, x, y, scale float64) {
	if src == nil {
		return
	}
	if math.Abs(scale-1) < 0.001 && int(math.Round(x)) == 0 && int(math.Round(y)) == 0 && src.Bounds().Size() == dst.Bounds().Size() {
		draw.Draw(dst, dst.Bounds(), src, src.Bounds().Min, draw.Over)
		return
	}
	sb := src.Bounds()
	w := int(math.Round(float64(sb.Dx()) * scale))
	h := int(math.Round(float64(sb.Dy()) * scale))
	if w <= 0 || h <= 0 {
		return
	}
	startX := int(math.Round(x))
	startY := int(math.Round(y))
	for dy := 0; dy < h; dy++ {
		sy := sb.Min.Y + int(float64(dy)/scale)
		if sy < sb.Min.Y {
			sy = sb.Min.Y
		}
		if sy >= sb.Max.Y {
			sy = sb.Max.Y - 1
		}
		ty := startY + dy
		if ty < dst.Bounds().Min.Y || ty >= dst.Bounds().Max.Y {
			continue
		}
		for dx := 0; dx < w; dx++ {
			sx := sb.Min.X + int(float64(dx)/scale)
			if sx < sb.Min.X {
				sx = sb.Min.X
			}
			if sx >= sb.Max.X {
				sx = sb.Max.X - 1
			}
			tx := startX + dx
			if tx < dst.Bounds().Min.X || tx >= dst.Bounds().Max.X {
				continue
			}
			blendPixel(dst, tx, ty, color.NRGBAModel.Convert(src.At(sx, sy)).(color.NRGBA))
		}
	}
}

func blendPixel(dst *image.RGBA, x, y int, src color.NRGBA) {
	if src.A == 0 {
		return
	}
	di := dst.PixOffset(x, y)
	da := float64(dst.Pix[di+3]) / 255
	sa := float64(src.A) / 255
	outA := sa + da*(1-sa)
	if outA <= 0 {
		return
	}
	dr := float64(dst.Pix[di]) / 255
	dg := float64(dst.Pix[di+1]) / 255
	db := float64(dst.Pix[di+2]) / 255
	sr := float64(src.R) / 255
	sg := float64(src.G) / 255
	sb := float64(src.B) / 255
	dst.Pix[di] = uint8(math.Round(((sr * sa) + (dr * da * (1 - sa))) / outA * 255))
	dst.Pix[di+1] = uint8(math.Round(((sg * sa) + (dg * da * (1 - sa))) / outA * 255))
	dst.Pix[di+2] = uint8(math.Round(((sb * sa) + (db * da * (1 - sa))) / outA * 255))
	dst.Pix[di+3] = uint8(math.Round(outA * 255))
}

func mouthLayerFor(layers *spriteLayers, mouth string) image.Image {
	switch mouth {
	case "mouth_a":
		return layers.mouthA
	case "mouth_o":
		return layers.mouthO
	case "mouth_e":
		return layers.mouthE
	case "mouth_i":
		return layers.mouthI
	case "mouth_u":
		return layers.mouthU
	default:
		return layers.mouthClosed
	}
}

func lipAt(lips []LipSyncFrame, frame int) LipSyncFrame {
	if len(lips) == 0 {
		return LipSyncFrame{Mouth: "mouth_closed"}
	}
	if frame < 0 {
		frame = 0
	}
	if frame >= len(lips) {
		frame = len(lips) - 1
	}
	return lips[frame]
}

func motionAt(events []MotionEvent, t float64) renderMotionState {
	state := renderMotionState{}
	for _, event := range events {
		if t < event.TimeSec || t > event.TimeSec+event.Duration {
			continue
		}
		progress := 0.0
		if event.Duration > 0 {
			progress = (t - event.TimeSec) / event.Duration
		}
		eased := math.Sin(progress * math.Pi)
		strength := event.Strength
		if strength <= 0 {
			strength = 1
		}
		switch event.Motion {
		case "idle_breath":
			state.bodyDy += math.Sin(t*math.Pi*0.9) * 2.5 * strength
			state.scalePulse += math.Sin(t*math.Pi*0.9) * 0.008 * strength
		case "blink":
			state.blinkPhase = math.Max(state.blinkPhase, eased)
		case "head_nod":
			state.headDy += eased * 7 * strength
			state.nodStrength = math.Max(state.nodStrength, strength)
		case "gesture_point":
			state.rightArmDx += eased * 10 * strength
			state.rightArmDy -= eased * 12 * strength
			state.gesturePhase = math.Max(state.gesturePhase, eased)
		case "gesture_present":
			state.rightArmDx += eased * 18 * strength
			state.rightArmDy -= eased * 7 * strength
			state.scalePulse += eased * 0.006 * strength
			state.gesturePhase = math.Max(state.gesturePhase, eased*0.82)
		case "gesture_wave":
			state.rightArmDx += math.Sin(progress*math.Pi*4) * 12 * strength
			state.rightArmDy -= eased * 14 * strength
			state.gesturePhase = math.Max(state.gesturePhase, eased)
		case "head_tilt":
			state.headDy += math.Sin(progress*math.Pi*2) * 4 * strength
			state.gesturePhase = math.Max(state.gesturePhase, eased*0.35)
		case "glow_pulse":
			state.scalePulse += eased * 0.012 * strength
		}
	}
	return state
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create frame %s: %w", path, err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return fmt.Errorf("write frame %s: %w", path, err)
	}
	return nil
}

func (r *Sprite2DRenderer) encodeAvatarLayer(ctx context.Context, outputDir string, fps int) (string, error) {
	ffmpeg := r.ffmpegPath
	if ffmpeg == "" {
		var err error
		ffmpeg, err = exec.LookPath("ffmpeg")
		if err != nil {
			return "", fmt.Errorf("ffmpeg is required to encode avatar layer")
		}
	}
	webmPath := filepath.Join(outputDir, "avatar_layer.webm")
	inputPattern := filepath.Join(outputDir, "frames", "frame_%06d.png")
	cmd := exec.CommandContext(ctx, ffmpeg,
		"-y",
		"-framerate", fmt.Sprintf("%d", fps),
		"-i", inputPattern,
		"-c:v", "libvpx-vp9",
		"-pix_fmt", "yuva420p",
		"-auto-alt-ref", "0",
		"-crf", "32",
		"-b:v", "0",
		webmPath,
	)
	if out, err := cmd.CombinedOutput(); err == nil {
		return webmPath, nil
	} else if len(out) > 0 {
		_ = os.WriteFile(filepath.Join(outputDir, "avatar_layer_encode_error.log"), out, 0o644)
	}

	mp4Path := filepath.Join(outputDir, "avatar_layer.mp4")
	cmd = exec.CommandContext(ctx, ffmpeg,
		"-y",
		"-framerate", fmt.Sprintf("%d", fps),
		"-i", inputPattern,
		"-c:v", "libx264",
		"-pix_fmt", "yuv420p",
		"-r", fmt.Sprintf("%d", fps),
		mp4Path,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("encode avatar layer failed: %w; output: %s", err, trimCommandOutput(out))
	}
	return mp4Path, nil
}
