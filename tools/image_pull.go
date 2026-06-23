package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tidwall/gjson"
)

type ImagePullTool struct {
	r *Registry
}

func NewImagePullTool(r *Registry) *ImagePullTool {
	return &ImagePullTool{r: r}
}

func (t *ImagePullTool) Name() string {
	return "image_pull"
}

func (t *ImagePullTool) Description() string {
	return "Pull a Docker image from registry."
}

func (t *ImagePullTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"image": {
				"type": "string",
				"description": "The Docker image name and tag to pull, e.g. mysql:8.0"
			}
		},
		"required": ["image"]
	}`)
}

func (t *ImagePullTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	parsed := gjson.ParseBytes(args)
	imageName := parsed.Get("image").String()
	if imageName == "" {
		return "", fmt.Errorf("missing required parameter: image")
	}

	size, err := t.r.Docker.PullImage(ctx, imageName)
	if err != nil {
		return "", err
	}
	sizeStr := fmt.Sprintf("%d B", size)
	if size > 1024*1024*1024 {
		sizeStr = fmt.Sprintf("%.2f GB", float64(size)/(1024*1024*1024))
	} else if size > 1024*1024 {
		sizeStr = fmt.Sprintf("%.2f MB", float64(size)/(1024*1024))
	}

	return fmt.Sprintf("pulled %s (%s)", imageName, sizeStr), nil
}
