// album.go
package main

import "time"

// Album 代表一张完整的专辑信息
type Album struct {
	Path        string // 专辑根目录
	Artist      string
	Title       string
	Year        string
	CoverArt    string  // 封面图片路径
	Discs       []*Disc // 专辑包含的光盘
	InfoContent string  // Info.txt 的内容
}

// Disc 代表一张光盘
type Disc struct {
	DiscNumber int
	CuePath    string
	WavPath    string
	Tracks     []*Track
}

// Track 代表一个音轨
type Track struct {
	Number      int
	Title       string
	Artist      string // 可能是合唱，所以每个轨道都保留
	StartTime   time.Duration
	EndTime     time.Duration
	Album       string // 反向引用
	AlbumArtist string // 专辑艺术家
	Year        string

	// 从网络获取的元数据
	OnlineID int    // 网易云音乐 ID
	Lyrics   string // 歌词文本
}
