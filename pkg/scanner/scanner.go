package scanner

import (
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/yleoer/music/pkg/album"
	"github.com/yleoer/music/pkg/converter"
	"github.com/yleoer/music/pkg/parser"
	"github.com/yleoer/music/pkg/util"
)

// AlbumScanner 负责扫描专辑目录并构建 Album 对象
type AlbumScanner struct {
	cueParser parser.CueParser // 修改为 CueParser 实例，而不是接口
	converter converter.TextConverter
	logger    *log.Logger
}

// NewAlbumScanner 创建一个新的 AlbumScanner 实例
func NewAlbumScanner(cp *parser.CueParser, tc converter.TextConverter, logger *log.Logger) *AlbumScanner {
	return &AlbumScanner{
		cueParser: *cp, // 注意这里是结构体，所以直接赋值。如果 CueParser 是接口，则传递接口。
		converter: tc,
		logger:    logger,
	}
}

// ScanAlbumDirectory 扫描专辑目录并构建 Album 对象
func (s *AlbumScanner) ScanAlbumDirectory(rootPath string) (*album.Album, error) {
	// ... (原逻辑，但调用 s.cueParser 和 s.converter 方法) ...
	albumObj := &album.Album{Path: rootPath}
	infoPath := filepath.Join(rootPath, "Info.txt")
	if infoContent, err := util.ReadTextFileContent(infoPath); err == nil {
		albumObj.InfoContent = infoContent
		s.parseInfoContent(albumObj)
	} else {
		s.logger.Printf("Warning: Info.txt not found or error reading in %s: %v. Attempting to parse from directory name.", rootPath, err)
		albumObj.Artist, albumObj.Title, albumObj.Year = s.parseArtistTitleYearFromDir(filepath.Base(rootPath))
	}
	albumObj.Artist = s.converter.TradToSim(albumObj.Artist)
	albumObj.Title = s.converter.TradToSim(albumObj.Title)
	// Find cover art
	coverPath := filepath.Join(rootPath, "folder.jpg")
	if _, err := os.Stat(coverPath); err == nil {
		albumObj.CoverArt = coverPath
	}
	s.logger.Printf("  Searching for CUE files in %s...", rootPath)
	discNumber := 1
	err := filepath.WalkDir(rootPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// 忽略子目录中的 .cue 文件，只处理一级目录或与音频文件同级的 .cue
		// 或者，如果你的 CUE 文件可能在子目录，这里需要更复杂的逻辑
		if !d.IsDir() && strings.HasSuffix(strings.ToLower(d.Name()), ".cue") && filepath.Dir(path) == rootPath {
			s.logger.Printf("  Found CUE file: %s", path)
			disc, err := s.cueParser.ProcessCueFile(path, albumObj, discNumber) // 调用新的 CueParser 方法
			if err != nil {
				s.logger.Printf("Error processing CUE file %s: %v", path, err)
				return nil // continue walking
			}
			albumObj.Discs = append(albumObj.Discs, disc)
			discNumber++
		}
		return nil
	})
	sort.Slice(albumObj.Discs, func(i, j int) bool {
		return albumObj.Discs[i].DiscNumber < albumObj.Discs[j].DiscNumber
	})
	return albumObj, err
}

// parseInfoContent 和 parseArtistTitleYearFromDir 成为 AlbumScanner 的私有方法
func (s *AlbumScanner) parseInfoContent(album *album.Album) {
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
}
func (s *AlbumScanner) parseArtistTitleYearFromDir(dirName string) (artist, title, year string) {
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
