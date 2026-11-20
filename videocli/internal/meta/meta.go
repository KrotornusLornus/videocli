// Package meta extrae metadatos de vídeo con ffprobe.
package meta

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
)

// Info son los metadatos que nos interesan de un vídeo.
type Info struct {
	DurationMS int64
	Width      int
	Height     int
	VideoCodec string
	AudioCodec string
	Title      string
}

type ffprobeOutput struct {
	Format struct {
		Duration  string `json:"duration"`
		Title     string `json:"title"`
		Filename  string `json:"filename"`
		Tags      map[string]string
	} `json:"format"`
	Streams []struct {
		CodecType  string `json:"codec_type"`
		CodecName  string `json:"codec_name"`
		Width      int    `json:"width"`
		Height     int    `json:"height"`
		Disposition struct {
			Default int `json:"default"`
		} `json:"disposition"`
	} `json:"streams"`
}

// Probe devuelve los metadatos de path mediante ffprobe.
func Probe(ctx context.Context, path string) (*Info, error) {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-print_format", "json",
		"-show_format", "-show_streams",
		"--", path)
	var out ffprobeOutput
	data, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("ffprobe salida: %w", err)
	}
	info := &Info{Title: out.Format.Title}
	if info.Title == "" {
		info.Title = out.Format.Tags["title"]
	}
	if d := out.Format.Duration; d != "" {
		if ms, err := durationToMS(d); err == nil {
			info.DurationMS = ms
		}
	}
	for _, s := range out.Streams {
		switch s.CodecType {
		case "video":
			if info.VideoCodec == "" {
				info.VideoCodec = s.CodecName
				info.Width, info.Height = s.Width, s.Height
			}
		case "audio":
			if info.AudioCodec == "" {
				info.AudioCodec = s.CodecName
			}
		}
	}
	return info, nil
}

// durationToMS convierte una duración ffprobe ("12.345", "N/A") a ms.
func durationToMS(d string) (int64, error) {
	if d == "N/A" {
		return 0, fmt.Errorf("duración no disponible")
	}
	f, err := strconv.ParseFloat(d, 64)
	if err != nil {
		return 0, err
	}
	return int64(f * 1000), nil
}
