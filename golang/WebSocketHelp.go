package websockethttp

import (
	"container/list"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"github.com/gorilla/websocket"
	"log"
	"net/http"
	"reflect"
	"time"
)

var WebsocketUpgrade = websocket.Upgrader{
	HandshakeTimeout: 7200,
	ReadBufferSize:   4096,
	WriteBufferSize:  4096,
	WriteBufferPool:  nil,
	Subprotocols:     nil,
	Error: func(w http.ResponseWriter, r *http.Request, status int, reason error) {
		log.Printf("WebSocket Upgrader error %v %s\n", status, reason.Error())
	},
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
	EnableCompression: true,
}

type SocketRequest struct {
	Uid     string            `json:"uid"`     // 消息唯一ID
	Handler string            `json:"handler"` // 目标对象
	Method  string            `json:"method"`  // 目标方法
	Header  map[string]string `json:"header"`  // 数据头
	Body    string            `json:"body"`    // 数据体（需要进行编码后传输）
	Sign    string            `json:"sign"`    // Body编码方式（base64）
}

type SocketResponse struct {
	Uid    string            `json:"uid"`    // 消息唯一ID
	Header map[string]string `json:"header"` // 数据头
	Code   int               `json:"code"`   // 业务状态
	Msg    string            `json:"msg"`    // 业务说明
	Body   string            `json:"body"`   // 数据体（需要进行编码后传输）
	Sign   string            `json:"sign"`   // Body编码方式（base64）
}

type SocketContext struct {
	Channel   *ConnChannel           `json:"channel"`
	Request   *SocketRequest         `json:"request"`
	Response  *SocketResponse        `json:"response"`
	ExtraData map[string]interface{} `json:"extra_data"`
}

type ConnChannel struct {
	Name   string
	PTime  int64
	Write  chan string
	Engine *WebSocketServer
	conn   *websocket.Conn
}

func (channel *ConnChannel) SendMessage(request *SocketRequest, callback func(response *SocketResponse)) {
	channel.Engine.SendMessageToChannel(channel, request, callback)
}

// SafeCloseChannel 安全的关闭channel操作
func SafeCloseChannel(channel chan string) {
	defer func() {
		recover()
	}()
	close(channel)
}

// CallChannel 加上超时绑定
type CallChannel struct {
	MsgId    string
	Callback func(response *SocketResponse)
	Timeout  int64
	errorMsg string
}

// HeartbeatCheck 是否做心跳检查
var HeartbeatCheck = false

// ConnMap 保存连接渠道
var ConnMap = make(map[string]*ConnChannel)

// CallbackMap 保存回调函数
var CallbackMap = make(map[string]CallChannel)

// HandlerFuncMap Handler方法保存
var HandlerFuncMap = make(map[string]func(context *SocketContext))

// HandlerObjectMap Handler对象保存
var HandlerObjectMap = make(map[string]interface{})

// ClientRequestFilterList 客户端对服务端 请求过滤器列表
var ClientRequestFilterList = list.New()

// ClientResponseFilterList 服务端对客户端 响应过滤器列表
var ClientResponseFilterList = list.New()

// ServerRequestFilterList 服务端对客户端 请求过滤器列表
var ServerRequestFilterList = list.New()

// ServerResponseFilterList 客户端对服务端 响应过滤器列表
var ServerResponseFilterList = list.New()

// InvokeObjectMethod (new(YourT2), "MethodFoo", 10, "abc")
func InvokeObjectMethod(objectExample interface{}, methodName string, args ...interface{}) []reflect.Value {
	parameter := make([]reflect.Value, len(args))
	for i := range args {
		parameter[i] = reflect.ValueOf(args[i])
	}
	return reflect.ValueOf(objectExample).MethodByName(methodName).Call(parameter)
}

// BuilderHandlerFuncKey 生成HandlerFunc的索引
func BuilderHandlerFuncKey(handlerName, handlerMethod string) string {
	return handlerName + "@" + handlerMethod
}

func MessageBodyDecode(sign, body string) string {
	decodeString := ""
	switch sign {
	case "none":
		decodeString = body
		break
	case "base64":
		decoded, e := base64.StdEncoding.DecodeString(body)
		if e != nil {
			log.Printf("WebSocket Body 解密失败 %v \n", e)
		} else {
			decodeString = string(decoded)
		}
		break
	case "url":
		decoded, e := base64.URLEncoding.DecodeString(body)
		if e != nil {
			log.Printf("WebSocket Body 解密失败 %v \n", e)
		} else {
			decodeString = string(decoded)
		}
		break
	default:
		log.Printf("MessageBodyDecode: 解密格式 %v \n", sign)
	}
	return decodeString
}

func MessageBodyEncode(sign, body string) string {
	decodeString := ""
	switch sign {
	case "none":
		decodeString = body
		break
	case "base64":
		decodeString = base64.StdEncoding.EncodeToString([]byte(body))
		break
	case "url":
		decodeString = base64.URLEncoding.EncodeToString([]byte(body))
		break
	default:
		log.Printf("MessageBodyDecode: 加密格式 %v \n", sign)
	}
	return decodeString
}

// GenerateUUID 生成作为消息UID的UUID Reference：https://github.com/go-basic/uuid
func GenerateUUID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("无法读取随机字节： %v", err)
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		buf[0:4],
		buf[4:6],
		buf[6:8],
		buf[8:10],
		buf[10:16]), nil
}

func UuidNew() string {
	uuid, err := GenerateUUID()
	if err != nil {
		return ""
	}
	return uuid
}

// 检查保存连接是否有失效的
func connTimeoutCheck() {
	interval := int64(1000 * 60 * 1)
	for range time.Tick(time.Millisecond * time.Duration(interval)) {
		if HeartbeatCheck {
			current := time.Now().UnixMilli()
			for _, m := range ConnMap {
				if (current - m.PTime) > interval {
					m.Engine.CloseConnection(m, 1000, "timeout")
				}
			}
		}
	}
}

// 检查回调函数是否有失效的
func callTimeoutCheck() {
	interval := int64(1000 * 60 * 1)
	for range time.Tick(time.Millisecond * time.Duration(interval)) {
		current := time.Now().UnixMilli()
		for k, m := range CallbackMap {
			if m.Timeout < current {
				m.Callback(&SocketResponse{
					Uid:  m.MsgId,
					Code: -1,
					Msg:  "timeout",
				})
				delete(CallbackMap, k)
			}
		}
	}
}

func init() {
	go func() { connTimeoutCheck() }()
	go func() { callTimeoutCheck() }()
}
