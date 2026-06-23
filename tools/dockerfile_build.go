package tools

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/tidwall/gjson"
)

type DockerfileBuildTool struct{ r *Registry }

func NewDockerfileBuildTool(r *Registry) *DockerfileBuildTool { return &DockerfileBuildTool{r: r} }
func (t *DockerfileBuildTool) Name() string                   { return "image_build" }
func (t *DockerfileBuildTool) Description() string {
	return "Build a Docker image from a Dockerfile."
}
func (t *DockerfileBuildTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "Directory path containing the Dockerfile"},
			"tag":  {"type": "string", "description": "Image tag to assign, e.g. myapp:latest"}
		},
		"required": ["path", "tag"]
	}`)
}

func (t *DockerfileBuildTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	parsed := gjson.ParseBytes(args)
	path := parsed.Get("path").String()
	tag := parsed.Get("tag").String()
	if path == "" || tag == "" {
		return "", fmt.Errorf("missing required parameters: path and tag")
	}
	buf, err := tarDirectory(path)
	if err != nil {
		return "", fmt.Errorf("failed to package build context: %w", err)
	}

	var output bytes.Buffer
	if err := t.r.Docker.BuildImage(ctx, buf, tag, &output); err != nil {
		return "", fmt.Errorf("build failed: %w", err)
	}

	result := output.String()
	if len(result) > 2000 {
		result = result[len(result)-2000:]
	}
	return fmt.Sprintf("built %s\n%s", tag, result), nil
}
func tarDirectory(srcDir string) (io.Reader, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	defer tw.Close()

	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		hdr := &tar.Header{
			Name:    filepath.ToSlash(rel),
			Mode:    int64(info.Mode()),
			ModTime: info.ModTime(),
			Size:    info.Size(),
		}
		if info.IsDir() {
			hdr.Typeflag = tar.TypeDir
			hdr.Name += "/"
			return tw.WriteHeader(hdr)
		}
		hdr.Typeflag = tar.TypeReg
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(tw, f)
		return err
	})
	return &buf, err
}
