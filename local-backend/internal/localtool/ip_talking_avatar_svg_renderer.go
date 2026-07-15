package localtool

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strings"
)

type SVG2DRenderer struct{}

func NewSVG2DRenderer() *SVG2DRenderer {
	return &SVG2DRenderer{}
}

func (r *SVG2DRenderer) Render(ctx context.Context, outputDir string, input LocalIpTalkingAvatarRenderInput, asset *CharacterAsset, lips []LipSyncFrame, motions []MotionEvent, durationSec float64) (string, string, error) {
	if asset == nil {
		return "", "", fmt.Errorf("character asset is required")
	}
	if asset.Renderer != "svg2d" {
		return "", "", fmt.Errorf("renderMode %q is not supported by svg2d renderer", asset.Renderer)
	}
	framesDir := filepath.Join(outputDir, "frames")
	if err := os.RemoveAll(framesDir); err != nil {
		return "", "", fmt.Errorf("clean frames dir: %w", err)
	}
	if err := os.MkdirAll(framesDir, 0o755); err != nil {
		return "", "", fmt.Errorf("create frames dir: %w", err)
	}
	texture, err := loadPuppetTexture(asset)
	if err != nil {
		return "", "", err
	}
	frameCount := int(math.Ceil(durationSec * float64(input.FPS)))
	if frameCount < 1 {
		frameCount = 1
	}
	for frame := 0; frame < frameCount; frame++ {
		t := float64(frame) / float64(input.FPS)
		img := image.NewRGBA(image.Rect(0, 0, input.Resolution.Width, input.Resolution.Height))
		renderPuppetFrame(img, input, asset, texture, lipAt(lips, frame), motionAt(motions, t), t)
		framePath := filepath.Join(framesDir, fmt.Sprintf("frame_%06d.png", frame+1))
		if err := writePNG(framePath, img); err != nil {
			return "", "", err
		}
	}
	avatarPath, err := NewSprite2DRenderer().encodeAvatarLayer(ctx, outputDir, input.FPS)
	if err != nil {
		return "", "", err
	}
	return framesDir, avatarPath, nil
}

type puppetTexture struct {
	img image.Image
	w   int
	h   int
}

type placedTexture struct {
	x    float64
	y    float64
	w    float64
	h    float64
	srcW float64
	srcH float64
}

func loadPuppetTexture(asset *CharacterAsset) (*puppetTexture, error) {
	path := asset.AssetPaths["frontTexture"]
	if path == "" {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open frontTexture: %w", err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode frontTexture: %w", err)
	}
	bounds := img.Bounds()
	return &puppetTexture{img: img, w: bounds.Dx(), h: bounds.Dy()}, nil
}

func renderPuppetFrame(dst *image.RGBA, input LocalIpTalkingAvatarRenderInput, asset *CharacterAsset, texture *puppetTexture, lip LipSyncFrame, motion renderMotionState, t float64) {
	id := strings.ToLower(asset.CharacterID)
	if texture != nil {
		renderTexturedPuppetFrame(dst, input, asset, texture, lip, motion, t)
		return
	}
	if strings.Contains(id, "aster") {
		renderAsterFrame(dst, input, asset, lip, motion, t)
		return
	}
	renderBoboFrame(dst, input, asset, lip, motion, t)
}

func puppetPose(input LocalIpTalkingAvatarRenderInput, asset *CharacterAsset, heightRatio float64) (float64, float64, float64) {
	scale := input.Style.Scale
	if scale <= 0 {
		scale = 1
	}
	scale *= asset.Anchor.Scale
	charH := float64(input.Resolution.Height) * heightRatio * scale
	unit := charH / 620
	marginX := float64(input.Resolution.Width) * 0.08
	baseY := float64(input.Resolution.Height) * 0.88
	centerX := float64(input.Resolution.Width) * 0.5
	switch input.Style.Position {
	case "left_bottom":
		centerX = marginX + charH*0.34
	case "right_bottom":
		centerX = float64(input.Resolution.Width) - marginX - charH*0.34
	case "center":
		baseY = float64(input.Resolution.Height) * 0.72
	}
	return centerX, baseY, unit
}

func renderTexturedPuppetFrame(dst *image.RGBA, input LocalIpTalkingAvatarRenderInput, asset *CharacterAsset, texture *puppetTexture, lip LipSyncFrame, motion renderMotionState, t float64) {
	id := strings.ToLower(asset.CharacterID)
	isAster := strings.Contains(id, "aster")
	heightRatio := 0.93
	baseRatio := 0.995
	if isAster {
		heightRatio = 0.96
		baseRatio = 0.99
	}
	styleScale := input.Style.Scale
	if styleScale <= 0 {
		styleScale = 1
	}
	bodyFloat := motion.bodyDy*0.75 + math.Sin(t*math.Pi*0.85)*5
	if !isAster {
		bodyFloat += math.Sin(t*math.Pi*1.55) * 3
	}
	scalePulse := 1 + motion.scalePulse*0.65
	targetH := float64(input.Resolution.Height) * heightRatio * styleScale * asset.Anchor.Scale * scalePulse
	targetW := targetH * float64(texture.w) / float64(texture.h)
	cx := float64(input.Resolution.Width)*0.5 + motion.bodyDx*targetH/1536
	marginX := float64(input.Resolution.Width) * 0.06
	switch input.Style.Position {
	case "left_bottom":
		cx = marginX + targetW*0.5
	case "right_bottom":
		cx = float64(input.Resolution.Width) - marginX - targetW*0.5
	}
	baseY := float64(input.Resolution.Height) * baseRatio
	x := cx - targetW/2
	y := baseY - targetH + bodyFloat + motion.headDy*0.45
	placement := placedTexture{x: x, y: y, w: targetW, h: targetH, srcW: float64(texture.w), srcH: float64(texture.h)}

	drawFilledEllipseRGBA(dst, cx, baseY-13, targetW*0.32, targetH*0.035, color.NRGBA{R: 36, G: 48, B: 72, A: 44})
	if isAster {
		orbX, orbY := placement.point(512, 175)
		pulse := 0.5 + 0.5*math.Sin(t*math.Pi*0.95)
		drawFilledEllipseRGBA(dst, orbX, orbY, targetW*0.16*(1+0.03*pulse), targetH*0.105*(1+0.04*pulse), color.NRGBA{R: 58, G: 124, B: 255, A: uint8(22 + 28*pulse)})
		drawFilledEllipseRGBA(dst, orbX, orbY, targetW*0.09, targetH*0.06, color.NRGBA{R: 160, G: 218, B: 255, A: uint8(28 + 32*pulse)})
	} else {
		haloX, haloY := placement.point(512, 245)
		pulse := 0.55 + 0.45*math.Sin(t*math.Pi*1.25)
		drawFilledEllipseRGBA(dst, haloX, haloY, targetW*0.12*(1+0.03*pulse), targetH*0.08*(1+0.04*pulse), color.NRGBA{R: 255, G: 212, B: 88, A: uint8(20 + 26*pulse)})
	}
	drawImageBilinear(dst, texture.img, placement)
	if isAster {
		drawAsterTextureOverlays(dst, placement, lip, motion, t)
	} else {
		drawBoboTextureOverlays(dst, placement, lip, motion, t)
	}
}

func drawBoboTextureOverlays(dst *image.RGBA, placement placedTexture, lip LipSyncFrame, motion renderMotionState, t float64) {
	glow := 0.55 + 0.45*math.Sin(t*math.Pi*1.35)
	lEyeX, lEyeY := placement.point(430, 618)
	rEyeX, rEyeY := placement.point(612, 618)
	mouthX, mouthY := placement.point(512, 670)
	unit := placement.h / 1536
	drawBoboTextureLimbMotion(dst, placement, motion, t, glow)
	if motion.blinkPhase > 0.52 {
		drawFilledEllipseRGBA(dst, lEyeX, lEyeY, 32*unit, 31*unit, color.NRGBA{R: 8, G: 10, B: 12, A: 220})
		drawFilledEllipseRGBA(dst, rEyeX, rEyeY, 32*unit, 31*unit, color.NRGBA{R: 8, G: 10, B: 12, A: 220})
		drawCapsule(dst, lEyeX-22*unit, lEyeY+2*unit, lEyeX+22*unit, lEyeY+2*unit, 5*unit, color.NRGBA{R: 255, G: 222, B: 88, A: 240})
		drawCapsule(dst, rEyeX-22*unit, rEyeY+2*unit, rEyeX+22*unit, rEyeY+2*unit, 5*unit, color.NRGBA{R: 255, G: 222, B: 88, A: 240})
	}
	open := math.Max(0, math.Min(1, lip.Open))
	if open > 0.11 {
		drawFilledRoundedRect(dst, mouthX-48*unit, mouthY-26*unit, mouthX+48*unit, mouthY+29*unit, 22*unit, color.NRGBA{R: 4, G: 7, B: 9, A: 205})
		switch lip.Mouth {
		case "mouth_o", "mouth_u":
			drawFilledEllipseRGBA(dst, mouthX, mouthY, (23+15*open)*unit, (22+25*open)*unit, color.NRGBA{R: 255, G: 220, B: 85, A: 245})
			drawFilledEllipseRGBA(dst, mouthX, mouthY, (10+8*open)*unit, (9+13*open)*unit, color.NRGBA{R: 34, G: 18, B: 24, A: 230})
		default:
			drawFilledRoundedRect(dst, mouthX-(32+22*open)*unit, mouthY-(5+5*open)*unit, mouthX+(32+22*open)*unit, mouthY+(10+24*open)*unit, 14*unit, color.NRGBA{R: 255, G: 220, B: 85, A: 246})
			drawFilledRoundedRect(dst, mouthX-(18+10*open)*unit, mouthY+(2-2*open)*unit, mouthX+(18+10*open)*unit, mouthY+(6+10*open)*unit, 6*unit, color.NRGBA{R: 35, G: 17, B: 23, A: 205})
		}
	}
	drawFilledEllipseRGBA(dst, lEyeX, lEyeY, 54*unit, 54*unit, color.NRGBA{R: 255, G: 214, B: 73, A: uint8(20 + 25*glow)})
	drawFilledEllipseRGBA(dst, rEyeX, rEyeY, 54*unit, 54*unit, color.NRGBA{R: 255, G: 214, B: 73, A: uint8(20 + 25*glow)})
	wave := math.Max(motion.gesturePhase, math.Max(0, math.Sin(t*math.Pi*0.78))*0.3)
	if wave > 0.22 {
		handX, handY := placement.point(795, 988)
		drawBoboSpark(dst, handX+34*unit, handY-70*unit, 18*unit*(0.8+wave), color.NRGBA{R: 255, G: 224, B: 91, A: uint8(130 + 80*wave)})
		drawBoboSpark(dst, handX+80*unit, handY-30*unit, 12*unit*(0.8+wave), color.NRGBA{R: 85, G: 200, B: 255, A: uint8(110 + 90*wave)})
	}
	heartX, heartY := placement.point(654, 1002)
	drawFilledEllipseRGBA(dst, heartX, heartY, 48*unit*(1+0.08*glow), 48*unit*(1+0.08*glow), color.NRGBA{R: 255, G: 214, B: 70, A: 30})
}

func drawAsterTextureOverlays(dst *image.RGBA, placement placedTexture, lip LipSyncFrame, motion renderMotionState, t float64) {
	unit := placement.h / 1536
	pulse := 0.5 + 0.5*math.Sin(t*math.Pi*0.95)
	drawAsterTextureLimbMotion(dst, placement, motion, t, pulse)
	eyeX, eyeY := placement.point(404, 595)
	mouthX, mouthY := placement.point(510, 685)
	if motion.blinkPhase > 0.54 {
		drawFilledEllipseRGBA(dst, eyeX, eyeY, 43*unit, 31*unit, color.NRGBA{R: 8, G: 13, B: 25, A: 220})
		drawCapsule(dst, eyeX-29*unit, eyeY+3*unit, eyeX+22*unit, eyeY+3*unit, 5*unit, color.NRGBA{R: 92, G: 212, B: 255, A: 235})
	}
	open := math.Max(0, math.Min(1, lip.Open))
	if open > 0.1 {
		drawFilledRoundedRect(dst, mouthX-30*unit, mouthY-12*unit, mouthX+26*unit, mouthY+18*unit, 10*unit, color.NRGBA{R: 8, G: 13, B: 22, A: 190})
		drawFilledRoundedRect(dst, mouthX-(18+10*open)*unit, mouthY-(2+3*open)*unit, mouthX+(18+10*open)*unit, mouthY+(4+14*open)*unit, 6*unit, color.NRGBA{R: 80, G: 204, B: 255, A: 210})
	}
	lensX, lensY := placement.point(650, 565)
	drawStrokeEllipse(dst, lensX, lensY, 70*unit*(1+0.035*pulse), 70*unit*(1+0.035*pulse), 3.5*unit, color.NRGBA{R: 125, G: 215, B: 255, A: uint8(60 + 65*pulse)})
	if motion.gesturePhase > 0.2 {
		drawCapsule(dst, lensX-48*unit, lensY-45*unit, lensX+42*unit, lensY+36*unit, 3.2*unit, color.NRGBA{R: 245, G: 250, B: 255, A: uint8(95 + 95*motion.gesturePhase)})
	}
	orbX, orbY := placement.point(512, 175)
	drawStrokeEllipse(dst, orbX, orbY, 104*unit*(1+0.05*pulse), 62*unit*(1+0.04*pulse), 3*unit, color.NRGBA{R: 134, G: 192, B: 255, A: uint8(45 + 70*pulse)})
	chestX, chestY := placement.point(512, 1040)
	drawFilledEllipseRGBA(dst, chestX, chestY, 118*unit*(1+0.03*pulse), 118*unit*(1+0.03*pulse), color.NRGBA{R: 66, G: 178, B: 255, A: uint8(18 + 34*pulse)})
}

func drawBoboTextureLimbMotion(dst *image.RGBA, placement placedTexture, motion renderMotionState, t, glow float64) {
	unit := placement.h / 1536
	idle := 0.35 + 0.25*math.Sin(t*math.Pi*0.72)
	leftShoulderX, leftShoulderY := placement.point(270, 995)
	rightShoulderX, rightShoulderY := placement.point(748, 990)
	leftHandX, leftHandY := placement.point(205, 1108)
	rightHandX, rightHandY := placement.point(806, 1018)
	leftHandX += (motion.leftArmDx*8 - idle*11) * unit
	leftHandY += (motion.leftArmDy*8 + math.Sin(t*math.Pi*0.82)*7) * unit
	rightHandX += (motion.rightArmDx*8 + motion.gesturePhase*28) * unit
	rightHandY += (motion.rightArmDy*8 - motion.gesturePhase*44 + math.Sin(t*math.Pi*0.86)*5) * unit
	arm := color.NRGBA{R: 255, G: 255, B: 248, A: 212}
	hand := color.NRGBA{R: 255, G: 217, B: 72, A: 225}
	drawCapsule(dst, leftShoulderX, leftShoulderY, leftHandX, leftHandY, 18*unit, arm)
	drawFilledEllipseRGBA(dst, leftHandX, leftHandY, 28*unit, 25*unit, hand)
	drawCapsule(dst, rightShoulderX, rightShoulderY, rightHandX, rightHandY, 18*unit, arm)
	drawFilledEllipseRGBA(dst, rightHandX, rightHandY, 28*unit, 25*unit, hand)
	if motion.gesturePhase > 0.18 {
		drawCapsule(dst, rightHandX+10*unit, rightHandY-3*unit, rightHandX+52*unit, rightHandY-30*unit, 5*unit, hand)
		drawBoboSpark(dst, rightHandX+62*unit, rightHandY-42*unit, 12*unit*(0.8+motion.gesturePhase), color.NRGBA{R: 255, G: 226, B: 74, A: uint8(120 + 95*motion.gesturePhase)})
	}
	leftFootX, leftFootY := placement.point(365, 1448)
	rightFootX, rightFootY := placement.point(660, 1448)
	leftFootY += motion.leftFootDy * 7 * unit
	rightFootY += motion.rightFootDy * 7 * unit
	footA := uint8(76 + 80*glow)
	drawFilledEllipseRGBA(dst, leftFootX, leftFootY, 46*unit, 20*unit, color.NRGBA{R: 87, G: 173, B: 255, A: footA})
	drawFilledEllipseRGBA(dst, rightFootX, rightFootY, 46*unit, 20*unit, color.NRGBA{R: 255, G: 207, B: 74, A: footA})
}

func drawAsterTextureLimbMotion(dst *image.RGBA, placement placedTexture, motion renderMotionState, t, pulse float64) {
	unit := placement.h / 1536
	leftShoulderX, leftShoulderY := placement.point(315, 910)
	rightShoulderX, rightShoulderY := placement.point(716, 910)
	leftHandX, leftHandY := placement.point(260, 1112)
	rightHandX, rightHandY := placement.point(760, 1048)
	leftHandX += (motion.leftArmDx*7 - math.Sin(t*math.Pi*0.62)*5) * unit
	leftHandY += (motion.leftArmDy*7 + math.Sin(t*math.Pi*0.7)*4) * unit
	rightHandX += (motion.rightArmDx*8 + motion.gesturePhase*30) * unit
	rightHandY += (motion.rightArmDy*8 - motion.gesturePhase*38) * unit
	sleeve := color.NRGBA{R: 239, G: 244, B: 244, A: 210}
	gold := color.NRGBA{R: 224, G: 181, B: 82, A: 190}
	drawCapsule(dst, leftShoulderX, leftShoulderY, leftHandX, leftHandY, 15*unit, sleeve)
	drawFilledRoundedRect(dst, leftHandX-42*unit, leftHandY-24*unit, leftHandX+26*unit, leftHandY+22*unit, 8*unit, color.NRGBA{R: 42, G: 78, B: 112, A: 136})
	drawStrokeRoundedRect(dst, leftHandX-42*unit, leftHandY-24*unit, leftHandX+26*unit, leftHandY+22*unit, 8*unit, 3*unit, color.NRGBA{R: 118, G: 202, B: 255, A: 170})
	drawCapsule(dst, rightShoulderX, rightShoulderY, rightHandX, rightHandY, 15*unit, sleeve)
	drawFilledEllipseRGBA(dst, rightHandX, rightHandY, 24*unit, 23*unit, color.NRGBA{R: 246, G: 250, B: 248, A: 230})
	if motion.gesturePhase > 0.14 {
		drawCapsule(dst, rightHandX+9*unit, rightHandY-6*unit, rightHandX+54*unit, rightHandY-34*unit, 4.5*unit, color.NRGBA{R: 246, G: 250, B: 248, A: 235})
		drawStrokeEllipse(dst, rightHandX+64*unit, rightHandY-40*unit, 30*unit*(1+0.08*pulse), 18*unit*(1+0.06*pulse), 2.5*unit, gold)
	}
	leftFootX, leftFootY := placement.point(395, 1452)
	rightFootX, rightFootY := placement.point(628, 1452)
	leftFootY += motion.leftFootDy * 5 * unit
	rightFootY += motion.rightFootDy * 5 * unit
	drawFilledEllipseRGBA(dst, leftFootX, leftFootY, 54*unit, 17*unit, color.NRGBA{R: 80, G: 177, B: 255, A: uint8(42 + 60*pulse)})
	drawFilledEllipseRGBA(dst, rightFootX, rightFootY, 54*unit, 17*unit, color.NRGBA{R: 231, G: 185, B: 77, A: uint8(34 + 48*pulse)})
}

func (p placedTexture) point(srcX, srcY float64) (float64, float64) {
	return p.x + srcX/p.srcW*p.w, p.y + srcY/p.srcH*p.h
}

func drawImageBilinear(dst *image.RGBA, src image.Image, placement placedTexture) {
	sb := src.Bounds()
	if placement.w <= 0 || placement.h <= 0 {
		return
	}
	minX := int(math.Floor(placement.x))
	maxX := int(math.Ceil(placement.x + placement.w))
	minY := int(math.Floor(placement.y))
	maxY := int(math.Ceil(placement.y + placement.h))
	for y := minY; y < maxY; y++ {
		if y < dst.Bounds().Min.Y || y >= dst.Bounds().Max.Y {
			continue
		}
		fy := (float64(y)+0.5-placement.y)/placement.h*float64(sb.Dy()) - 0.5
		sy0 := int(math.Floor(fy))
		wy := fy - float64(sy0)
		for x := minX; x < maxX; x++ {
			if x < dst.Bounds().Min.X || x >= dst.Bounds().Max.X {
				continue
			}
			fx := (float64(x)+0.5-placement.x)/placement.w*float64(sb.Dx()) - 0.5
			sx0 := int(math.Floor(fx))
			wx := fx - float64(sx0)
			c := bilinearNRGBA(src, sb, sx0, sy0, wx, wy)
			blendPixel(dst, x, y, c)
		}
	}
}

func bilinearNRGBA(src image.Image, bounds image.Rectangle, sx0, sy0 int, wx, wy float64) color.NRGBA {
	sample := func(x, y int) color.NRGBA {
		if x < 0 {
			x = 0
		}
		if y < 0 {
			y = 0
		}
		if x >= bounds.Dx() {
			x = bounds.Dx() - 1
		}
		if y >= bounds.Dy() {
			y = bounds.Dy() - 1
		}
		return color.NRGBAModel.Convert(src.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.NRGBA)
	}
	c00 := sample(sx0, sy0)
	c10 := sample(sx0+1, sy0)
	c01 := sample(sx0, sy0+1)
	c11 := sample(sx0+1, sy0+1)
	interp := func(a00, a10, a01, a11 uint8) uint8 {
		top := float64(a00)*(1-wx) + float64(a10)*wx
		bottom := float64(a01)*(1-wx) + float64(a11)*wx
		return uint8(math.Round(top*(1-wy) + bottom*wy))
	}
	return color.NRGBA{
		R: interp(c00.R, c10.R, c01.R, c11.R),
		G: interp(c00.G, c10.G, c01.G, c11.G),
		B: interp(c00.B, c10.B, c01.B, c11.B),
		A: interp(c00.A, c10.A, c01.A, c11.A),
	}
}

func renderBoboFrame(dst *image.RGBA, input LocalIpTalkingAvatarRenderInput, asset *CharacterAsset, lip LipSyncFrame, motion renderMotionState, t float64) {
	cx, baseY, u := puppetPose(input, asset, 0.72)
	cx += motion.bodyDx * u
	bodyFloat := math.Sin(t*math.Pi*1.18)*10*u + motion.bodyDy*u*0.9
	expressive := input.InteractionLevel != "subtle"
	if expressive {
		bodyFloat += math.Sin(t*math.Pi*2.2) * 2.5 * u
	}
	bodyScale := 1 + motion.scalePulse*1.8 + math.Sin(t*math.Pi*1.18)*0.012
	headY := baseY - 420*u + bodyFloat + motion.headDy*u*1.2
	bodyY := baseY - 250*u + bodyFloat
	wave := motion.gesturePhase
	if expressive {
		wave = math.Max(wave, math.Max(0, math.Sin(t*math.Pi*0.85))*0.42)
	}
	glow := 0.55 + 0.45*math.Sin(t*math.Pi*1.35)

	drawFilledEllipseRGBA(dst, cx, bodyY+122*u, 164*u*bodyScale, 46*u, color.NRGBA{R: 80, G: 151, B: 255, A: 34})
	drawFilledEllipseRGBA(dst, cx, bodyY+104*u, 118*u*bodyScale, 28*u, color.NRGBA{R: 255, G: 199, B: 72, A: uint8(28 + 36*glow)})

	// Arms sit behind the head and cloud shell so gesture motion feels attached.
	leftArmX := cx - 126*u - wave*16*u + motion.leftArmDx*u
	leftArmY := bodyY - 34*u + math.Sin(t*math.Pi*1.4)*4*u + motion.leftArmDy*u
	rightArmX := cx + 126*u + wave*30*u + motion.rightArmDx*u
	rightArmY := bodyY - 36*u - wave*44*u + motion.rightArmDy*u
	drawCapsule(dst, leftArmX, leftArmY, cx-74*u, bodyY-18*u, 17*u, color.NRGBA{R: 255, G: 255, B: 249, A: 245})
	drawFilledEllipseRGBA(dst, leftArmX-8*u, leftArmY+3*u, 18*u, 18*u, color.NRGBA{R: 255, G: 210, B: 70, A: 235})
	drawCapsule(dst, cx+76*u, bodyY-18*u, rightArmX, rightArmY, 17*u, color.NRGBA{R: 255, G: 255, B: 249, A: 245})
	drawFilledEllipseRGBA(dst, rightArmX+8*u, rightArmY-3*u, 18*u, 18*u, color.NRGBA{R: 255, G: 210, B: 70, A: 240})

	drawBoboCloudShell(dst, cx, bodyY, u, bodyScale)
	drawCapsule(dst, cx-82*u, bodyY+8*u, leftArmX-4*u, leftArmY+7*u, 13*u, color.NRGBA{R: 255, G: 255, B: 249, A: 224})
	drawFilledEllipseRGBA(dst, leftArmX-12*u, leftArmY+8*u, 15*u, 15*u, color.NRGBA{R: 255, G: 210, B: 70, A: 238})
	drawCapsule(dst, cx+83*u, bodyY+6*u, rightArmX+2*u, rightArmY, 13*u, color.NRGBA{R: 255, G: 255, B: 249, A: 230})
	drawFilledEllipseRGBA(dst, rightArmX+12*u, rightArmY-4*u, 17*u, 17*u, color.NRGBA{R: 255, G: 210, B: 70, A: 244})
	if wave > 0.32 {
		drawBoboSpark(dst, rightArmX+42*u, rightArmY-34*u, 10*u*(0.8+wave*0.5), color.NRGBA{R: 255, G: 216, B: 77, A: uint8(145 + 75*wave)})
		drawBoboSpark(dst, rightArmX+18*u, rightArmY-58*u, 7*u*(0.8+wave*0.4), color.NRGBA{R: 74, G: 188, B: 255, A: uint8(110 + 75*wave)})
	}
	drawFilledEllipseRGBA(dst, cx, bodyY+82*u, 38*u, 32*u, color.NRGBA{R: 255, G: 219, B: 94, A: uint8(170 + 40*glow)})
	drawBoboHeart(dst, cx, bodyY+62*u, 18*u*(1+0.08*math.Sin(t*math.Pi*2.4)))

	antennaPulse := uint8(150 + 70*glow)
	drawCapsule(dst, cx+42*u, headY-127*u, cx+64*u+math.Sin(t*math.Pi*1.5)*3*u, headY-164*u, 5*u, color.NRGBA{R: 35, G: 152, B: 126, A: 235})
	drawFilledEllipseRGBA(dst, cx+69*u, headY-169*u, 13*u, 13*u, color.NRGBA{R: 39, G: 176, B: 126, A: antennaPulse})
	drawFilledEllipseRGBA(dst, cx+69*u, headY-169*u, 23*u, 23*u, color.NRGBA{R: 39, G: 176, B: 126, A: 38})

	drawBoboFace(dst, cx, headY, u, lip, motion, glow)
	drawBoboFeet(dst, cx, baseY+bodyFloat, u, glow, motion)
}

func drawBoboCloudShell(dst *image.RGBA, cx, cy, u, bodyScale float64) {
	shadow := color.NRGBA{R: 206, G: 225, B: 236, A: 210}
	fill := color.NRGBA{R: 255, G: 255, B: 249, A: 255}
	drawFilledEllipseRGBA(dst, cx-64*u, cy-26*u, 86*u*bodyScale, 92*u, shadow)
	drawFilledEllipseRGBA(dst, cx+46*u, cy-24*u, 94*u*bodyScale, 98*u, shadow)
	drawFilledEllipseRGBA(dst, cx, cy+26*u, 116*u*bodyScale, 116*u, shadow)
	drawFilledEllipseRGBA(dst, cx-64*u, cy-34*u, 84*u*bodyScale, 90*u, fill)
	drawFilledEllipseRGBA(dst, cx+45*u, cy-34*u, 92*u*bodyScale, 96*u, fill)
	drawFilledEllipseRGBA(dst, cx, cy+14*u, 118*u*bodyScale, 112*u, fill)
	drawFilledEllipseRGBA(dst, cx-90*u, cy+84*u, 34*u, 30*u, color.NRGBA{R: 232, G: 246, B: 250, A: 240})
	drawFilledEllipseRGBA(dst, cx-42*u, cy+102*u, 39*u, 32*u, color.NRGBA{R: 232, G: 246, B: 250, A: 240})
	drawFilledEllipseRGBA(dst, cx+22*u, cy+104*u, 42*u, 32*u, color.NRGBA{R: 232, G: 246, B: 250, A: 240})
	drawFilledEllipseRGBA(dst, cx+86*u, cy+82*u, 34*u, 30*u, color.NRGBA{R: 232, G: 246, B: 250, A: 240})
}

func drawBoboFace(dst *image.RGBA, cx, cy, u float64, lip LipSyncFrame, motion renderMotionState, glow float64) {
	drawFilledRoundedRect(dst, cx-112*u, cy-64*u, cx+112*u, cy+68*u, 54*u, color.NRGBA{R: 31, G: 39, B: 44, A: 255})
	drawFilledRoundedRect(dst, cx-100*u, cy-52*u, cx+100*u, cy+56*u, 45*u, color.NRGBA{R: 18, G: 24, B: 28, A: 255})
	eyeA := uint8(210 + 45*glow)
	if motion.blinkPhase > 0.52 {
		drawCapsule(dst, cx-45*u, cy-6*u, cx-19*u, cy-6*u, 4*u, color.NRGBA{R: 255, G: 211, B: 78, A: eyeA})
		drawCapsule(dst, cx+19*u, cy-6*u, cx+45*u, cy-6*u, 4*u, color.NRGBA{R: 255, G: 211, B: 78, A: eyeA})
	} else {
		drawFilledEllipseRGBA(dst, cx-34*u, cy-10*u, 14*u, 19*u, color.NRGBA{R: 255, G: 214, B: 77, A: eyeA})
		drawFilledEllipseRGBA(dst, cx+34*u, cy-10*u, 14*u, 19*u, color.NRGBA{R: 255, G: 214, B: 77, A: eyeA})
		drawFilledEllipseRGBA(dst, cx-29*u, cy-17*u, 4*u, 5*u, color.NRGBA{R: 255, G: 246, B: 166, A: 215})
		drawFilledEllipseRGBA(dst, cx+39*u, cy-17*u, 4*u, 5*u, color.NRGBA{R: 255, G: 246, B: 166, A: 215})
	}
	open := math.Max(0, math.Min(1, lip.Open))
	if open < 0.08 {
		drawCapsule(dst, cx-24*u, cy+30*u, cx+24*u, cy+30*u+math.Sin(glow)*1.5*u, 4*u, color.NRGBA{R: 255, G: 224, B: 94, A: 220})
		return
	}
	switch lip.Mouth {
	case "mouth_o", "mouth_u":
		drawFilledEllipseRGBA(dst, cx, cy+32*u, (15+8*open)*u, (14+18*open)*u, color.NRGBA{R: 255, G: 218, B: 81, A: 235})
		drawFilledEllipseRGBA(dst, cx, cy+32*u, (7+5*open)*u, (6+9*open)*u, color.NRGBA{R: 32, G: 23, B: 29, A: 230})
	default:
		drawFilledRoundedRect(dst, cx-(20+15*open)*u, cy+(24-2*open)*u, cx+(20+15*open)*u, cy+(34+18*open)*u, 8*u, color.NRGBA{R: 255, G: 218, B: 81, A: 235})
		drawFilledRoundedRect(dst, cx-(12+9*open)*u, cy+(29-2*open)*u, cx+(12+9*open)*u, cy+(31+10*open)*u, 5*u, color.NRGBA{R: 34, G: 24, B: 28, A: 210})
	}
}

func drawBoboFeet(dst *image.RGBA, cx, baseY, u, glow float64, motion renderMotionState) {
	footA := uint8(130 + 75*glow)
	drawFilledEllipseRGBA(dst, cx-54*u, baseY-10*u, 35*u, 15*u, color.NRGBA{R: 95, G: 173, B: 255, A: 52})
	drawFilledEllipseRGBA(dst, cx+54*u, baseY-10*u, 35*u, 15*u, color.NRGBA{R: 255, G: 202, B: 82, A: 52})
	drawFilledEllipseRGBA(dst, cx-52*u, baseY-22*u+motion.leftFootDy*u, 28*u, 18*u, color.NRGBA{R: 96, G: 164, B: 246, A: footA})
	drawFilledEllipseRGBA(dst, cx+52*u, baseY-22*u+motion.rightFootDy*u, 28*u, 18*u, color.NRGBA{R: 255, G: 205, B: 76, A: footA})
}

func drawBoboHeart(dst *image.RGBA, cx, cy, r float64) {
	c := color.NRGBA{R: 255, G: 172, B: 64, A: 235}
	drawFilledEllipseRGBA(dst, cx-r*0.45, cy-r*0.2, r*0.46, r*0.46, c)
	drawFilledEllipseRGBA(dst, cx+r*0.45, cy-r*0.2, r*0.46, r*0.46, c)
	drawFilledPolygon(dst, []pointF{{cx - r, cy - r*0.05}, {cx + r, cy - r*0.05}, {cx, cy + r*1.08}}, c)
}

func drawBoboSpark(dst *image.RGBA, cx, cy, r float64, c color.NRGBA) {
	points := make([]pointF, 0, 8)
	for i := 0; i < 8; i++ {
		a := -math.Pi/2 + float64(i)*math.Pi/4
		radius := r
		if i%2 == 1 {
			radius = r * 0.42
		}
		points = append(points, pointF{x: cx + math.Cos(a)*radius, y: cy + math.Sin(a)*radius})
	}
	drawFilledPolygon(dst, points, c)
}

func renderAsterFrame(dst *image.RGBA, input LocalIpTalkingAvatarRenderInput, asset *CharacterAsset, lip LipSyncFrame, motion renderMotionState, t float64) {
	cx, baseY, u := puppetPose(input, asset, 0.78)
	cx += motion.bodyDx * u
	bodyFloat := math.Sin(t*math.Pi*0.72)*3.8*u + motion.bodyDy*u*0.45
	headNod := motion.headDy * u * 0.7
	gesture := motion.gesturePhase
	if input.InteractionLevel == "expressive" {
		gesture = math.Max(gesture, math.Max(0, math.Sin(t*math.Pi*0.42))*0.22)
	}
	bodyY := baseY - 250*u + bodyFloat
	headY := baseY - 472*u + bodyFloat + headNod
	pulse := 0.5 + 0.5*math.Sin(t*math.Pi*0.95)

	drawFilledEllipseRGBA(dst, cx, baseY-28*u, 156*u, 31*u, color.NRGBA{R: 34, G: 49, B: 92, A: 48})
	drawAsterCapeAndRobe(dst, cx, bodyY, u, pulse)
	drawAsterArms(dst, cx, bodyY, u, gesture, motion)
	drawAsterHead(dst, cx, headY, u, lip, motion, pulse)
	drawAsterOrb(dst, cx, headY-166*u+math.Sin(t*math.Pi*0.9)*8*u, u, t, pulse)
}

func drawAsterCapeAndRobe(dst *image.RGBA, cx, cy, u, pulse float64) {
	drawFilledPolygon(dst, []pointF{
		{cx - 122*u, cy - 72*u},
		{cx + 122*u, cy - 72*u},
		{cx + 172*u, cy + 230*u},
		{cx - 172*u, cy + 230*u},
	}, color.NRGBA{R: 23, G: 31, B: 58, A: 238})
	drawFilledPolygon(dst, []pointF{
		{cx - 78*u, cy - 94*u},
		{cx + 78*u, cy - 94*u},
		{cx + 108*u, cy + 216*u},
		{cx - 108*u, cy + 216*u},
	}, color.NRGBA{R: 246, G: 250, B: 248, A: 250})
	drawCapsule(dst, cx-94*u, cy-46*u, cx-138*u, cy+204*u, 9*u, color.NRGBA{R: 232, G: 179, B: 79, A: 230})
	drawCapsule(dst, cx+94*u, cy-46*u, cx+138*u, cy+204*u, 9*u, color.NRGBA{R: 232, G: 179, B: 79, A: 230})
	drawFilledEllipseRGBA(dst, cx, cy+20*u, 42*u, 42*u, color.NRGBA{R: 38, G: 57, B: 86, A: 255})
	drawFilledEllipseRGBA(dst, cx, cy+20*u, 22*u, 22*u, color.NRGBA{R: 80, G: 180, B: 255, A: uint8(140 + 85*pulse)})
	drawCapsule(dst, cx-18*u, cy+18*u, cx+18*u, cy+18*u, 3*u, color.NRGBA{R: 242, G: 204, B: 105, A: 220})
	drawCapsule(dst, cx, cy+2*u, cx, cy+36*u, 3*u, color.NRGBA{R: 242, G: 204, B: 105, A: 220})
}

func drawAsterArms(dst *image.RGBA, cx, cy, u, gesture float64, motion renderMotionState) {
	leftHandX := cx - 134*u + motion.leftArmDx*u
	leftHandY := cy + 30*u + motion.leftArmDy*u
	rightHandX := cx + 126*u + gesture*34*u + motion.rightArmDx*u
	rightHandY := cy + 28*u - gesture*52*u + motion.rightArmDy*u
	drawCapsule(dst, cx-70*u, cy-48*u, leftHandX, leftHandY, 16*u, color.NRGBA{R: 238, G: 243, B: 245, A: 245})
	drawFilledRoundedRect(dst, leftHandX-46*u, leftHandY-32*u, leftHandX+28*u, leftHandY+24*u, 8*u, color.NRGBA{R: 52, G: 89, B: 122, A: 145})
	drawStrokeRoundedRect(dst, leftHandX-46*u, leftHandY-32*u, leftHandX+28*u, leftHandY+24*u, 8*u, 3*u, color.NRGBA{R: 117, G: 198, B: 255, A: 170})
	drawCapsule(dst, cx+70*u, cy-48*u, rightHandX, rightHandY, 16*u, color.NRGBA{R: 238, G: 243, B: 245, A: 245})
	drawFilledEllipseRGBA(dst, rightHandX+7*u, rightHandY-4*u, 17*u, 17*u, color.NRGBA{R: 246, G: 250, B: 248, A: 250})
	drawCapsule(dst, rightHandX+14*u, rightHandY-8*u, rightHandX+46*u, rightHandY-34*u, 5*u, color.NRGBA{R: 246, G: 250, B: 248, A: 250})
}

func drawAsterHead(dst *image.RGBA, cx, cy, u float64, lip LipSyncFrame, motion renderMotionState, pulse float64) {
	drawFilledPolygon(dst, []pointF{
		{cx - 96*u, cy - 72*u},
		{cx, cy - 138*u},
		{cx + 96*u, cy - 72*u},
		{cx + 82*u, cy + 88*u},
		{cx - 82*u, cy + 88*u},
	}, color.NRGBA{R: 239, G: 244, B: 244, A: 255})
	drawStrokePolygon(dst, []pointF{
		{cx - 96*u, cy - 72*u},
		{cx, cy - 138*u},
		{cx + 96*u, cy - 72*u},
		{cx + 82*u, cy + 88*u},
		{cx - 82*u, cy + 88*u},
	}, 5*u, color.NRGBA{R: 222, G: 181, B: 92, A: 230})
	drawFilledRoundedRect(dst, cx-58*u, cy-58*u, cx+58*u, cy+50*u, 22*u, color.NRGBA{R: 18, G: 27, B: 39, A: 255})
	if motion.blinkPhase > 0.54 {
		drawCapsule(dst, cx-21*u, cy-7*u, cx+9*u, cy-7*u, 4*u, color.NRGBA{R: 91, G: 203, B: 255, A: 220})
	} else {
		drawFilledEllipseRGBA(dst, cx-8*u, cy-10*u, 16*u, 22*u, color.NRGBA{R: 80, G: 203, B: 255, A: uint8(180 + 65*pulse)})
		drawFilledEllipseRGBA(dst, cx-4*u, cy-16*u, 5*u, 7*u, color.NRGBA{R: 215, G: 245, B: 255, A: 235})
	}
	drawStrokeEllipse(dst, cx+42*u, cy-8*u, 28*u, 28*u, 4*u, color.NRGBA{R: 229, G: 185, B: 88, A: 230})
	drawCapsule(dst, cx+63*u, cy+12*u, cx+90*u, cy+40*u, 5*u, color.NRGBA{R: 229, G: 185, B: 88, A: 220})
	drawCapsule(dst, cx+25*u, cy-42*u, cx+62*u, cy-54*u, 3*u, color.NRGBA{R: 127, G: 219, B: 255, A: uint8(105 + 80*pulse)})

	open := math.Max(0, math.Min(1, lip.Open))
	if open < 0.08 {
		drawCapsule(dst, cx-19*u, cy+27*u, cx+20*u, cy+27*u, 3*u, color.NRGBA{R: 92, G: 210, B: 255, A: 180})
	} else {
		drawFilledRoundedRect(dst, cx-(16+10*open)*u, cy+(20-1*open)*u, cx+(16+10*open)*u, cy+(25+13*open)*u, 5*u, color.NRGBA{R: 87, G: 210, B: 255, A: 190})
	}
}

func drawAsterOrb(dst *image.RGBA, cx, cy, u, t, pulse float64) {
	size := (34 + 5*pulse) * u
	tilt := math.Sin(t*math.Pi*0.65) * 10 * u
	points := []pointF{
		{cx, cy - size},
		{cx + size*0.82, cy + tilt},
		{cx, cy + size},
		{cx - size*0.82, cy - tilt},
	}
	drawFilledPolygon(dst, points, color.NRGBA{R: 64, G: 147, B: 255, A: uint8(82 + 82*pulse)})
	drawStrokePolygon(dst, points, 4*u, color.NRGBA{R: 148, G: 222, B: 255, A: 205})
	drawCapsule(dst, cx-size*0.62, cy, cx+size*0.62, cy, 2*u, color.NRGBA{R: 226, G: 246, B: 255, A: 170})
	drawCapsule(dst, cx, cy-size*0.62, cx, cy+size*0.62, 2*u, color.NRGBA{R: 226, G: 246, B: 255, A: 170})
}

type pointF struct {
	x float64
	y float64
}

func drawFilledEllipseRGBA(dst *image.RGBA, cx, cy, rx, ry float64, c color.NRGBA) {
	if rx <= 0 || ry <= 0 || c.A == 0 {
		return
	}
	minX := int(math.Floor(cx - rx))
	maxX := int(math.Ceil(cx + rx))
	minY := int(math.Floor(cy - ry))
	maxY := int(math.Ceil(cy + ry))
	for y := minY; y <= maxY; y++ {
		if y < dst.Bounds().Min.Y || y >= dst.Bounds().Max.Y {
			continue
		}
		for x := minX; x <= maxX; x++ {
			if x < dst.Bounds().Min.X || x >= dst.Bounds().Max.X {
				continue
			}
			dx := (float64(x) + 0.5 - cx) / rx
			dy := (float64(y) + 0.5 - cy) / ry
			if dx*dx+dy*dy <= 1 {
				blendPixel(dst, x, y, c)
			}
		}
	}
}

func drawStrokeEllipse(dst *image.RGBA, cx, cy, rx, ry, thickness float64, c color.NRGBA) {
	if thickness <= 0 {
		return
	}
	steps := int(math.Max(48, (rx+ry)*1.4))
	for i := 0; i < steps; i++ {
		a := float64(i) / float64(steps) * math.Pi * 2
		drawFilledEllipseRGBA(dst, cx+math.Cos(a)*rx, cy+math.Sin(a)*ry, thickness*0.5, thickness*0.5, c)
	}
}

func drawFilledRoundedRect(dst *image.RGBA, x0, y0, x1, y1, r float64, c color.NRGBA) {
	if x1 < x0 {
		x0, x1 = x1, x0
	}
	if y1 < y0 {
		y0, y1 = y1, y0
	}
	r = math.Min(r, math.Min((x1-x0)/2, (y1-y0)/2))
	drawFilledRectRGBA(dst, x0+r, y0, x1-r, y1, c)
	drawFilledRectRGBA(dst, x0, y0+r, x1, y1-r, c)
	drawFilledEllipseRGBA(dst, x0+r, y0+r, r, r, c)
	drawFilledEllipseRGBA(dst, x1-r, y0+r, r, r, c)
	drawFilledEllipseRGBA(dst, x0+r, y1-r, r, r, c)
	drawFilledEllipseRGBA(dst, x1-r, y1-r, r, r, c)
}

func drawStrokeRoundedRect(dst *image.RGBA, x0, y0, x1, y1, r, thickness float64, c color.NRGBA) {
	drawCapsule(dst, x0+r, y0, x1-r, y0, thickness, c)
	drawCapsule(dst, x0+r, y1, x1-r, y1, thickness, c)
	drawCapsule(dst, x0, y0+r, x0, y1-r, thickness, c)
	drawCapsule(dst, x1, y0+r, x1, y1-r, thickness, c)
	drawStrokeEllipseArc(dst, x0+r, y0+r, r, r, thickness, math.Pi, math.Pi*1.5, c)
	drawStrokeEllipseArc(dst, x1-r, y0+r, r, r, thickness, math.Pi*1.5, math.Pi*2, c)
	drawStrokeEllipseArc(dst, x1-r, y1-r, r, r, thickness, 0, math.Pi*0.5, c)
	drawStrokeEllipseArc(dst, x0+r, y1-r, r, r, thickness, math.Pi*0.5, math.Pi, c)
}

func drawStrokeEllipseArc(dst *image.RGBA, cx, cy, rx, ry, thickness, start, end float64, c color.NRGBA) {
	steps := int(math.Max(12, (rx+ry)*0.35))
	for i := 0; i <= steps; i++ {
		a := start + (end-start)*float64(i)/float64(steps)
		drawFilledEllipseRGBA(dst, cx+math.Cos(a)*rx, cy+math.Sin(a)*ry, thickness*0.5, thickness*0.5, c)
	}
}

func drawFilledRectRGBA(dst *image.RGBA, x0, y0, x1, y1 float64, c color.NRGBA) {
	minX := int(math.Floor(x0))
	maxX := int(math.Ceil(x1))
	minY := int(math.Floor(y0))
	maxY := int(math.Ceil(y1))
	for y := minY; y <= maxY; y++ {
		if y < dst.Bounds().Min.Y || y >= dst.Bounds().Max.Y {
			continue
		}
		for x := minX; x <= maxX; x++ {
			if x < dst.Bounds().Min.X || x >= dst.Bounds().Max.X {
				continue
			}
			blendPixel(dst, x, y, c)
		}
	}
}

func drawCapsule(dst *image.RGBA, x0, y0, x1, y1, radius float64, c color.NRGBA) {
	dx := x1 - x0
	dy := y1 - y0
	dist := math.Hypot(dx, dy)
	steps := int(math.Max(1, dist/(radius*0.5)))
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		drawFilledEllipseRGBA(dst, x0+dx*t, y0+dy*t, radius, radius, c)
	}
}

func drawFilledPolygon(dst *image.RGBA, points []pointF, c color.NRGBA) {
	if len(points) < 3 || c.A == 0 {
		return
	}
	minX, maxX, minY, maxY := polygonBounds(points)
	for y := int(math.Floor(minY)); y <= int(math.Ceil(maxY)); y++ {
		if y < dst.Bounds().Min.Y || y >= dst.Bounds().Max.Y {
			continue
		}
		for x := int(math.Floor(minX)); x <= int(math.Ceil(maxX)); x++ {
			if x < dst.Bounds().Min.X || x >= dst.Bounds().Max.X {
				continue
			}
			if pointInPolygon(float64(x)+0.5, float64(y)+0.5, points) {
				blendPixel(dst, x, y, c)
			}
		}
	}
}

func drawStrokePolygon(dst *image.RGBA, points []pointF, thickness float64, c color.NRGBA) {
	if len(points) < 2 {
		return
	}
	for i := range points {
		next := (i + 1) % len(points)
		drawCapsule(dst, points[i].x, points[i].y, points[next].x, points[next].y, thickness*0.5, c)
	}
}

func polygonBounds(points []pointF) (float64, float64, float64, float64) {
	minX, maxX := points[0].x, points[0].x
	minY, maxY := points[0].y, points[0].y
	for _, p := range points[1:] {
		minX = math.Min(minX, p.x)
		maxX = math.Max(maxX, p.x)
		minY = math.Min(minY, p.y)
		maxY = math.Max(maxY, p.y)
	}
	return minX, maxX, minY, maxY
}

func pointInPolygon(x, y float64, points []pointF) bool {
	inside := false
	j := len(points) - 1
	for i := range points {
		xi, yi := points[i].x, points[i].y
		xj, yj := points[j].x, points[j].y
		intersects := ((yi > y) != (yj > y)) && (x < (xj-xi)*(y-yi)/(yj-yi+1e-9)+xi)
		if intersects {
			inside = !inside
		}
		j = i
	}
	return inside
}
