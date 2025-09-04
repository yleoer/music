package converter

import (
	"fmt"
	"log"

	"github.com/liuzl/gocc"
)

// openCCConverter 是 TextConverter 的一个实现
type openCCConverter struct {
	converter *gocc.OpenCC
	logger    *log.Logger
}

// NewOpenCCConverter 初始化并返回一个 OpenCC 转换器实例
func NewOpenCCConverter(log *log.Logger) (TextConverter, error) {
	// 初始化转换器：t2s.json 代表 Traditional Chinese to Simplified Chinese
	converter, err := gocc.New("t2s")
	if err != nil {
		return nil, fmt.Errorf("failed to initialize OpenCC converter: %w", err)
	}
	log.Println("OpenCC converter (t2s) initialized.")
	return &openCCConverter{converter: converter, logger: log}, nil
}

// TradToSim 将繁体中文转换为简体
func (c *openCCConverter) TradToSim(text string) string {
	if c.converter == nil {
		fmt.Println("WARN: OpenCC converter not initialized, returning original text.")
		return text
	}
	out, err := c.converter.Convert(text)
	if err != nil {
		fmt.Printf("WARN: Failed to convert text '%s' from Traditional to Simplified: %v", text, err)
		return text // 在转换失败时返回原文
	}
	return out
}
