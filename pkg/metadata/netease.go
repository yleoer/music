package metadata

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/yleoer/music/pkg/album"
)

const NeteaseSearchAPI = "http://music.163.com/api/search/get/web"

type NeteaseSearchResult struct {
	Result struct {
		Songs []struct {
			ID      int    `json:"id"`
			Name    string `json:"name"`
			Artists []struct {
				Name string `json:"name"`
			} `json:"artists"`
			Album struct {
				Name string `json:"name"`
			} `json:"album"`
		} `json:"songs"`
	} `json:"result"`
}

type NeteaseLyricResult struct {
	Lrc struct {
		Lyric string `json:"lyric"`
	} `json:"lrc"`
}

// NeteaseClient 是 Fetcher 的网易云音乐实现
type NeteaseClient struct {
	baseURL    string
	httpClient *http.Client
	logger     *log.Logger
}

// NewNeteaseClient 创建一个新的 NeteaseClient 实例
func NewNeteaseClient(baseURL string, timeout time.Duration, logger *log.Logger) Fetcher {
	if baseURL == "" {
		baseURL = "http://music.163.com" // Default to Netease's base URL
	}
	return &NeteaseClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		logger: logger,
	}
}

// FetchMetadataAndUpdateTrack 搜索并更新 Track 信息
func (c *NeteaseClient) FetchMetadataAndUpdateTrack(track *album.Track) {
	log.Printf("    -> Searching online for: [%s - %s]", track.Artist, track.Title)

	query := fmt.Sprintf("%s %s", track.Title, track.Artist)
	params := url.Values{}
	params.Add("s", query)
	params.Add("type", "1") // 1 for songs
	params.Add("limit", "5")

	resp, err := http.Get(NeteaseSearchAPI + "?" + params.Encode())
	if err != nil {
		log.Printf("    -> ERROR: Failed to search: %v", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result NeteaseSearchResult
	if json.Unmarshal(body, &result) != nil || len(result.Result.Songs) == 0 {
		log.Printf("    -> WARN: No results found for '%s'.", query)
		return
	}

	// 简单匹配：选择第一个结果
	bestMatch := result.Result.Songs[0]
	track.OnlineID = bestMatch.ID
	log.Printf("    -> Matched song: %s (ID: %d)", bestMatch.Name, bestMatch.ID)

	// 获取歌词
	c.fetchLyrics(track)
}

func (c *NeteaseClient) fetchLyrics(track *album.Track) {
	if track.OnlineID == 0 {
		return
	}
	lyricURL := fmt.Sprintf("http://music.163.com/api/song/lyric?id=%d&lv=1&kv=1&tv=-1", track.OnlineID)
	resp, err := http.Get(lyricURL)
	if err != nil {
		log.Printf("    -> ERROR: Failed to get lyrics: %v", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var lyricResult NeteaseLyricResult
	if json.Unmarshal(body, &lyricResult) == nil {
		track.Lyrics = lyricResult.Lrc.Lyric
		log.Println("    -> Lyrics downloaded successfully.")
	}
}
