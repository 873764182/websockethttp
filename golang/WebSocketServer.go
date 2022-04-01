package websockethttp

import (
	"encoding/json"
	"github.com/gorilla/websocket"
	"log"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"
)

type WebSocketServer struct {
	verifyFunc  func(request *http.Request) bool
	nameBuilder func(request *http.Request, channel *ConnChannel) string
}

// RegisterConnVerify 注册连接验证函数（判断是否连接）
func (e *WebSocketServer) RegisterConnVerify(verifyFunc func(request *http.Request) bool) {
	e.verifyFunc = verifyFunc
}

// RegisterNameBuilder 注册连接名称构建函数（连接名称）
func (e *WebSocketServer) RegisterNameBuilder(nameBuilder func(request *http.Request, channel *ConnChannel) string) {
	e.nameBuilder = nameBuilder
}

// RegisterRequestHandlerObject 推荐使用RegisterRequestHandlerFunc，不经过反射性能可以进一步提高，也会优先调用
// RegisterRequestHandlerObject 注册请求处理器（所有的业务应该在Handler中处理）
// RegisterRequestHandlerObject handler：对象全名 method：方法名称
func (e *WebSocketServer) RegisterRequestHandlerObject(handler interface{}) {
	HandlerObjectMap[reflect.TypeOf(handler).Elem().Name()] = handler
}

// RegisterRequestHandlerFunc 注册请求处理器（所有的业务应该在Handler中处理）
func (e *WebSocketServer) RegisterRequestHandlerFunc(handlerName, handlerMethod string, handlerFunc func(context *SocketContext)) {
	HandlerFuncMap[BuilderHandlerFuncKey(handlerName, handlerMethod)] = handlerFunc
}

// CloseConnection 关闭连接
func (e *WebSocketServer) CloseConnection(channel *ConnChannel, code int, message string) {
	SafeCloseChannel(channel.Write)
	err := channel.conn.Close()
	delete(ConnMap, channel.Name)
	log.Printf("CloseConnection: %v %v %v %v", channel.Name, code, message, err)
}

// GetConnChannelByName 获取连接的消息通道
func (e *WebSocketServer) GetConnChannelByName(name string) *ConnChannel {
	channel, ok := ConnMap[name]
	if channel != nil && ok {
		return channel
	} else {
		return nil
	}
}

// GetConnChannelMap 获取连接的消息通道列表
func (e *WebSocketServer) GetConnChannelMap() map[string]*ConnChannel {
	return ConnMap
}

// AddClientRequestFilterFunc 添加Request过滤器函数（仅针对客户端对服务端的请求有效）
func (e *WebSocketServer) AddClientRequestFilterFunc(filter func(request *SocketRequest, channel *ConnChannel) bool) {
	ClientRequestFilterList.PushBack(filter)
}

// AddClientResponseFilterFunc 添加Response过滤器函数（仅针对服务端对客户端的响应有效）
func (e *WebSocketServer) AddClientResponseFilterFunc(filter func(request *SocketResponse, channel *ConnChannel) bool) {
	ClientResponseFilterList.PushBack(filter)
}

// AddServerRequestFilterFunc 添加Request过滤器函数（仅针对服务端对客户端的请求有效）
func (e *WebSocketServer) AddServerRequestFilterFunc(filter func(request *SocketRequest, channel *ConnChannel) bool) {
	ServerRequestFilterList.PushBack(filter)
}

// AddServerResponseFilterFunc 添加Response过滤器函数（仅针对客户端对服务端的响应有效）
func (e *WebSocketServer) AddServerResponseFilterFunc(filter func(request *SocketResponse, channel *ConnChannel) bool) {
	ServerResponseFilterList.PushBack(filter)
}

// EnableHeartbeatHandler 启用心跳处理函数（心跳处理函数的名称是与客户端约定好的）
func (e *WebSocketServer) EnableHeartbeatHandler(showLogs bool) {
	e.RegisterRequestHandlerFunc("Health", "Index", func(context *SocketContext) {
		context.Channel.PTime = time.Now().UnixMilli() // 更新连接的活跃时间记录
		context.Response.Code = 0
		context.Response.Msg = "success"
		if showLogs {
			log.Printf("Default Health Handler: name(%s) body(%s)", context.Channel.Name, context.Request.Body)
		}
	})
	HeartbeatCheck = true
}

// SendMessageToChannel 发送消息到指定渠道
func (e *WebSocketServer) SendMessageToChannel(channel *ConnChannel, request *SocketRequest, callback func(response *SocketResponse)) {
	request.Body = MessageBodyEncode(request.Sign, request.Body) // 对消息体进行比编码

	// 应用request过滤器处理
	if ServerRequestFilterList.Len() > 0 {
		for element := ServerRequestFilterList.Front(); element != nil; element = element.Next() {
			if element.Value.(func(request *SocketRequest, channel *ConnChannel) bool)(request, channel) {
				return
			}
		}
	}

	requestJsonString, _ := json.Marshal(request)
	if channel.conn.WriteMessage(websocket.TextMessage, requestJsonString) != nil {
		log.Printf("RequestHandler: 向客户端发送SocketRequest失败 %v", string(requestJsonString))
	}
	CallbackMap[request.Uid] = CallChannel{
		Callback: callback,
		Timeout:  time.Now().UnixMilli() + 1000*60,
		errorMsg: "",
	}
}

// LauncherInNetHttpServer 启动服务
func (e *WebSocketServer) LauncherInNetHttpServer(path string, port int, callback func(channel *ConnChannel) string) {
	http.HandleFunc(path, func(writer http.ResponseWriter, request *http.Request) {
		e.Launcher(writer, request, callback)
	})
	log.Panic(http.ListenAndServe(":"+strconv.Itoa(port), nil))
}

// Launcher 启动服务(兼容使用gin等第三方web框架)
func (e *WebSocketServer) Launcher(writer http.ResponseWriter, request *http.Request, callback func(channel *ConnChannel) string) *WebSocketServer {
	// 调用校验函数处理校验
	if e.verifyFunc != nil && !e.verifyFunc(request) {
		_, _ = writer.Write([]byte("verify_error"))
		return nil
	}
	// 建立连接
	conn, err := WebsocketUpgrade.Upgrade(writer, request, nil)
	if err != nil {
		_, _ = writer.Write([]byte("upgrade_error"))
		return nil
	}
	// 填充数据模型
	channel := new(ConnChannel)
	channel.Name = "" // 在这里可以绑定用户ID
	channel.PTime = time.Now().UnixMilli()
	channel.Write = make(chan string)
	channel.Engine = e
	channel.conn = conn

	conn.SetCloseHandler(func(code int, text string) error {
		e.CloseConnection(channel, code, text)
		return nil
	})

	if e.nameBuilder != nil {
		name := e.nameBuilder(request, channel)
		if len(name) > 0 {
			temp, ok := ConnMap[name]
			if ok && temp != nil {
				e.CloseConnection(temp, 1000, "repetition_conn")
			}
			channel.Name = name
			ConnMap[name] = channel // 保存连接信息
		}
	}

	go func() { e.readClientMessage(channel) }() // 新线程中监听消息

	go func() { e.monitorWriteMessage(channel) }() // 新线程中监听消息

	_, _ = writer.Write([]byte(callback(channel))) // 回调到主函数同时将主函数的返回值当成WEB的返回值

	return e
}

func (e WebSocketServer) monitorWriteMessage(channel *ConnChannel) {
	for {
		cwm := <-channel.Write
		if strings.HasPrefix(cwm, "{") && strings.HasSuffix(cwm, "}") {
			if channel.conn.WriteMessage(websocket.TextMessage, []byte(cwm)) != nil {
				log.Printf("monitorWrite: 向客户端发送request失败 %v %v", channel.Name, cwm)
			}
		} else {
			e.CloseConnection(channel, 1000, "close_action")
			log.Printf("monitorWrite: 收到关闭信息 %v %v", channel.Name, cwm)
			return
		}
	}
}

func (e *WebSocketServer) readClientMessage(channel *ConnChannel) {
	for {
		mt, body, err := channel.conn.ReadMessage() // 读取客户端消息（会阻塞线程）
		if err != nil {
			e.CloseConnection(channel, 1000, "error")
			return // 结束线程
		} else {
			switch mt {
			case websocket.BinaryMessage: // 字节消息(response走BinaryMessage)
				go func() { e.responseHandler(channel, body) }()
				break
			case websocket.TextMessage: // 文本消息(request走TextMessage)
				go func() { e.requestHandler(channel, body) }()
				break
			default:
				e.CloseConnection(channel, 1000, "error")
				return // 结束线程
			}
		}
	}
}

func (e *WebSocketServer) requestHandler(channel *ConnChannel, msg []byte) {
	request := SocketRequest{}
	err := json.Unmarshal(msg, &request)
	if err != nil {
		log.Printf("RequestHandler: WebSocket连接的传输格式错误 %v", string(msg))
		_ = channel.conn.WriteMessage(websocket.BinaryMessage, []byte("你的WebSocket数据格式错误"))
		return
	}
	request.Body = MessageBodyDecode(request.Sign, request.Body)
	response := SocketResponse{
		Uid: request.Uid,
	}

	// 应用request过滤器处理
	if ClientRequestFilterList.Len() > 0 {
		for element := ClientRequestFilterList.Front(); element != nil; element = element.Next() {
			if element.Value.(func(request *SocketRequest, channel *ConnChannel) bool)(&request, channel) {
				return
			}
		}
	}

	if len(request.Handler) > 0 && len(request.Method) > 0 {
		context := SocketContext{
			Channel:  channel,
			Request:  &request,
			Response: &response,
		}
		// 优先调用HandlerFunc处理
		handlerFunc, hok := HandlerFuncMap[BuilderHandlerFuncKey(request.Handler, request.Method)]
		if handlerFunc != nil && hok {
			handlerFunc(&context)
		} else { // 不存在则调用HandlerObject处理
			handlerObject, ook := HandlerObjectMap[request.Handler]
			if handlerObject != nil && ook {
				InvokeObjectMethod(handlerObject, request.Method, &context) // 通过反射执行对应的handler方法
			} else {
				log.Printf("RequestHandler: 没有找到对应的Handlerc处理 %v %v", request.Handler, request.Method)
			}
		}
	}

	// 应用response过滤器处理
	if ClientResponseFilterList.Len() > 0 {
		for element := ClientResponseFilterList.Front(); element != nil; element = element.Next() {
			if element.Value.(func(request *SocketResponse, channel *ConnChannel) bool)(&response, channel) {
				return
			}
		}
	}

	responseJsonString, _ := json.Marshal(&response)
	if channel.conn.WriteMessage(websocket.BinaryMessage, responseJsonString) != nil {
		log.Printf("RequestHandler: 向客户端发送SocketResponse失败 %v", string(responseJsonString))
	}
}

func (e *WebSocketServer) responseHandler(channel *ConnChannel, msg []byte) {
	response := SocketResponse{}
	err := json.Unmarshal(msg, &response)
	if err != nil {
		log.Printf("ResponseHandler: 相应的数据格式错误 %v", string(msg))
		return
	}
	// 应用response过滤器处理
	if ServerResponseFilterList.Len() > 0 {
		for element := ServerResponseFilterList.Front(); element != nil; element = element.Next() {
			if element.Value.(func(request *SocketResponse, channel *ConnChannel) bool)(&response, channel) {
				return
			}
		}
	}
	model, ok := CallbackMap[response.Uid]
	if !ok {
		log.Printf("ResponseHandler: 不存在回调函数的注册 %v", response.Uid)
		return
	}
	go func() {
		model.Callback(&response)
	}()
	delete(CallbackMap, response.Uid)
}
