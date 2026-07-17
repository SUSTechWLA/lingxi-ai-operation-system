package llmutil

import (
	"bytes"
	"image"
	_ "image/gif" // register GIF decoder
	"image/jpeg"
	_ "image/png" // register PNG decoder

	"go.uber.org/zap"
)

const (
	maxImageDim   = 720
	jpegQuality   = 75
	maxImageBytes = 512 * 1024 // 512KB target max after compression
)

// CompressImageBytes resizes and compresses an image to reduce token usage for multimodal LLMs.
// Rules:
//   - If the longest side > maxImageDim, resize proportionally
//   - Always re-encode as JPEG (quality 75) for consistency
//   - If the resulting JPEG is still > maxImageBytes, reduce quality further
//   - PNG input is converted to JPEG
//
// Returns the compressed JPEG bytes and the MIME type "image/jpeg".
func CompressImageBytes(data []byte) ([]byte, string) {
	if len(data) == 0 {
		return data, "image/jpeg"
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		zap.L().Warn("failed to decode image for compression, using original", zap.Error(err))
		return data, "image/jpeg"
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	origW, origH := w, h

	// Resize if needed
	if w > maxImageDim || h > maxImageDim {
		scale := float64(maxImageDim) / float64(max(w, h))
		w = int(float64(w) * scale)
		h = int(float64(h) * scale)
		if w < 1 {
			w = 1
		}
		if h < 1 {
			h = 1
		}
		img = resizeNearest(img, w, h)
		zap.L().Debug("image resized",
			zap.Int("origW", origW), zap.Int("origH", origH),
			zap.Int("newW", w), zap.Int("newH", h))
	}

	// Encode as JPEG with quality setting
	quality := jpegQuality
	for {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
			zap.L().Warn("failed to encode JPEG, using original", zap.Error(err))
			return data, "image/jpeg"
		}
		if buf.Len() <= maxImageBytes || quality <= 30 {
			zap.L().Debug("image compressed",
				zap.Int("origBytes", len(data)),
				zap.Int("compressedBytes", buf.Len()),
				zap.Int("quality", quality))
			return buf.Bytes(), "image/jpeg"
		}
		quality -= 15
	}
}

// resizeNearest performs a simple nearest-neighbor resize using the standard library.
func resizeNearest(src image.Image, newW, newH int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	srcBounds := src.Bounds()
	srcW, srcH := srcBounds.Dx(), srcBounds.Dy()

	for y := 0; y < newH; y++ {
		srcY := y * srcH / newH
		for x := 0; x < newW; x++ {
			srcX := x * srcW / newW
			dst.Set(x, y, src.At(srcBounds.Min.X+srcX, srcBounds.Min.Y+srcY))
		}
	}
	return dst
}
