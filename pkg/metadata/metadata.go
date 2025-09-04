package metadata

import (
	"github.com/yleoer/music/pkg/album"
)

const (
	neteaseSearchPath = "/api/search/get/web"
	neteaseLyricPath  = "/api/song/lyric"
)

// Fetcher 定义获取元数据和歌词的接口
type Fetcher interface {
	FetchMetadataAndUpdateTrack(track *album.Track)
}
