package main

import (
	"log"
	"os"
	"path/filepath"
	"strings" // 新增导入
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// 定义全局常量，指定下载目录和音乐库目录
const (
	downloadDir = "/app/download" // 监听目录
	musicLibDir = "/app/music"    // 刮削后的文件存放目录
	dataDir     = "/app/data"     // SQLite数据库文件存放目录

	dbFileName = "music.db" // SQLite数据库文件名

	// 文件稳定性检查相关参数
	stabilityCheckInterval = 5 * time.Second // 每次检查的间隔
	stabilityQuietDuration = 1 * time.Minute // 文件在多长时间内没有变化才算稳定
	stabilityMaxWait       = 12 * time.Hour  // 最长等待文件稳定的时间
)

// 定义一个全局的互斥锁来保护扫描过程，防止并发扫描同一个目录
var scanMutex sync.Mutex

// 定义一个map来记录需要延迟扫描的目录，并存储它们的扫描时间
var pendingScans = make(map[string]*time.Timer)
var pendingScansMutex sync.Mutex // 保护 pendingScans map

func main() {
	log.Printf("Monitoring download directory: %s", downloadDir)
	log.Printf("Processed music will be stored in: %s", musicLibDir)

	// 确保下载目录和音乐库目录存在，如果不存在则尝试创建
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		log.Fatalf("Failed to create download directory %s: %v", downloadDir, err)
	}
	if err := os.MkdirAll(musicLibDir, 0755); err != nil {
		log.Fatalf("Failed to create music library directory %s: %v", musicLibDir, err)
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Fatalf("Failed to create database directory %s: %v", dataDir, err)
	}

	// 初始化数据库，数据库文件存放在 dataDir 目录
	dbPath := filepath.Join(dataDir, dbFileName)
	err := InitDB(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer CloseDB()
	// 初始扫描下载目录 (只扫描一级目录，并根据DB判断是否已处理)
	log.Println("Performing initial scan for unprocessed albums in download directory...")
	initialScan(downloadDir)
	// 启动文件系统监听 (只监听下载目录的一级子目录)
	startFileWatcher(downloadDir)
	// 保持主Goroutine运行，等待文件监听器退出
	<-make(chan struct{})
}

// initialScan 只扫描一级子目录，并根据DB判断是否已处理
// now takes downloadRoot as its parameter
func initialScan(downloadRoot string) {
	entries, err := os.ReadDir(downloadRoot)
	if err != nil {
		log.Printf("Error reading download directory for initial scan: %v", err)
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			albumDir := filepath.Join(downloadRoot, entry.Name())
			processed, err := IsAlbumProcessed(albumDir) // 检查是否已处理
			if err != nil {
				log.Printf("Error checking processed status for %s: %v", albumDir, err)
			}
			if !processed {
				log.Printf("  -> Found unprocessed album directory: %s. Scheduling scan.", albumDir)
				triggerScan(albumDir)
			} else {
				log.Printf("  -> Album directory %s already processed. Skipping.", albumDir)
			}
		}
	}
	log.Println("Initial scan completed.")
}

// startFileWatcher 启动文件系统监听器，只监听下载目录的一级子目录
// now takes downloadRoot as its parameter
func startFileWatcher(downloadRoot string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatalf("Error creating file watcher: %v", err)
	}
	defer watcher.Close()

	// 仅监听下载根目录本身，用于捕获新的一级子目录创建事件
	if err := watcher.Add(downloadRoot); err != nil {
		log.Fatalf("Error adding download root path %s to watcher: %v", downloadRoot, err)
	}
	log.Printf("  -> Adding download root directory to watcher: %s (for new subdirectories)", downloadRoot)

	// 事件处理循环
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				log.Printf("Watcher event: %s, on %s", event.Op.String(), event.Name)

				// 获取事件发生的目录
				eventDir := filepath.Dir(event.Name)

				// 仅处理下载根目录的直接子目录内的事件，或下载根目录本身的 CREATE 事件
				if eventDir != downloadRoot && event.Name != downloadRoot {
					log.Printf("  -> Event in non-first-level directory %s. Ignoring (only watching %s's direct children).", event.Name, downloadRoot)
					continue
				}

				// 处理下载根目录下的新目录创建事件
				if event.Op&fsnotify.Create == fsnotify.Create && eventDir == downloadRoot {
					if isDirectory(event.Name) { // 确保是目录
						log.Printf("  -> New top-level directory created: %s. Scheduling scan.", event.Name)
						// 即使是新创建的目录，也检查是否已处理（可能上次创建失败后被复用）
						processed, err := IsAlbumProcessed(event.Name)
						if err != nil {
							log.Printf("Error checking processed status for new dir %s: %v", event.Name, err)
						}
						if !processed {
							triggerScan(event.Name)
						} else {
							log.Printf("  -> New directory %s already processed. Skipping.", event.Name)
						}
						continue
					}
				}

				// 处理一级子目录内的文件变动 (Write/Remove/Rename)
				// 或者一级子目录自身的 Rename/Remove 事件
				albumPathToTrigger := event.Name
				if isDirectory(event.Name) { // 如果是目录本身（例如重命名或删除）
					albumPathToTrigger = event.Name
				} else { // 如果是目录内的文件
					albumPathToTrigger = filepath.Dir(event.Name)
				}

				// 只处理下载根目录的直接子目录的事件
				if filepath.Dir(albumPathToTrigger) == downloadRoot {
					log.Printf("  -> File/directory change detected in top-level album candidate: %s. Scheduling rescan.", albumPathToTrigger)
					// 对于文件变动，不清空数据库状态，直接触发扫描。
					// 扫描函数内部会判断是否需要重新处理或覆盖。
					triggerScan(albumPathToTrigger)
				} else if albumPathToTrigger == downloadRoot {
					// 下载根目录本身被修改（例如添加文件），忽略，因为我们只关注子目录
					log.Printf("  -> Download root directory %s itself changed. Ignoring.", downloadRoot)
				} else {
					log.Printf("  -> Event %s not in a direct album directory. Ignoring.", event.Name)
				}

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("Watcher error: %v", err)
			}
		}
	}()

	// 阻塞，等待事件循环运行
	<-make(chan struct{})
}

// isDirectory 辅助函数，检查路径是否为目录
func isDirectory(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false // 或者根据错误类型判断，例如 os.IsNotExist(err)
	}
	return info.IsDir()
}

// triggerScan 将一个目录添加到延迟扫描队列
func triggerScan(dirPath string) {
	pendingScansMutex.Lock()
	defer pendingScansMutex.Unlock()

	// 如果这个目录已经有一个待定的扫描任务，就重置计时器
	if timer, ok := pendingScans[dirPath]; ok {
		timer.Stop()
	}

	// 启动一个新的计时器，延迟一段时间后执行扫描
	timer := time.AfterFunc(stabilityCheckInterval, func() {
		performScan(dirPath)
		// 扫描完成后从队列中移除
		pendingScansMutex.Lock()
		delete(pendingScans, dirPath)
		pendingScansMutex.Unlock()
	})
	pendingScans[dirPath] = timer
	log.Printf("Scheduled scan for %s in %v", dirPath, stabilityCheckInterval)
}

// performScan 执行实际的专辑目录扫描和处理
func performScan(dir string) {
	scanMutex.Lock() // 获取全局锁，避免并发处理同一个目录
	defer scanMutex.Unlock()

	log.Printf("-> Performing full scan for changes in directory: %s", dir)

	// --- 文件稳定性检查 ---
	if !waitForFilesStability(dir, stabilityCheckInterval, stabilityQuietDuration, stabilityMaxWait) {
		log.Printf("  -> Files in %s are still changing. Rescheduling scan.", dir)
		triggerScan(dir) // 重新调度一次扫描
		return
	}
	// --- 结束文件稳定性检查 ---

	// 只有在文件稳定后，才检查是否已处理。
	// 这样做的原因是，如果文件正在下载，它可能已被数据库记录（例如，上次下载失败），
	// 但现在文件内容发生了变化，我们需要重新处理。
	processed, err := IsAlbumProcessed(dir)
	if err != nil {
		log.Printf("Error checking processed status for %s before scan: %v", dir, err)
		// 即使出错也尝试处理，避免遗漏
	}
	// 如果文件稳定，且数据库中已经标记为 processed，则跳过
	// 这适用于以下场景：某个专辑目录以前处理过，并且现在没有文件发生了显著变化 (稳定检查通过)。
	// 如果有文件变化，waitForFilesStability 就会返回 false，重新调度。
	if processed {
		log.Printf("  -> Album directory %s already processed (after stability check). Skipping.", dir)
		return
	}

	album, err := ScanAlbumDirectory(dir)
	if err != nil {
		log.Printf("Error scanning album directory %s: %v", dir, err)
		return
	}

	if album != nil && len(album.Discs) > 0 {
		log.Printf("Album '%s - %s' (%s) found with %d discs. Processing...", album.Artist, album.Title, album.Year, len(album.Discs))

		err = ProcessAlbum(album, musicLibDir)
		if err != nil {
			log.Printf("Error processing album '%s - %s': %v", album.Artist, album.Title, err)
		} else {
			log.Printf("Successfully processed album '%s - %s'.", album.Artist, album.Title)
			AddProcessedAlbum(dir) // 处理成功，标记为已处理 (使用下载目录路径作为唯一标识)
		}
	} else {
		log.Printf("No valid album data found in %s after scan. Not marking as processed.", dir)
	}
}

// fileInfo struct 用于存储文件的关键信息
type fileInfo struct {
	Size    int64
	ModTime time.Time
}

// waitForFilesStability 检查目录中的文件是否稳定（大小和修改时间在指定 `quietDuration` 内没有变化）
// checkInterval: 每次检查的间隔
// quietDuration: 文件需要保持多少时间没有变化才算稳定
// maxWait: 最长等待文件稳定的总时间
func waitForFilesStability(dir string, checkInterval, quietDuration, maxWait time.Duration) bool {
	log.Printf("  -> Waiting for files in %s to stabilize for %v...", dir, quietDuration)
	// 存储上次检查时每个文件的状态
	previousFileStates := make(map[string]fileInfo)

	// 记录每个文件上次被检测到稳定（没有变化）的时间
	fileQuietTimes := make(map[string]time.Time)
	startOverallWait := time.Now()
	for time.Since(startOverallWait) < maxWait {
		currentCheckTime := time.Now()

		entries, err := os.ReadDir(dir)
		if err != nil {
			log.Printf("Error reading directory %s for stability check: %v", dir, err)
			// 如果目录不可读，短时返回false让它重试，长时间错误则放弃
			return false
		}
		allRelevantFilesQuiet := true                  // 标记所有相关文件是否都在 quietDuration 内没有变化
		hasRelevantFiles := false                      // 标记目录中是否存在我们关心的文件
		currentFileStates := make(map[string]fileInfo) // 本次检查的文件状态

		for _, entry := range entries {
			filePath := filepath.Join(dir, entry.Name())
			if entry.IsDir() { // 忽略子目录
				continue
			}
			info, err := entry.Info()
			if err != nil {
				if os.IsNotExist(err) { // 文件可能在检查间隔内被删除
					continue
				}
				log.Printf("Error getting file info for %s: %v", filePath, err)
				allRelevantFilesQuiet = false // 无法获取信息视为不稳定
				hasRelevantFiles = true
				break // 目录不稳定，跳出文件循环
			}
			// 仅关注音频文件和信息文件 (这里可以根据需要扩展或简化)
			ext := strings.ToLower(filepath.Ext(entry.Name()))
			if isAudioFile(filePath) ||
				strings.ToLower(entry.Name()) == "info.txt" ||
				strings.HasSuffix(ext, ".cue") ||
				strings.HasSuffix(ext, ".json") ||
				strings.HasSuffix(ext, ".jpg") || strings.HasSuffix(ext, ".png") { // 包含图片文件

				hasRelevantFiles = true
				currentFileStates[filePath] = fileInfo{Size: info.Size(), ModTime: info.ModTime()}
				prevInfo, exists := previousFileStates[filePath]
				// 判断文件是否发生变化
				fileChanged := false
				if exists && (prevInfo.Size != info.Size() || !prevInfo.ModTime.Equal(info.ModTime())) {
					fileChanged = true
				} else if !exists { // 新增文件
					fileChanged = true
				}
				if fileChanged {
					// 如果文件变化了，重置其 quiet time
					fileQuietTimes[filePath] = currentCheckTime // 更新为当前检查时间
					allRelevantFilesQuiet = false               // 至少一个文件变化，则整体不稳定
				} else {
					// 文件没有变化，检查它是否已经“稳定”了足够长的时间
					lastChangeTime := fileQuietTimes[filePath] // 获取上次检测到变化的时间
					if lastChangeTime.IsZero() {               // 如果是第一次看到这个文件且没有变化
						fileQuietTimes[filePath] = currentCheckTime // 假定它从现在开始是安静的
						// 但为了达到 "quietDuration" 的要求，它还需要进一步等待
						allRelevantFilesQuiet = false
					} else if currentCheckTime.Sub(lastChangeTime) < quietDuration {
						// 虽然这次没变化，但离上次变化还没有达到 quietDuration
						allRelevantFilesQuiet = false
					}
				}
			}
		}

		// 更新本次检查的文件状态为下一次循环的 previousFileStates
		previousFileStates = currentFileStates
		// 如果目录中没有我们关心的文件，则直接认为稳定
		if !hasRelevantFiles {
			log.Printf("  -> No relevant files found in %s that require stability check. Proceeding.", dir)
			return true
		}
		// 检查所有相关文件是否都已达到 quietDuration 的稳定性
		// 遍历所有在 `fileQuietTimes` 中的文件，看它们是否真的都安静了足够久
		allCurrentlyQuiet := true
		for filePath, lastChangeTime := range fileQuietTimes {
			// 如果文件在本次循环已经删除了，跳过
			if _, exists := currentFileStates[filePath]; !exists {
				continue
			}
			if currentCheckTime.Sub(lastChangeTime) < quietDuration {
				allCurrentlyQuiet = false
				break
			}
		}
		if allRelevantFilesQuiet && allCurrentlyQuiet {
			log.Printf("  -> All relevant files in %s are stable for at least %v.", dir, quietDuration)
			return true
		}
		time.Sleep(checkInterval) // 等待下次检查
	}
	log.Printf("  -> Max wait time for stability exceeded for %s. Files still active within %v or new files appeared.", dir, quietDuration)
	return false // 达到最大等待时间，文件仍不稳定
}

// isAudioFile 辅助函数，判断文件是否为音频文件
func isAudioFile(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".wav", ".flac", ".mp3", ".m4a", ".aac", ".ogg", ".ape", ".wv": // 添加更多音频格式
		return true
	default:
		return false
	}
}
