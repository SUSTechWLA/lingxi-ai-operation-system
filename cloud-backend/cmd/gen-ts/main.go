package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tangying-ai/aios-core/internal/core/apispec"
)

func main() {
	output := flag.String("output", filepath.Join("..", "frontend", "src", "utils", "api-types.generated.ts"), "generated TypeScript output")
	check := flag.Bool("check", false, "fail when the generated file is stale")
	flag.Parse()

	generated := apispec.RenderTypeScript(apispec.BuildCloudSpec())
	if *check {
		current, err := os.ReadFile(*output)
		if err != nil {
			fatalf("read %s: %v", *output, err)
		}
		if !bytes.Equal(current, generated) {
			fatalf("%s is stale; run make gen-ts", *output)
		}
		return
	}
	if err := os.WriteFile(*output, generated, 0o644); err != nil {
		fatalf("write %s: %v", *output, err)
	}
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
