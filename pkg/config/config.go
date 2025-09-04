package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	DownloadDir            string        `json:"download_dir"`             // 监听目录
	MusicLibDir            string        `json:"music_lib_dir"`            // 刮削后的文件存放目录
	DataDir                string        `json:"data_dir"`                 // SQLite数据库文件存放目录
	DBFileName             string        `json:"db_file_name"`             // SQLite数据库文件名
	DBPath                 string        `json:"-"`                        // 完整的数据库文件路径
	StabilityCheckInterval time.Duration `json:"stability_check_interval"` // 每次检查的间隔
	StabilityQuietDuration time.Duration `json:"stability_quiet_duration"` // 文件在多长时间内没有变化才算稳定
	StabilityMaxWait       time.Duration `json:"stability_max_wait"`       // 最长等待文件稳定的时间
	FFmpegPath             string        `json:"ffmpeg_path"`              // FFmpeg 可执行文件路径
	NeteaseAPI             string        `json:"netease_api"`              // 网易云音乐 API 地址
	HTTPTimeout            time.Duration `json:"http_timeout"`             // HTTP 请求超时
}

const (
	downloadDir = "/app/download"
	musicDir    = "/app/music"
	dataDir     = "/app/data"

	dbFileName = "music.db"
	ffmpeg     = "ffmpeg"
	neteaseAPI = "http://music.163.com/api/search/get/web"

	// 文件稳定性检查相关参数
	stabilityCheckInterval = 5 * time.Second // 每次检查的间隔
	stabilityQuietDuration = 1 * time.Minute // 文件在多长时间内没有变化才算稳定
	stabilityMaxWait       = 12 * time.Hour  // 最长等待文件稳定的时间

	httpTimeout = 30 * time.Second
)

// LoadConfig 从环境变量或默认值加载配置
func LoadConfig() (*Config, error) {
	// 尝试加载 .env 文件
	_ = godotenv.Load()

	cfg := &Config{
		DownloadDir:            os.Getenv("DOWNLOAD_DIR"),
		MusicLibDir:            os.Getenv("MUSIC_LIB_DIR"),
		DataDir:                os.Getenv("DATA_DIR"),
		DBFileName:             os.Getenv("DB_FILE_NAME"),
		StabilityCheckInterval: parseDurationOrDefault(os.Getenv("STABILITY_CHECK_INTERVAL"), stabilityCheckInterval),
		StabilityQuietDuration: parseDurationOrDefault(os.Getenv("STABILITY_QUIET_DURATION"), stabilityQuietDuration),
		StabilityMaxWait:       parseDurationOrDefault(os.Getenv("STABILITY_MAX_WAIT"), stabilityMaxWait),
		FFmpegPath:             os.Getenv("FFMPEG_PATH"),
		NeteaseAPI:             os.Getenv("NETEASE_API"),
		HTTPTimeout:            parseDurationOrDefault(os.Getenv("HTTP_TIMEOUT"), httpTimeout),
	}

	// 设置默认值
	if cfg.DownloadDir == "" {
		cfg.DownloadDir = downloadDir
	}
	if cfg.MusicLibDir == "" {
		cfg.MusicLibDir = musicDir
	}
	if cfg.DataDir == "" {
		cfg.DataDir = dataDir
	}
	if cfg.DBFileName == "" {
		cfg.DBFileName = dbFileName
	}
	if cfg.FFmpegPath == "" {
		cfg.FFmpegPath = ffmpeg
	}
	if cfg.NeteaseAPI == "" {
		cfg.NeteaseAPI = neteaseAPI
	}
	cfg.DBPath = filepath.Join(cfg.DataDir, cfg.DBFileName)
	// 确认目录存在
	if err := os.MkdirAll(cfg.DownloadDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create download directory %s: %w", cfg.DownloadDir, err)
	}
	if err := os.MkdirAll(cfg.MusicLibDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create music library directory %s: %w", cfg.MusicLibDir, err)
	}
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory %s: %w", cfg.DataDir, err)
	}
	log.Printf("Configuration loaded: DownloadDir=%s, MusicLibDir=%s, DataDir=%s, DBPath=%s",
		cfg.DownloadDir, cfg.MusicLibDir, cfg.DataDir, cfg.DBPath)
	return cfg, nil
}

func parseDurationOrDefault(s string, defaultValue time.Duration) time.Duration {
	if s == "" {
		return defaultValue
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		log.Printf("Warning: Could not parse duration '%s', using default '%v'. Error: %v", s, defaultValue, err)
		return defaultValue
	}
	return d
}
