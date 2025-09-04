// utils.go
package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// readTextFileContent 智能读取文本文件内容，自动处理UTF-8和GBK编码
// 返回的内容保证是UTF-8编码的字符串。
func readTextFileContent(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	// 1. 检查 UTF-8 BOM
	if bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
		log.Printf("  -> Detected UTF-8 with BOM for %s", filepath.Base(path))
		return string(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})), nil
	}

	// 2. 验证是否为有效的 UTF-8
	if utf8.Valid(data) {
		log.Printf("  -> Detected valid UTF-8 (No BOM) for %s", filepath.Base(path))
		return string(data), nil
	}

	// 3. 回退到 GBK
	log.Printf("  -> Not valid UTF-8. Assuming GBK for %s", filepath.Base(path))
	gbkReader := transform.NewReader(bytes.NewReader(data), simplifiedchinese.GBK.NewDecoder())

	decodedData, err := io.ReadAll(gbkReader)
	if err != nil {
		return "", fmt.Errorf("failed to decode %s as GBK: %w", filepath.Base(path), err)
	}

	return string(decodedData), nil
}

// sanitizeFilename 清理字符串以用作安全的文件名
func sanitizeFilename(name string) string {
	re := regexp.MustCompile(`[\\/:*?"<>|]`)
	return re.ReplaceAllString(name, "-")
}

// formatDurationToFFmpegTime 将 time.Duration 格式化为 FFmpeg 的 HH:MM:SS.ms 格式
func formatDurationToFFmpegTime(d time.Duration) string {
	d = d.Round(time.Millisecond)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second
	d -= s * time.Second
	ms := d / time.Millisecond
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, ms)
}
