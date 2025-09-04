package database

// AlbumStore 定义专辑处理状态存储接口
type AlbumStore interface {
	AddProcessedAlbum(albumPath string) error        // 将专辑路径标记为已处理
	IsAlbumProcessed(albumPath string) (bool, error) // 检查专辑路径是否已处理
	Close() error                                    // 关闭数据库连接
}
