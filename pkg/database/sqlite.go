package database

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// sqliteStore 是 AlbumStore 接口的 SQLite 实现
type sqliteStore struct {
	db     *sql.DB
	logger *log.Logger
}

const createTableSQL = `
	CREATE TABLE IF NOT EXISTS processed_albums (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		path TEXT NOT NULL UNIQUE,
		processed_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`

// NewSQLiteStore 初始化 SQLite 数据库并返回 AlbumStore 接口实例
func NewSQLiteStore(dataSourceName string, log *log.Logger) (AlbumStore, error) {
	db, err := sql.Open("sqlite3", dataSourceName)
	if err != nil {
		return nil, fmt.Errorf("failed to open SQLite database: %w", err)
	}
	// 尝试创建表，如果不存在
	if _, err := db.Exec(createTableSQL); err != nil {
		db.Close() // 创建表失败也要关闭连接
		return nil, fmt.Errorf("failed to create processed_albums table: %w", err)
	}
	log.Printf("SQLite database initialized at: %s", dataSourceName)
	return &sqliteStore{db: db, logger: log}, nil
}

// Close 关闭数据库连接
func (s *sqliteStore) Close() error {
	if s.db != nil {
		err := s.db.Close()
		s.logger.Println("SQLite database connection closed.")
		return err
	}
	return nil
}

// AddProcessedAlbum 将专辑路径标记为已处理
func (s *sqliteStore) AddProcessedAlbum(albumPath string) error {
	_, err := s.db.Exec("INSERT OR IGNORE INTO processed_albums (path, processed_at) VALUES (?, ?)", albumPath, time.Now())
	if err != nil {
		s.logger.Printf("ERROR: Failed to add album %s to processed_albums: %v", albumPath, err)
		return fmt.Errorf("failed to add processed album %s: %w", albumPath, err)
	}
	s.logger.Printf("Album %s marked as processed.", albumPath)
	return nil
}

// IsAlbumProcessed 检查专辑路径是否已处理
func (s *sqliteStore) IsAlbumProcessed(albumPath string) (bool, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM processed_albums WHERE path = ?", albumPath).Scan(&count)
	if err != nil {
		s.logger.Printf("ERROR: Failed to check if album %s is processed: %v", albumPath, err)
		return false, fmt.Errorf("failed to check processed status for %s: %w", albumPath, err)
	}
	return count > 0, nil
}
