// parser.go
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type CueSheet struct {
	WavFile string
	Tracks  []Track
}

// parseCueTime 将 MM:SS:FF 格式的时间字符串转换为 time.Duration
func parseCueTime(timeStr string) (time.Duration, error) {
	parts := strings.Split(timeStr, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid time format: %s", timeStr)
	}
	minutes, _ := strconv.Atoi(parts[0])
	seconds, _ := strconv.Atoi(parts[1])
	frames, _ := strconv.Atoi(parts[2])
	totalMilliseconds := int64(minutes*60*1000 + seconds*1000 + (frames*1000)/75)
	return time.Duration(totalMilliseconds) * time.Millisecond, nil
}

// parseCueFile 解析 .cue 文件并返回一个 CueSheet 结构体
// 现在它会在内部调用 readTextFileContent 来处理编码
func parseCueFile(cuePath string) (*CueSheet, error) {
	content, err := readTextFileContent(cuePath) // 使用通用读取函数
	if err != nil {
		return nil, fmt.Errorf("failed to read CUE file with encoding detection: %w", err)
	}

	cue := &CueSheet{}
	var currentTrack *Track

	fileRegex := regexp.MustCompile(`(?i)FILE "([^"]+)"`) // (?i) for case-insensitive
	trackRegex := regexp.MustCompile(`(?i)TRACK (\d+) AUDIO`)
	titleRegex := regexp.MustCompile(`(?i)TITLE "([^"]+)"`)
	indexRegex := regexp.MustCompile(`(?i)INDEX 01 (\d{2}:\d{2}:\d{2})`)

	// 使用 bufio.NewScanner 处理已经解码为 UTF-8 的内容
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if matches := fileRegex.FindStringSubmatch(line); len(matches) > 1 {
			cue.WavFile = matches[1]
		} else if matches := trackRegex.FindStringSubmatch(line); len(matches) > 1 {
			if currentTrack != nil {
				cue.Tracks = append(cue.Tracks, *currentTrack)
			}
			num, _ := strconv.Atoi(matches[1])
			currentTrack = &Track{Number: num}
		} else if currentTrack != nil {
			if matches := titleRegex.FindStringSubmatch(line); len(matches) > 1 {
				currentTrack.Title = matches[1]
			} else if matches := indexRegex.FindStringSubmatch(line); len(matches) > 1 {
				startTime, _ := parseCueTime(matches[1])
				currentTrack.StartTime = startTime
			}
		}
	}
	if currentTrack != nil {
		cue.Tracks = append(cue.Tracks, *currentTrack)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(cue.Tracks) == 0 {
		return nil, fmt.Errorf("no tracks found in cue file '%s'", cuePath)
	}

	return cue, nil
}

// processCueFile 读取并解析 CUE 文件，返回 Disc 对象（此函数在 scanner.go 中被调用，需要确保能访问到 parser.go 中的函数）
// 这里是其简化版本，确保它能正确调用 parseCueFile
func processCueFile(cuePath string, album *Album, discNumber int) (*Disc, error) {
	cueSheet, err := parseCueFile(cuePath)
	if err != nil {
		return nil, err
	}

	// 确定 WAV 文件的路径。CUE 文件中的 WAV 文件名可能是相对路径。
	wavFileInCue := cueSheet.WavFile
	sourceWavPath := filepath.Join(filepath.Dir(cuePath), wavFileInCue)
	if _, err := os.Stat(sourceWavPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("source WAV file '%s' specified in CUE not found", sourceWavPath)
	}

	disc := &Disc{
		DiscNumber: discNumber,
		CuePath:    cuePath,
		WavPath:    sourceWavPath,
		Tracks:     make([]*Track, 0, len(cueSheet.Tracks)),
	}

	// 填充轨道信息，计算 EndTime
	for i, cueTrack := range cueSheet.Tracks {
		track := &Track{
			Number:      cueTrack.Number,
			Title:       TradToSim(cueTrack.Title), // CUE 中的标题也可能需要繁转简
			Album:       album.Title,
			AlbumArtist: album.Artist,
			Artist:      album.Artist, // 默认与专辑艺术家相同，之后可能被网络元数据覆盖
			Year:        album.Year,
			StartTime:   cueTrack.StartTime,
		}

		// 分离合唱艺术家 (根据CUE标题解析) - 简单示例
		if strings.Contains(track.Title, " （与") || strings.Contains(track.Title, " (与") {
			// 示例： "笨小孩（与柯受良、吴宗宪合唱）"
			reFt := regexp.MustCompile(`(.+)[ （](?:与|feat\.)(.+)[）)]`)
			if matches := reFt.FindStringSubmatch(track.Title); len(matches) > 2 {
				track.Title = strings.TrimSpace(matches[1])
				track.Artist = fmt.Sprintf("%s, %s", album.Artist, TradToSim(matches[2])) // 歌曲艺术家
			}
		} else {
			track.Artist = album.Artist // 默认与专辑艺术家相同
		}

		// 计算当前轨道的结束时间
		if i+1 < len(cueSheet.Tracks) {
			track.EndTime = cueSheet.Tracks[i+1].StartTime
		} else {
			// 最后一个轨道的结束时间无法从CUE直接获得，需要额外处理，
			// 比如通过解析WAV文件时长或在FFmpeg中省略-to参数（切割到文件结尾）
			// 目前，我们让FFmpeg默认切割到文件末尾即可（不提供-to）
		}

		disc.Tracks = append(disc.Tracks, track)
	}

	return disc, nil
}
