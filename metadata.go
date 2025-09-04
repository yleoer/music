// metadata.go
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
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

// FetchMetadataAndUpdateTrack 搜索并更新 Track 信息
func FetchMetadataAndUpdateTrack(track *Track) {
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
	fetchLyrics(track)
}

// fetchLyrics 获取歌词
func fetchLyrics(track *Track) {
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
