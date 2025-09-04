// scanner.go
package main

import (
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ScanAlbumDirectory 扫描专辑目录并构建 Album 对象
func ScanAlbumDirectory(rootPath string) (*Album, error) {
	album := &Album{Path: rootPath}

	// 1. 读取 Info.txt 并解析（使用新的通用读取函数）
	infoPath := filepath.Join(rootPath, "Info.txt")
	if infoContent, err := readTextFileContent(infoPath); err == nil {
		album.InfoContent = infoContent
		parseInfoContent(album) // 调用更新后的解析函数
	} else {
		log.Printf("Warning: Info.txt not found or error reading in %s: %v. Attempting to parse from directory name.", rootPath, err)
		// 尝试从目录名解析作为备用
		album.Artist, album.Title, album.Year = parseArtistTitleYearFromDir(filepath.Base(rootPath))
	}
	// 确保所有从 Info.txt 或目录名获取的字段都转换为简体
	album.Artist = TradToSim(album.Artist)
	album.Title = TradToSim(album.Title)
	// Year通常是数字，无需转换

	// 2. 查找封面
	coverPath := filepath.Join(rootPath, "folder.jpg")
	if _, err := os.Stat(coverPath); err == nil {
		album.CoverArt = coverPath
	}

	// 3. 遍历子目录查找 CUE 文件
	log.Printf("  Searching for CUE files in %s...", rootPath)
	discNumber := 1
	err := filepath.WalkDir(rootPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(strings.ToLower(d.Name()), ".cue") {
			log.Printf("  Found CUE file: %s", path)
			disc, err := processCueFile(path, album, discNumber) // 传入 discNumber
			if err != nil {
				log.Printf("Error processing CUE file %s: %v", path, err)
				return nil // continue walking
			}
			album.Discs = append(album.Discs, disc)
			discNumber++
		}
		return nil
	})

	// 对找到的 Disc 进行排序，确保 Disc 1, Disc 2 顺序
	// 可选：实现一个 sort.Slice 逻辑

	return album, err
}

// parseInfoContent 从 Info.txt 内容中提取信息
// 使用正则表达式更好地匹配你提供的文本格式
func parseInfoContent(album *Album) {
	content := album.InfoContent

	// 专辑名称
	reAlbumName := regexp.MustCompile(`(?i)专辑名称：\s*(.+)`)
	if matches := reAlbumName.FindStringSubmatch(content); len(matches) > 1 {
		album.Title = strings.TrimSpace(matches[1])
	}

	// 出版日期 (年份)
	reDate := regexp.MustCompile(`(?i)出版日期：\s*(\d{4})年`)
	if matches := reDate.FindStringSubmatch(content); len(matches) > 1 {
		album.Year = strings.TrimSpace(matches[1])
	}

	// 艺术家：从第一行“刘德华《笨小孩 1993-1998 国语精选》专辑简介”中提取
	reArtist := regexp.MustCompile(`^(.+)《`)
	if matches := reArtist.FindStringSubmatch(content); len(matches) > 1 {
		album.Artist = strings.TrimSpace(matches[1])
	} else {
		// 如果第一行不符合，可以尝试从其他行找，或默认
		// 示例：如果 Info.txt 中没有明确的“艺术家：”字段，这里可以手动指定或留空让用户填写
		log.Printf("Warning: Could not extract artist from Info.txt. Falling back to default \"Unknown Artist\".")
		album.Artist = "Unknown Artist"
	}

	// 后续可以继续扩展，例如提取出版商等
}

// parseArtistTitleYearFromDir 从目录名解析艺术家、标题和年份作为备用方案
func parseArtistTitleYearFromDir(dirName string) (artist, title, year string) {
	// 假设目录名为 "艺术家 - 专辑名 年份" 或 "专辑名 年份"
	// 示例：劉德華 - 笨小孩 1993-1998 國語精選 WAV+CUE -> Artist: 劉德華, Title: 笨小孩 1993-1998 國語精選, Year: 1993 (或空)

	// 首先尝试匹配 "艺术家 - 专辑名 [年份]"
	reFull := regexp.MustCompile(`^(.+?)\s*-\s*(.+?)(?:\s*\(?(\d{4})\)?)?(?:\s+WAV\+CUE)?$`)
	if matches := reFull.FindStringSubmatch(dirName); len(matches) > 0 {
		artist = strings.TrimSpace(matches[1])
		title = strings.TrimSpace(matches[2])
		if len(matches) > 3 && matches[3] != "" {
			year = matches[3]
		}
		return
	}

	// 否则，尝试匹配 "专辑名 [年份]"
	reSimple := regexp.MustCompile(`^(.+?)(?:\s*\(?(\d{4})\)?)?(?:\s+WAV\+CUE)?$`)
	if matches := reSimple.FindStringSubmatch(dirName); len(matches) > 0 {
		title = strings.TrimSpace(matches[1])
		if len(matches) > 2 && matches[2] != "" {
			year = matches[2]
		}
		artist = "Unknown Artist" // 无法从目录名推断艺术家
		return
	}

	// 最差情况，直接用目录名作为标题
	return "Unknown Artist", dirName, ""
}
