package main

import (
	"database/sql"
	"log"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

var db *sql.DB

const createTableSQL = `
	CREATE TABLE IF NOT EXISTS processed_albums (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		path TEXT NOT NULL UNIQUE,
		processed_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`

// InitDB 初始化 SQLite 数据库
func InitDB(dataSourceName string) error {
	var err error
	db, err = sql.Open("sqlite3", dataSourceName)
	if err != nil {
		return err
	}

	// 创建表，如果不存在
	_, err = db.Exec(createTableSQL)
	if err != nil {
		return err
	}
	log.Printf("SQLite database initialized at: %s", dataSourceName)
	return nil
}

// CloseDB 关闭数据库连接
func CloseDB() {
	if db != nil {
		db.Close()
		log.Println("SQLite database connection closed.")
	}
}

// AddProcessedAlbum 将专辑路径标记为已处理
func AddProcessedAlbum(albumPath string) error {
	_, err := db.Exec("INSERT OR IGNORE INTO processed_albums (path) VALUES (?)", albumPath)
	if err != nil {
		log.Printf("Error adding album %s to processed_albums: %v", albumPath, err)
	} else {
		log.Printf("Album %s marked as processed.", albumPath)
	}
	return err
}

// IsAlbumProcessed 检查专辑路径是否已处理
func IsAlbumProcessed(albumPath string) (bool, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM processed_albums WHERE path = ?", albumPath).Scan(&count)
	if err != nil {
		log.Printf("Error checking if album %s is processed: %v", albumPath, err)
		return false, err
	}
	return count > 0, nil
}
