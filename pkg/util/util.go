package util

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// ReadTextFileContent 智能读取文本文件内容，自动处理UTF-8和GBK编码
// 返回的内容保证是UTF-8编码的字符串。
func ReadTextFileContent(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	if bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
		return string(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})), nil
	}

	if utf8.Valid(data) {
		return string(data), nil
	}

	gbkReader := transform.NewReader(bytes.NewReader(data), simplifiedchinese.GBK.NewDecoder())
	decodedData, err := io.ReadAll(gbkReader)
	if err != nil {
		return "", fmt.Errorf("failed to decode %s as GBK: %w", filepath.Base(path), err)
	}

	return string(decodedData), nil
}

// SanitizeFileName 清理文件名，移除或替换不适用于文件路径的字符
func SanitizeFileName(name string) string {
	// 替换所有斜杠为下划线
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")

	// 移除其他不安全的文件名字符 (Windows/Linux通用不推荐的字符)
	invalidChars := []string{":", "*", "?", "\"", "<", ">", "|"}
	for _, char := range invalidChars {
		name = strings.ReplaceAll(name, char, "")
	}
	// 移除文件名首尾空格和连续空格
	name = strings.TrimSpace(name)
	name = strings.Join(strings.Fields(name), " ") // 将多个空格替换为一个空格
	return name
}

// FormatDurationToFFmpegTime 将 time.Duration 格式化为 FFmpeg 的 HH:MM:SS.ms 格式
func FormatDurationToFFmpegTime(d time.Duration) string {
	d = d.Round(time.Millisecond) // Round to nearest millisecond
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second
	d -= s * time.Second
	ms := d / time.Millisecond
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, ms)
}

// IsDirectory 辅助函数，检查路径是否为目录
func IsDirectory(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// IsRelevantMusicFile 辅助函数，判断文件是否为我们关心的音乐相关文件
func IsRelevantMusicFile(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	name := strings.ToLower(filepath.Base(filePath))
	switch ext {
	case ".wav", ".flac", ".mp3", ".m4a", ".aac", ".ogg", ".ape", ".wv":
		return true
	case ".cue", ".json", ".jpg", ".png": // CUE文件，潜在的json元数据，图片封面
		return true
	default:
		// 特殊文件名，如 Info.txt
		if name == "info.txt" {
			return true
		}
		return false
	}
}
