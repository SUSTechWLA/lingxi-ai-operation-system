package localtool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalIpTalkingAvatarProjectPromoDemo(t *testing.T) {
	if os.Getenv("TANGYING_RUN_IP_AVATAR_PROMO_DEMO") != "1" {
		t.Skip("set TANGYING_RUN_IP_AVATAR_PROMO_DEMO=1 to generate the project promo demo")
	}
	requireFFmpegForAvatarTest(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(cwd)))
	audioPath := os.Getenv("TANGYING_IP_AVATAR_PROMO_AUDIO")
	outputDir := os.Getenv("TANGYING_IP_AVATAR_PROMO_OUTPUT_DIR")
	if outputDir == "" {
		outputDir = filepath.Join(repoRoot, "tmp", "ip_talking_avatar_project_promo")
	}
	if !filepath.IsAbs(outputDir) {
		outputDir = filepath.Join(repoRoot, outputDir)
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	script := os.Getenv("TANGYING_IP_AVATAR_PROMO_SCRIPT")
	if script == "" {
		script = "大家好，我是波波。欢迎来到躺营 AI 视频创作助手。这个开源项目把脚本、分镜、素材、口播、IP 数字人和最终合成串成一条清楚的创作流程。重点是，口播类视频可以用本地 IP 数字人降低拍摄成本，让角色跟着内容说话、点头、挥手、展示重点。影视类视频也可以继续服务 AIGC 镜头创作。所以，从想法到成片，用户可以一步一步审核、修改和预览。"
	}
	subtitlePath := filepath.Join(outputDir, "promo.srt")
	if err := os.WriteFile(subtitlePath, []byte("1\n00:00:00,000 --> 00:00:08,000\n"+script+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	backgroundPath := writeDemoBackground(t, filepath.Join(outputDir, "promo_background.png"))

	characterID := os.Getenv("TANGYING_IP_AVATAR_PROMO_CHARACTER")
	if characterID == "" {
		characterID = "bobo"
	}
	renderMode := os.Getenv("TANGYING_IP_AVATAR_PROMO_RENDER_MODE")
	if renderMode == "" {
		renderMode = "svg2d"
	}
	output, err := NewLocalIpTalkingAvatarRenderTool(repoRoot).Execute(context.Background(), LocalIpTalkingAvatarRenderInput{
		CharacterID:      characterID,
		Script:           script,
		AudioPath:        audioPath,
		SubtitlePath:     subtitlePath,
		BackgroundPath:   backgroundPath,
		OutputDir:        outputDir,
		RenderMode:       renderMode,
		InteractionLevel: "expressive",
		Resolution:       Resolution{Width: 1280, Height: 720},
		FPS:              24,
		Style: RenderStyle{
			Position:               "center_bottom",
			Scale:                  1,
			SubtitleEnabled:        true,
			BackgroundEnabled:      true,
			TransparentAvatarVideo: true,
		},
		MotionPolicy: defaultMotionPolicy(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !output.Success || !output.QA.FinalVideoGenerated {
		t.Fatalf("promo demo did not produce final video: %#v", output)
	}
	fmt.Printf("PROMO_VIDEO_PATH=%s\n", output.VideoPath)
	fmt.Printf("PROMO_OUTPUT_DIR=%s\n", outputDir)
}
