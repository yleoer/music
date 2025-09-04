// processor.go
package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ProcessAlbum 调用 FFmpeg 处理整张专辑
func ProcessAlbum(album *Album, targetDir string) error {
	sanitizedArtist := sanitizeFileName(album.Artist)
	sanitizedAlbumTitle := sanitizeFileName(album.Title)
	sanitizedAlbumYear := album.Year // 年份通常是数字，不需要特殊处理
	// 构建目标专辑目录路径： /app/music/歌手/专辑名 (年份)
	// 例如: /app/music/Metallica/Master of Puppets (1986)
	artistDir := filepath.Join(targetDir, sanitizedArtist)
	albumOutputDir := filepath.Join(artistDir, fmt.Sprintf("%s (%s)", sanitizedAlbumTitle, sanitizedAlbumYear))
	if err := os.MkdirAll(albumOutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create album output directory %s: %v", albumOutputDir, err)
	}

	for _, disc := range album.Discs {
		// 如果有多个 Disc，可能还需要在 albumOutputDir 下创建 Disc_N 这样的子目录
		// 例：/app/music/歌手/专辑名 (年份)/Disc 1
		discOutputDir := albumOutputDir
		if len(album.Discs) > 1 {
			discOutputDir = filepath.Join(albumOutputDir, fmt.Sprintf("Disc %d", disc.DiscNumber))
			if err := os.MkdirAll(discOutputDir, 0755); err != nil {
				return fmt.Errorf("failed to create disc output directory %s: %v", discOutputDir, err)
			}
		}
		for _, track := range disc.Tracks {
			time.Sleep(1 * time.Second)
			log.Printf("  Processing Track %02d: %s", track.Number, track.Title)

			// 1. 获取在线元数据
			FetchMetadataAndUpdateTrack(track)

			// 2. 构建 FFmpeg 命令
			trackFileName := fmt.Sprintf("%02d - %s.%s", track.Number, sanitizeFileName(track.Title), "flac") // 示例输出文件名和格式，可以动态获取
			convertedFilePath := filepath.Join(discOutputDir, trackFileName)

			cmd, err := buildFFmpegCommand(disc.WavPath, convertedFilePath, track, album.CoverArt)
			if err != nil {
				log.Printf("  -> ERROR: Could not build ffmpeg command: %v", err)
				continue
			}

			// 3. 执行命令
			log.Printf("  -> Executing FFmpeg...")
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				log.Printf("  -> ERROR: FFmpeg execution failed for track %s.", track.Title)
				log.Printf("  -> FFmpeg output:\n%s", stderr.String())
				continue
			}
			log.Printf("  -> Successfully created %s", convertedFilePath)
		}
	}
	return nil
}

// buildFFmpegCommand 构建一条包含了切割、转码和元数据写入的命令
func buildFFmpegCommand(inputFile, outputFile string, track *Track, coverArtPath string) (*exec.Cmd, error) {
	fmt.Printf("Building ffmpeg %s -> %s (cover: %s)\n", inputFile, outputFile, coverArtPath)
	var args []string
	// 1. 全局选项 (例如：-y 覆盖输出文件)
	args = append(args, "-y")
	// 2. 第一个输入文件 (音频文件) 及其相关选项
	// -ss/起始时间 和 -to/结束时间 对第一个输入文件有效
	args = append(args, "-ss", formatDurationToFFmpegTime(track.StartTime))
	if track.EndTime > 0 {
		args = append(args, "-to", formatDurationToFFmpegTime(track.EndTime))
	}
	args = append(args, "-i", inputFile) // <-- Audio input
	// 3. 第二个输入文件 (封面图片)
	if coverArtPath != "" {
		args = append(args, "-i", coverArtPath) // <-- Image input
	}
	// 4. 输出文件流的映射和编码
	args = append(args, "-map", "0:a") // 映射第一个输入文件（原始音频）的所有音频流
	// 这部分是关键修改：针对 FLAC 格式的图片嵌入
	if coverArtPath != "" {
		// 映射第二个输入文件（图片）的流，并将其标记为 attached_pic
		// 对于 FLAC 容器，FFmpeg 会自动处理将其作为元数据嵌入。
		// 使用 -c:v mjpeg 是一个安全的选择，确保图片以 JPEG 编码嵌入。
		args = append(args,
			"-map", "1:v", // 映射第二个输入文件的视频流（即图片）
			"-c:v", "mjpeg", // 将图片编码为 MJPEG 格式 (FLAC容器可理解的图片嵌入方式)
			"-disposition:v", "attached_pic", // 标记为附加图片
			"-vsync", "0", // 禁用视频同步，因为这只是图片
		)
	}
	// 5. 音频编码设置：转为高质量 FLAC (无损)
	// 移除了 -b:a 320k，因为 FLAC 是无损的，通常不直接设置比特率，
	// 编码器会根据音频内容自动以最高质量压缩。
	args = append(args, "-c:a", "flac")
	// 6. 嵌入元数据
	addMetadata(&args, "title", track.Title)
	addMetadata(&args, "artist", track.Artist)
	addMetadata(&args, "album_artist", track.AlbumArtist)
	addMetadata(&args, "album", track.Album)
	addMetadata(&args, "date", track.Year) // FLAC通常使用 date 作为日期标签
	addMetadata(&args, "track", fmt.Sprintf("%d", track.Number))
	// FLAC 也支持 disc 标签
	// addMetadata(&args, "disc", fmt.Sprintf("%d", track.DiscNumber)) // 假设 Track 结构体有 DiscNumber
	if track.Lyrics != "" {
		// 对于 FLAC，歌词通常嵌入为 UNSYNCEDLYRICS 方式。
		// FFmpeg 可以通过简单提供 metadata 标签来处理。
		addMetadata(&args, "lyrics", track.Lyrics) // 使用 "lyrics" 标签
	}
	// 7. 最终的输出文件路径
	args = append(args, outputFile)
	cmd := exec.Command("ffmpeg", args...)
	// 打印完整的 FFmpeg 命令，有助于调试
	// log.Printf("  -> FFmpeg Command: ffmpeg %s", strings.Join(args, " "))
	return cmd, nil
}

// addMetadata remains the same
func addMetadata(args *[]string, key, value string) {
	if value != "" {
		*args = append(*args, "-metadata", fmt.Sprintf("%s=%s", key, value))
	}
}

// sanitizeFileName 清理文件名，移除或替换不适用于文件路径的字符
func sanitizeFileName(name string) string {
	// 替换所有斜杠为下划线
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")

	// 移除其他不安全的文件名字符 (Windows/Linux通用不推荐的字符)
	// 更多字符可以根据需要添加: : * ? " < > |
	invalidChars := []string{":", "*", "?", "\"", "<", ">", "|"}
	for _, char := range invalidChars {
		name = strings.ReplaceAll(name, char, "")
	}
	// 移除文件名首尾空格和连续空格
	name = strings.TrimSpace(name)
	name = strings.Join(strings.Fields(name), " ") // 将多个空格替换为一个空格
	// 限制长度，如果需要
	// if len(name) > 200 {
	//     name = name[:200]
	// }
	return name
}
