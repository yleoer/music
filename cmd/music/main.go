package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
	"github.com/yleoer/music/pkg/config"
	"github.com/yleoer/music/pkg/converter"
	"github.com/yleoer/music/pkg/database"
	"github.com/yleoer/music/pkg/metadata"
	"github.com/yleoer/music/pkg/parser"
	"github.com/yleoer/music/pkg/processor"
	"github.com/yleoer/music/pkg/scanner"
	"github.com/yleoer/music/pkg/scheduler"
	"github.com/yleoer/music/pkg/util"
)

func main() {
	// 1. 初始化日志器
	logger := log.New(os.Stdout, "[MusicProcessor] ", log.LstdFlags|log.Lshortfile)
	logger.Println("Starting Music Processor application...")
	// 2. 加载配置
	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Fatalf("Failed to load configuration: %v", err)
	}
	logger.Printf("Configuration loaded: DownloadDir=%s, MusicLibDir=%s, DataDir=%s, DBPath=%s",
		cfg.DownloadDir, cfg.MusicLibDir, cfg.DataDir, cfg.DBPath)
	// 3. 初始化所有依赖服务
	// 3.1 繁简体转换器
	t2sConverter, err := converter.NewOpenCCConverter(logger)
	if err != nil {
		logger.Fatalf("Failed to initialize OpenCC converter: %v", err)
	}
	// 3.2 数据库存储
	dbStore, err := database.NewSQLiteStore(cfg.DBPath, logger)
	if err != nil {
		logger.Fatalf("Failed to initialize database: %v", err)
	}
	defer dbStore.Close()
	// 3.3 元数据获取器
	metaFetcher := metadata.NewNeteaseClient(cfg.NeteaseAPI, cfg.HTTPTimeout, logger)
	// 3.4 CUE 文件解析器 (依赖于 TextConverter)
	cueParser := parser.NewCueParser(t2sConverter, logger)
	// 3.5 专辑扫描器 (依赖于 CueParser 和 TextConverter)
	albumScanner := scanner.NewAlbumScanner(cueParser, t2sConverter, logger)
	// 3.6 FFmpeg 处理器 (依赖于 MetadataFetcher, Config)
	ffmpegProcessor := processor.NewFFmpegProcessor(cfg.FFmpegPath, logger)
	// 4. 初始化任务调度器
	taskScheduler := scheduler.NewTaskScheduler(
		cfg,
		dbStore,
		albumScanner,
		ffmpegProcessor,
		metaFetcher,
		logger,
	)
	// 5. 执行初始扫描
	taskScheduler.InitialScan(cfg.DownloadDir)
	// 6. 启动文件系统监听器 (fsnotify 监听器现在只关注一级目录的事件)
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Fatalf("Error creating file watcher: %v", err)
	}
	defer watcher.Close()
	if err := watcher.Add(cfg.DownloadDir); err != nil {
		logger.Fatalf("Error adding download root path %s to watcher: %v", cfg.DownloadDir, err)
	}
	logger.Printf("Monitoring download directory %s for new top-level subdirectories...", cfg.DownloadDir)
	// 7. 处理文件系统事件
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				logger.Printf("Watcher event: %s, on %s", event.Op.String(), event.Name)
				// 仅关注下载根目录下的直接子目录事件
				// 1. 新建顶级目录
				if event.Op&fsnotify.Create == fsnotify.Create && filepath.Dir(event.Name) == cfg.DownloadDir {
					if util.IsDirectory(event.Name) {
						logger.Printf("  -> New top-level directory created: %s. Scheduling scan.", event.Name)
						// 即使是新创建的目录，也检查是否已处理（可能上次创建失败后被复用）
						// taskScheduler 内部会处理重复扫描和已处理标记
						taskScheduler.TriggerScan(event.Name)
						continue
					}
				}
				// 2. 顶级目录内的文件变化 或 顶级目录本身被修改
				albumPathCandidate := event.Name
				// 如果event.Name是文件，我们关注它所在的父目录
				if !util.IsDirectory(event.Name) {
					albumPathCandidate = filepath.Dir(event.Name)
				}
				// 且这个父目录必须是下载根目录的直接子目录
				if filepath.Dir(albumPathCandidate) == cfg.DownloadDir {
					logger.Printf("  -> File/directory change detected in top-level album candidate: %s. Scheduling rescan.", albumPathCandidate)
					taskScheduler.TriggerScan(albumPathCandidate)
				} else if albumPathCandidate == cfg.DownloadDir {
					// 下载根目录本身被修改（例如添加文件），忽略，因为我们只关注子目录
					logger.Printf("  -> Download root directory %s itself changed. Ignoring.", cfg.DownloadDir)
				} else {
					logger.Printf("  -> Event %s not in a direct album directory. Ignoring.", event.Name)
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				logger.Printf("ERROR: Watcher error: %v", err)
			}
		}
	}()
	// 保持主Goroutine运行
	logger.Println("Application is running. Press Ctrl+C to exit.")
	<-make(chan struct{})
}
