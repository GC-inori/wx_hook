package gateway

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"wxbot/pkg/protocol"
)

const (
	mediaResolveTimeout = 15 * time.Second
	maxResolvedMedia    = 50 << 20
)

var mediaPathFields = []string{"img_path", "file_path", "path", "image", "img", "file"}

// resolveMediaPayload downloads or decodes remote media references in supported
// action payload fields and replaces them with files under mediaDir.
func resolveMediaPayload(ctx context.Context, action string, raw json.RawMessage, mediaDir string) (json.RawMessage, error) {
	if canonical, ok := protocol.Canonical(action); ok {
		action = canonical
	}
	if action != "sendImage" && action != "sendFile" {
		return raw, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("decode media action data: %w", err)
	}
	changed := false
	for _, key := range mediaPathFields {
		field, ok := fields[key]
		if !ok {
			continue
		}
		var value string
		if json.Unmarshal(field, &value) != nil {
			continue
		}
		resolved, isRemote, err := resolveMediaReference(ctx, value, mediaDir)
		if err != nil {
			return nil, fmt.Errorf("resolve media field %q: %w", key, err)
		}
		if isRemote {
			encoded, err := json.Marshal(resolved)
			if err != nil {
				return nil, fmt.Errorf("encode resolved media path: %w", err)
			}
			fields[key] = encoded
			changed = true
		}
	}
	if !changed {
		return raw, nil
	}
	resolved, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("encode resolved media action data: %w", err)
	}
	return resolved, nil
}

func resolveMediaReference(parent context.Context, value, mediaDir string) (string, bool, error) {
	lowerValue := strings.ToLower(value)
	switch {
	case strings.HasPrefix(lowerValue, "http://"), strings.HasPrefix(lowerValue, "https://"):
		path, err := downloadMedia(parent, value, mediaDir)
		return path, true, err
	case strings.HasPrefix(lowerValue, "base64://"), strings.HasPrefix(lowerValue, "data:"):
		path, err := decodeMedia(value, mediaDir)
		return path, true, err
	default:
		return value, false, nil
	}
}

func downloadMedia(parent context.Context, source, mediaDir string) (string, error) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, mediaResolveTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return "", fmt.Errorf("invalid media URL: %w", err)
	}
	response, err := (&http.Client{Timeout: mediaResolveTimeout}).Do(request)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("download returned HTTP %d", response.StatusCode)
	}
	if response.ContentLength > maxResolvedMedia {
		return "", fmt.Errorf("media exceeds the %d-byte limit", maxResolvedMedia)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResolvedMedia+1))
	if err != nil {
		return "", fmt.Errorf("read downloaded media: %w", err)
	}
	if int64(len(data)) > maxResolvedMedia {
		return "", fmt.Errorf("media exceeds the %d-byte limit", maxResolvedMedia)
	}
	parsed, err := url.Parse(source)
	if err != nil {
		return "", fmt.Errorf("invalid media URL: %w", err)
	}
	ext := mediaExtension(filepath.Ext(parsed.Path), response.Header.Get("Content-Type"), data)
	return writeResolvedMedia(mediaDir, ext, data)
}

func decodeMedia(value, mediaDir string) (string, error) {
	var encoded, contentType string
	if strings.HasPrefix(strings.ToLower(value), "base64://") {
		encoded = value[len("base64://"):]
	} else {
		comma := strings.IndexByte(value, ',')
		if comma < 0 {
			return "", errors.New("invalid data URI: missing comma")
		}
		metadata := value[len("data:"):comma]
		isBase64 := false
		for _, part := range strings.Split(metadata, ";") {
			if strings.EqualFold(part, "base64") {
				isBase64 = true
				continue
			}
			if strings.Contains(part, "/") {
				contentType = part
			}
		}
		if !isBase64 {
			return "", errors.New("data URI is not base64 encoded")
		}
		encoded = value[comma+1:]
	}
	if int64(len(encoded)) > ((maxResolvedMedia+2)/3)*4+8 {
		return "", fmt.Errorf("media exceeds the %d-byte limit", maxResolvedMedia)
	}
	data, err := decodeBase64(encoded)
	if err != nil {
		return "", fmt.Errorf("invalid base64 media: %w", err)
	}
	if int64(len(data)) > maxResolvedMedia {
		return "", fmt.Errorf("media exceeds the %d-byte limit", maxResolvedMedia)
	}
	ext := mediaExtension("", contentType, data)
	return writeResolvedMedia(mediaDir, ext, data)
}

func decodeBase64(encoded string) ([]byte, error) {
	for _, encoding := range []*base64.Encoding{
		base64.StdEncoding, base64.RawStdEncoding,
		base64.URLEncoding, base64.RawURLEncoding,
	} {
		data, err := encoding.DecodeString(encoded)
		if err == nil {
			return data, nil
		}
	}
	return nil, errors.New("unsupported base64 encoding")
}

func mediaExtension(urlExtension, contentType string, data []byte) string {
	if extension := normalizedMediaExtension(urlExtension); extension != "" && mime.TypeByExtension(extension) != "" {
		return extension
	}
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if mediaType == "" && len(data) != 0 {
		mediaType, _, _ = mime.ParseMediaType(http.DetectContentType(data[:min(len(data), 512)]))
	}
	if extension := canonicalMediaExtension(mediaType); extension != "" {
		return extension
	}
	extensions, _ := mime.ExtensionsByType(mediaType)
	sort.Slice(extensions, func(i, j int) bool {
		if len(extensions[i]) == len(extensions[j]) {
			return extensions[i] < extensions[j]
		}
		return len(extensions[i]) < len(extensions[j])
	})
	for _, extension := range extensions {
		if normalized := normalizedMediaExtension(extension); normalized != "" {
			return normalized
		}
	}
	return ".bin"
}

func canonicalMediaExtension(mediaType string) string {
	switch strings.ToLower(mediaType) {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/bmp":
		return ".bmp"
	case "image/svg+xml":
		return ".svg"
	case "application/pdf":
		return ".pdf"
	case "application/zip":
		return ".zip"
	case "text/plain":
		return ".txt"
	case "text/html":
		return ".html"
	case "application/json":
		return ".json"
	default:
		return ""
	}
}

func normalizedMediaExtension(extension string) string {
	extension = strings.ToLower(strings.TrimSpace(extension))
	if extension == "" || len(extension) > 12 || extension[0] != '.' {
		return ""
	}
	for _, char := range extension[1:] {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') {
			return ""
		}
	}
	return extension
}

func writeResolvedMedia(mediaDir, extension string, data []byte) (string, error) {
	if strings.TrimSpace(mediaDir) == "" {
		mediaDir = filepath.Join("data", "media")
	}
	tmpDir := filepath.Join(mediaDir, "tmp")
	if err := os.MkdirAll(tmpDir, 0750); err != nil {
		return "", fmt.Errorf("create media directory: %w", err)
	}
	file, err := os.CreateTemp(tmpDir, "media-*"+extension)
	if err != nil {
		return "", fmt.Errorf("create media file: %w", err)
	}
	name := file.Name()
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(name)
		return "", fmt.Errorf("write media file: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(name)
		return "", fmt.Errorf("close media file: %w", err)
	}
	return name, nil
}

func (s *Server) mediaDirectory() string {
	cfg := s.currentConfig()
	mediaDir := strings.TrimSpace(cfg.Policy.MediaDir)
	if mediaDir == "" {
		mediaDir = filepath.Join("data", "media")
	}
	if filepath.IsAbs(mediaDir) {
		return filepath.Clean(mediaDir)
	}
	if root := strings.TrimSpace(cfg.RootDir); root != "" {
		return filepath.Join(root, mediaDir)
	}
	return mediaDir
}
