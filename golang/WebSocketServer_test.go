package websockethttp

import (
	"log"
	"net/http"
	"strconv"
	"testing"
	"time"
)

func TestStartUp(t *testing.T) {

	// 创建服务器对象
	server := new(WebSocketServer)
	// 注册连接校验处理函数，参数都在 request 中（你可以在这里返回 true/false 来决定是否允许客户端的连接请求）
	server.RegisterConnVerify(func(request *http.Request) bool {
		return true
	})
	// 注册连接名构建函数，如果不设置这个服务器将不会保存连接通道，请起一个唯一的名称，推荐使用用户ID
	server.RegisterNameBuilder(func(request *http.Request, channel *ConnChannel) string {
		return strconv.Itoa(time.Now().Second())
	})
	// 添加 client 对 server 请求的过滤器，可以添加多个，返回 true 将终止请求，也就是不会到handler处理器
	server.AddClientRequestFilterFunc(func(request *SocketRequest, channel *ConnChannel) bool {
		return false
	})
	// 添加 server 对 client 响应的过滤器，可以添加多个，返回 true 将终止响应，也就是不会响应数据
	server.AddClientResponseFilterFunc(func(request *SocketResponse, channel *ConnChannel) bool {
		return false
	})
	// 添加server对client发送request时的过滤器
	server.AddServerRequestFilterFunc(func(request *SocketRequest, channel *ConnChannel) bool {
		return false
	})
	// 添加client对server发送response时的过滤器
	server.AddServerResponseFilterFunc(func(request *SocketResponse, channel *ConnChannel) bool {
		return false
	})
	// 添加Handler处理器（不推荐使用对象的方式，需要经过反射，性能不如直接添加函数的方式）
	//server.RegisterRequestHandlerObject(new(HeartbeatHandler))
	// 添加Handler处理函数，处理名名称是 Chat，方法是 Room 的 request 请求（推荐）
	server.RegisterRequestHandlerFunc("Chat", "Room", func(context *SocketContext) {
		// 聊天室功能，直接将消息发送给全部连接用户
		for name, channel := range server.GetConnChannelMap() {
			// server 向 client 发送 request
			channel.SendMessage(&SocketRequest{
				Uid:     UuidNew(),
				Handler: "Chat",
				Method:  "Room",
				Header:  make(map[string]string),
				Body:    context.Request.Body,
				Sign:    "none",
			}, func(response *SocketResponse) {
				log.Printf("消息派发结果 -> name：%v Code：%v", name, response.Code)
			})
		}
	})
	// 打开健康检查
	server.EnableHeartbeatHandler(true)
	// 启动服务器（依赖于go的net/http库可以快速的启动服务器，但是如果你项目不用net/http库就不怎么方便）
	//server.LauncherInNetHttpServer("/websocket/http", 8080, func(channel *ConnChannel) string {
	//	return "success"	// WebSocket连接建立前的http请求响应
	//})
	// 使用 Launcher 可以兼容gin等任意web框架启动
	http.HandleFunc("/websocket/http", func(writer http.ResponseWriter, request *http.Request) {
		// 启动我们的 server
		server.Launcher(writer, request, func(channel *ConnChannel) string {
			return "success"
		})
	})
	log.Print(http.ListenAndServe(":"+strconv.Itoa(8080), nil))
}
