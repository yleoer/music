package scheduler

import (
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/yleoer/music/pkg/config"
	"github.com/yleoer/music/pkg/database"
	"github.com/yleoer/music/pkg/metadata"
	"github.com/yleoer/music/pkg/processor"
	"github.com/yleoer/music/pkg/scanner"
	"github.com/yleoer/music/pkg/util"
)

// TaskScheduler 负责调度专辑扫描和处理任务
type TaskScheduler struct {
	cfg               *config.Config
	dbStore           database.AlbumStore
	albumScanner      *scanner.AlbumScanner
	albumProcessor    *processor.FFmpegProcessor
	metaFetcher       metadata.Fetcher
	logger            *log.Logger
	scanMutex         sync.Mutex // 保护扫描过程
	pendingScans      map[string]*time.Timer
	pendingScansMutex sync.Mutex // 保护 pendingScans map
}

// NewTaskScheduler 创建一个新的 TaskScheduler 实例
func NewTaskScheduler(
	cfg *config.Config,
	dbStore database.AlbumStore,
	albumScanner *scanner.AlbumScanner,
	albumProcessor *processor.FFmpegProcessor,
	metaFetcher metadata.Fetcher,
	logger *log.Logger,
) *TaskScheduler {
	return &TaskScheduler{
		cfg:            cfg,
		dbStore:        dbStore,
		albumScanner:   albumScanner,
		albumProcessor: albumProcessor,
		metaFetcher:    metaFetcher,
		logger:         logger,
		pendingScans:   make(map[string]*time.Timer),
	}
}

// InitialScan 对下载目录进行初始扫描
func (ts *TaskScheduler) InitialScan(downloadRoot string) {
	ts.logger.Println("Performing initial scan for unprocessed albums in download directory...")
	entries, err := os.ReadDir(downloadRoot)
	if err != nil {
		ts.logger.Printf("ERROR: Error reading download directory %s for initial scan: %v", downloadRoot, err)
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			albumDir := filepath.Join(downloadRoot, entry.Name())
			processed, err := ts.dbStore.IsAlbumProcessed(albumDir)
			if err != nil {
				ts.logger.Printf("ERROR: Error checking processed status for %s: %v", albumDir, err)
			}
			if !processed {
				ts.logger.Printf("  -> Found unprocessed album directory: %s. Scheduling scan.", albumDir)
				ts.TriggerScan(albumDir)
			} else {
				ts.logger.Printf("  -> Album directory %s already processed. Skipping.", albumDir)
			}
		}
	}
	ts.logger.Println("Initial scan completed.")
}

// TriggerScan 将一个目录添加到延迟扫描队列
func (ts *TaskScheduler) TriggerScan(dirPath string) {
	ts.pendingScansMutex.Lock()
	defer ts.pendingScansMutex.Unlock()
	// 如果这个目录已经有一个待定的扫描任务，就重置计时器
	if timer, ok := ts.pendingScans[dirPath]; ok {
		timer.Stop()
	}
	// 启动一个新的计时器，延迟一段时间后执行扫描
	timer := time.AfterFunc(ts.cfg.StabilityCheckInterval, func() {
		ts.performScan(dirPath)
		// 扫描完成后从队列中移除
		ts.pendingScansMutex.Lock()
		delete(ts.pendingScans, dirPath)
		ts.pendingScansMutex.Unlock()
	})
	ts.pendingScans[dirPath] = timer
	ts.logger.Printf("Scheduled scan for %s in %v", dirPath, ts.cfg.StabilityCheckInterval)
}

// performScan 执行实际的专辑目录扫描和处理
func (ts *TaskScheduler) performScan(dir string) {
	ts.scanMutex.Lock() // 获取全局锁，避免并发处理同一个目录
	defer ts.scanMutex.Unlock()
	ts.logger.Printf("-> Performing full scan for changes in directory: %s", dir)
	// --- 文件稳定性检查 ---
	if !ts.waitForFilesStability(dir) {
		ts.logger.Printf("  -> Files in %s are still changing. Rescheduling scan.", dir)
		ts.TriggerScan(dir) // 重新调度一次扫描
		return
	}
	// --- 结束文件稳定性检查 ---
	processed, err := ts.dbStore.IsAlbumProcessed(dir)
	if err != nil {
		ts.logger.Printf("ERROR: Error checking processed status for %s before scan: %v", dir, err)
		// 即使出错也尝试处理，避免遗漏
	}
	if processed {
		ts.logger.Printf("  -> Album directory %s already processed (after stability check). Skipping.", dir)
		return
	}
	album, err := ts.albumScanner.ScanAlbumDirectory(dir)
	if err != nil {
		ts.logger.Printf("ERROR: Error scanning album directory %s: %v", dir, err)
		return
	}
	if album != nil && len(album.Discs) > 0 {
		ts.logger.Printf("Album '%s - %s' (%s) found with %d discs. Processing metadata and transcoding...", album.Artist, album.Title, album.Year, len(album.Discs))
		// 处理每个轨道的元数据
		for _, disc := range album.Discs {
			for _, track := range disc.Tracks {
				ts.metaFetcher.FetchMetadataAndUpdateTrack(track)
			}
		}

		err = ts.albumProcessor.ProcessAlbum(album, ts.cfg.MusicLibDir)
		if err != nil {
			ts.logger.Printf("ERROR: Error processing album '%s - %s': %v", album.Artist, album.Title, err)
		} else {
			ts.logger.Printf("Successfully processed album '%s - %s'.", album.Artist, album.Title)
			ts.dbStore.AddProcessedAlbum(dir) // 处理成功，标记为已处理
		}
	} else {
		ts.logger.Printf("No valid album data found in %s after scan. Not marking as processed.", dir)
	}
}

// waitForFilesStability 检查目录中的文件是否稳定
func (ts *TaskScheduler) waitForFilesStability(dir string) bool {
	ts.logger.Printf("  -> Waiting for files in %s to stabilize for %v...", dir, ts.cfg.StabilityQuietDuration)
	previousFileStates := make(map[string]fileInfo)
	fileQuietTimes := make(map[string]time.Time)
	startOverallWait := time.Now()
	for time.Since(startOverallWait) < ts.cfg.StabilityMaxWait {
		currentCheckTime := time.Now()
		entries, err := os.ReadDir(dir)
		if err != nil {
			ts.logger.Printf("ERROR: Error reading directory %s for stability check: %v", dir, err)
			time.Sleep(ts.cfg.StabilityCheckInterval)
			continue
		}
		allRelevantFilesQuiet := true
		hasRelevantFiles := false
		currentFileStates := make(map[string]fileInfo)
		for _, entry := range entries {
			filePath := filepath.Join(dir, entry.Name())
			if entry.IsDir() {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				ts.logger.Printf("ERROR: Error getting file info for %s: %v", filePath, err)
				allRelevantFilesQuiet = false
				hasRelevantFiles = true
				break
			}
			//ext := strings.ToLower(filepath.Ext(entry.Name()))
			// 统一使用 util.IsRelevantMusicFile 辅助函数
			if util.IsRelevantMusicFile(filePath) {
				hasRelevantFiles = true
				currentFileStates[filePath] = fileInfo{Size: info.Size(), ModTime: info.ModTime()}
				prevInfo, exists := previousFileStates[filePath]
				fileChanged := false
				if exists && (prevInfo.Size != info.Size() || !prevInfo.ModTime.Equal(info.ModTime())) {
					fileChanged = true
				} else if !exists {
					fileChanged = true
				}
				if fileChanged {
					fileQuietTimes[filePath] = currentCheckTime
					allRelevantFilesQuiet = false
				} else {
					lastChangeTime := fileQuietTimes[filePath]
					if lastChangeTime.IsZero() {
						fileQuietTimes[filePath] = currentCheckTime
						allRelevantFilesQuiet = false
					} else if currentCheckTime.Sub(lastChangeTime) < ts.cfg.StabilityQuietDuration {
						allRelevantFilesQuiet = false
					}
				}
			}
		}
		previousFileStates = currentFileStates
		if !hasRelevantFiles {
			ts.logger.Printf("  -> No relevant files found in %s that require stability check. Proceeding.", dir)
			return true
		}
		allCurrentlyQuiet := true
		for filePath, lastChangeTime := range fileQuietTimes {
			if _, exists := currentFileStates[filePath]; !exists { // 文件被删除
				continue
			}
			if currentCheckTime.Sub(lastChangeTime) < ts.cfg.StabilityQuietDuration {
				allCurrentlyQuiet = false
				break
			}
		}
		if allRelevantFilesQuiet && allCurrentlyQuiet {
			ts.logger.Printf("  -> All relevant files in %s are stable for at least %v.", dir, ts.cfg.StabilityQuietDuration)
			return true
		}
		time.Sleep(ts.cfg.StabilityCheckInterval)
	}
	ts.logger.Printf("  -> Max wait time for stability exceeded for %s. Files still active within %v or new files appeared.", dir, ts.cfg.StabilityQuietDuration)
	return false
}

// fileInfo struct 用于存储文件的关键信息 (可移到 util 包)
type fileInfo struct {
	Size    int64
	ModTime time.Time
}
