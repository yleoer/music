package converter

// TextConverter 定义文本转换器接口
type TextConverter interface {
	TradToSim(text string) string // 将繁体中文转换为简体
}

var textConverter TextConverter

func GetTextConverter() TextConverter {
	return textConverter
}
