// converter.go
package main

import "github.com/liuzl/gocc"

var t2sConverter *gocc.OpenCC

func init() {
	// 初始化转换器：t2s.json 代表 Traditional Chinese to Simplified Chinese
	var err error
	t2sConverter, err = gocc.New("t2s")
	if err != nil {
		panic("Failed to initialize OpenCC converter: " + err.Error())
	}
}

// TradToSim 将繁体中文转换为简体
func TradToSim(text string) string {
	out, err := t2sConverter.Convert(text)
	if err != nil {
		// 在转换失败时返回原文，保证程序健壮性
		return text
	}
	return out
}
