package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func (cfg apiConfig) ensureAssetsDir() error {
	if _, err := os.Stat(cfg.assetsRoot); os.IsNotExist(err) {
		return os.Mkdir(cfg.assetsRoot, 0755)
	}
	return nil
}

func getAssetPath(mediaType string) string {
	base := make([]byte, 32)
	_, err := rand.Read(base)
	if err != nil {
		panic("failed to generate random bytes")
	}
	id := base64.RawURLEncoding.EncodeToString(base)

	ext := mediaTypeToExt(mediaType)
	return fmt.Sprintf("%s%s", id, ext)
}

func (cfg apiConfig) getObjectURL(key string) string {
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, key)
}

func (cfg apiConfig) getAssetDiskPath(assetPath string) string {
	return filepath.Join(cfg.assetsRoot, assetPath)
}

func (cfg apiConfig) getAssetURL(assetPath string) string {
	return fmt.Sprintf("http://localhost:%s/assets/%s", cfg.port, assetPath)
}

func mediaTypeToExt(mediaType string) string {
	parts := strings.Split(mediaType, "/")
	if len(parts) != 2 {
		return ".bin"
	}
	return "." + parts[1]
}

type ffprobeOut struct {
	Streams []struct {
		CodecType          string            `json:"codec_type"`
		Width              int               `json:"width"`
		Height             int               `json:"height"`
		SampleAspectRatio  string            `json:"sample_aspect_ratio"`  // e.g. "1:1", "4:3"
		DisplayAspectRatio string            `json:"display_aspect_ratio"` // e.g. "16:9"
		Tags               map[string]string `json:"tags"`                 // rotate sometimes here
		SideDataList       []struct {
			Rotation int `json:"rotation"` // sometimes rotation appears here
		} `json:"side_data_list"`
	} `json:"streams"`
}

// - getVideoAspectRatio takes a file path and returns the aspect ratio as a string.
func getVideoAspectRatio(filePath string) (string, error) {
	cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	b := new(bytes.Buffer)
	cmd.Stdout = b
	err := cmd.Run()
	if err != nil {
		return "", err
	}

	var out ffprobeOut
	err = json.Unmarshal(b.Bytes(), &out)
	if err != nil {
		return "", err
	}

	return out.Streams[0].DisplayAspectRatio, nil
}
