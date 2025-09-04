package processor

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/yleoer/music/pkg/album"
	"github.com/yleoer/music/pkg/util"
)

// FFmpegProcessor 负责通过 FFmpeg 处理音乐文件
type FFmpegProcessor struct {
	ffmpegPath string
	logger     *log.Logger
}

// NewFFmpegProcessor 创建一个新的 FFmpegProcessor 实例
func NewFFmpegProcessor(ffmpegPath string, logger *log.Logger) *FFmpegProcessor {
	return &FFmpegProcessor{ffmpegPath: ffmpegPath, logger: logger}
}

// ProcessAlbum 调用 FFmpeg 处理整张专辑
func (p *FFmpegProcessor) ProcessAlbum(album *album.Album, targetDir string) error {
	sanitizedArtist := util.SanitizeFileName(album.Artist)
	sanitizedAlbumTitle := util.SanitizeFileName(album.Title)
	sanitizedAlbumYear := album.Year // 年份通常是数字
	artistDir := filepath.Join(targetDir, sanitizedArtist)
	albumOutputDir := filepath.Join(artistDir, fmt.Sprintf("%s (%s)", sanitizedAlbumTitle, sanitizedAlbumYear))
	if err := os.MkdirAll(albumOutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create album output directory %s: %v", albumOutputDir, err)
	}
	for _, disc := range album.Discs {
		discOutputDir := albumOutputDir
		if len(album.Discs) > 1 {
			discOutputDir = filepath.Join(albumOutputDir, fmt.Sprintf("Disc %d", disc.DiscNumber))
			if err := os.MkdirAll(discOutputDir, 0755); err != nil {
				return fmt.Errorf("failed to create disc output directory %s: %v", discOutputDir, err)
			}
		}
		for _, track := range disc.Tracks {
			time.Sleep(1 * time.Second) // 避免API请求过快或磁盘IO过载
			p.logger.Printf("  Processing Track %02d: %s", track.Number, track.Title)
			trackFileName := fmt.Sprintf("%02d - %s.%s", track.Number, util.SanitizeFileName(track.Title), "flac")
			convertedFilePath := filepath.Join(discOutputDir, trackFileName)
			cmd, err := p.buildFFmpegCommand(disc.WavPath, convertedFilePath, track, album.CoverArt)
			if err != nil {
				p.logger.Printf("  -> ERROR: Could not build ffmpeg command for track %s: %v", track.Title, err)
				continue
			}
			p.logger.Printf("  -> Executing FFmpeg... Command: %s %s", p.ffmpegPath, strings.Join(cmd.Args[1:], " "))
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				p.logger.Printf("  -> ERROR: FFmpeg execution failed for track %s.", track.Title)
				p.logger.Printf("  -> FFmpeg output:\n%s", stderr.String())
				continue
			}
			p.logger.Printf("  -> Successfully created %s", convertedFilePath)
		}
	}
	return nil
}

// buildFFmpegCommand 构建一条包含了切割、转码和元数据写入的命令
func (p *FFmpegProcessor) buildFFmpegCommand(inputFile, outputFile string, track *album.Track, coverArtPath string) (*exec.Cmd, error) {
	var args []string
	args = append(args, "-y")
	args = append(args, "-ss", util.FormatDurationToFFmpegTime(track.StartTime))
	if track.EndTime > 0 { // 只有非最后一个轨道才设置结束时间
		args = append(args, "-to", util.FormatDurationToFFmpegTime(track.EndTime))
	}
	args = append(args, "-i", inputFile)
	if coverArtPath != "" {
		args = append(args, "-i", coverArtPath)
	}
	args = append(args, "-map", "0:a")
	if coverArtPath != "" {
		args = append(args,
			"-map", "1:v",
			"-c:v", "mjpeg",
			"-disposition:v", "attached_pic",
			"-vsync", "0",
		)
	}
	args = append(args, "-c:a", "flac")
	p.addMetadata(&args, "title", track.Title)
	p.addMetadata(&args, "artist", track.Artist)
	p.addMetadata(&args, "album_artist", track.AlbumArtist)
	p.addMetadata(&args, "album", track.Album)
	p.addMetadata(&args, "date", track.Year)
	p.addMetadata(&args, "track", fmt.Sprintf("%d", track.Number))
	if track.Lyrics != "" {
		p.addMetadata(&args, "lyrics", track.Lyrics)
	}
	args = append(args, outputFile)
	return exec.Command(p.ffmpegPath, args...), nil
}
func (p *FFmpegProcessor) addMetadata(args *[]string, key, value string) {
	if value != "" {
		*args = append(*args, "-metadata", fmt.Sprintf("%s=%s", key, value))
	}
}
