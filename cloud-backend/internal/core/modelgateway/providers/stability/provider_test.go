package stability

import "testing"

func TestIsImageContentTypeAcceptsParameters(t *testing.T) {
	if !isImageContentType("image/png; charset=binary") {
		t.Fatal("image content type with parameters should be accepted")
	}
	if isImageContentType("application/json; charset=utf-8") {
		t.Fatal("json content type should not be accepted as image")
	}
}
