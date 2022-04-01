package websockethttp

import "log"

// HeartbeatHandler 心跳检查处理器（其他Handler可以参考这个编写）
type HeartbeatHandler struct {
}

func (handler *HeartbeatHandler) Index(context *SocketContext) {
	log.Panicln(context)
}
