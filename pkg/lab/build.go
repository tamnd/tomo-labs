package lab

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tamnd/tomo-labs/pkg/container"
)

// Build builds the base image, the trace proxy, and each tool image. An empty
// only builds every tool; a name builds just that one. The proxy is built from
// the module root so it can pull in its Go packages; everything else builds from
// its own directory.
//
// Every image it retags leaves the previous copy behind as a dangling <none>
// layer, so it prunes those once the build finishes. A build is a natural
// checkpoint for that: it is the only thing that creates dangling images, and it
// runs off the hot path of a scored run.
func (l *Lab) Build(ctx context.Context, only string) error {
	defer l.rt.PruneImages(ctx)
	fmt.Fprintln(os.Stderr, "[build] base image ("+baseImage+")")
	if err := l.rt.Build(ctx, container.BuildSpec{
		Tag: baseImage, Context: filepath.Join(l.cfg.Root, "tools", "base"), Out: os.Stderr,
	}); err != nil {
		return err
	}

	fmt.Fprintln(os.Stderr, "[build] trace proxy ("+proxyImage+")")
	if err := l.rt.Build(ctx, container.BuildSpec{
		Tag:        proxyImage,
		Context:    l.cfg.Root,
		Dockerfile: filepath.Join(l.cfg.Root, "proxy", "Dockerfile"),
		Out:        os.Stderr,
	}); err != nil {
		return err
	}

	tools, err := l.Tools()
	if err != nil {
		return err
	}
	for _, t := range tools {
		if only != "" && t != only {
			continue
		}
		fmt.Fprintln(os.Stderr, "[build] tool image ("+toolPrefix+t+")")
		if err := l.rt.Build(ctx, container.BuildSpec{
			Tag: toolPrefix + t, Context: filepath.Join(l.cfg.Root, "tools", t), Out: os.Stderr,
		}); err != nil {
			return err
		}
		if err := l.captureImageSize(ctx, t); err != nil {
			return err
		}
	}
	return nil
}

// imageSizes records what a tool costs to install: the whole image, and the
// slice that is the tool itself sitting on top of the shared base. Install
// footprint is a real axis of the comparison, so it is saved next to the runs
// and read back by the report.
type imageSizes struct {
	Tool         string `json:"tool"`
	ImageBytes   int64  `json:"image_bytes"`
	BaseBytes    int64  `json:"base_bytes"`
	InstallBytes int64  `json:"install_bytes"`
}

func (l *Lab) captureImageSize(ctx context.Context, tool string) error {
	toolBytes := l.rt.ImageSize(ctx, toolPrefix+tool)
	if toolBytes == 0 {
		return nil
	}
	base := l.rt.ImageSize(ctx, baseImage)
	sizes := imageSizes{
		Tool:         tool,
		ImageBytes:   toolBytes,
		BaseBytes:    base,
		InstallBytes: toolBytes - base,
	}
	dir := filepath.Join(l.cfg.Data, tool)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(sizes, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "image.json"), append(b, '\n'), 0o644)
}

// imageKB reads a tool's saved install and image footprints in kbytes, or zeros
// if the tool was never built through this harness.
func (l *Lab) imageKB(tool string) (installKB, imageKB int) {
	b, err := os.ReadFile(filepath.Join(l.cfg.Data, tool, "image.json"))
	if err != nil {
		return 0, 0
	}
	var s imageSizes
	if json.Unmarshal(b, &s) != nil {
		return 0, 0
	}
	return int(s.InstallBytes / 1024), int(s.ImageBytes / 1024)
}
