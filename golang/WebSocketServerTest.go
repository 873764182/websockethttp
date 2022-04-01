package websockethttp

import (
	"github.com/go-basic/uuid"
	"log"
	"net/http"
	"strconv"
	"time"
)

func TestStartUp() {
	en := new(WebSocketServer)
	en.RegisterConnVerify(func(request *http.Request) bool {
		return true
	})
	en.RegisterNameBuilder(func(request *http.Request, channel *ConnChannel) string {
		return strconv.Itoa(time.Now().Second())
	})

	en.AddClientRequestFilterFunc(func(request *SocketRequest, channel *ConnChannel) bool {
		return false
	})
	en.AddClientResponseFilterFunc(func(request *SocketResponse, channel *ConnChannel) bool {
		return false
	})

	en.AddServerRequestFilterFunc(func(request *SocketRequest, channel *ConnChannel) bool {
		return false
	})
	en.AddServerResponseFilterFunc(func(request *SocketResponse, channel *ConnChannel) bool {
		return false
	})

	en.RegisterRequestHandlerObject(new(HeartbeatHandler))
	en.RegisterRequestHandlerFunc("Message", "Chat", func(context *SocketContext) {
		for name, channel := range en.GetConnChannelMap() {
			channel.SendMessage(&SocketRequest{
				Uid:     uuid.New(),
				Handler: "Message",
				Method:  "Chat",
				Header:  make(map[string]string),
				Body:    context.Request.Body,
				Sign:    "none",
			}, func(response *SocketResponse) {
				log.Printf("Message Chat %v %v", name, response.Uid)
			})
		}
	})

	http.HandleFunc("/conn", func(writer http.ResponseWriter, request *http.Request) {
		en.Launcher(writer, request, func(channel *ConnChannel) string {
			return "success"
		}).EnableHeartbeatHandler(true)
	})
	http.HandleFunc("/test", func(writer http.ResponseWriter, request *http.Request) {
		for name, channel := range en.GetConnChannelMap() {
			log.Printf("向渠道 %v 发送消息 request", name)
			channel.SendMessage(&SocketRequest{
				Uid:     uuid.New(),
				Handler: "Message",
				Method:  "Chat",
				Header:  make(map[string]string),
				Body:    "这是测试消息",
				Sign:    "none",
			}, func(response *SocketResponse) {
				log.Printf("向渠道 %v 发送消息 response %v", name, response.Uid)
			})
		}
		_, _ = writer.Write([]byte("OK"))
	})
	log.Print(http.ListenAndServe(":"+strconv.Itoa(8080), nil))
}
