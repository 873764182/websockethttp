# WebSocketHttp

#### 介绍

使用WebSocket协议，可以非常方便的在客户端与服务端之间建立连接通道。 但是在我们的实际项目中往往会遇到以下问题：

1. 依赖于网络情况，会经常断开，所以需要一种重连机制
2. 为了节约资源，项目一般只有一个socket通道连接，然后多个业务功能共用这一个通道传输数据，需要对数据进行业务区分
3. 消息从socket发出后没有回复，不知道对方是否处理，需要一种约定对每一次的发送都要做响应

“WebSocket Http” 的目的就是为了做到以上功能一种上层约定，让socket之间的数据传输可以像Http一样简单，而且是双向的， 不单单是客户端对服务端发请求，服务端也可以对客户端发送请求，双方处理机制完全相同，差异仅客户端没有过滤器设置

```go
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
```

##### 需求分析

> 为了处理第一种情况我们需要有一种健康检查机制，定时检查连接的状态
>
> 为了处理第二种情况，那我们需要约定一个数据格式，才能在一个通道里面做好数据区分，所以我们要有一个 request
>
> 既然我们都有了request，那加个response第三种情况也能解决了
>
> 因为socket协议是没有响应值的，所以 response 我们约定通过异步线程回调的方式响应，通过uid做唯一处理
>
> 抽象出Handler（处理器）这一层做区分业务功能，所有业务都在handler中处理
>
> 所以我们得到以下的request与response的数据格式

###### request

```json
{
  "uid": "",
  "handler": "",
  "method": "",
  "header": {},
  "body": "",
  "sign": ""
}
```

###### response

```json
 {
  "uid": "",
  "header": {},
  "code": 0,
  "msg": "",
  "body": "",
  "sign": ""
}
```

##### Json字段说明

- uid：消息唯一ID
- handler：消息处理器名称
- method：消息处理器方法
- header：请求头或者响应头
- body：消息体
- sign：body 加密方式 -> none, url, base64
- code: response 状态码
- msg：response 状态说明

#### 消息流程图

![message.png](.images/message.png "message")

#### 软件架构

这是一个聚合项目，分有多种语言版本，每一个语言都有客户端与服务端版本，根据你的项目语言与需求进行选择。

- [golang](https://gitee.com/vesmr/websockethttp-go "websockethttp-go")
- [javascript](https://gitee.com/vesmr/websockethttp-js "websockethttp-js")
- java 开发中...
- c/c++ 开发中...
- dart 开发中...

#### 安装教程

- 直接下载对应版本源码到自己的项目中
- 使用各种语言的中央仓库命令安装

#### 使用说明

我们基于WebSocketHttp创建一个简单的聊天室来介绍使用，

- 服务端使用 golang
- 客户端使用 javascript

1. 下载 golang目录 代码到自己项目，并执行 依赖初始化操作
2. 在你的 golang main 函数中添加代码 代码参考 [WebSocketServer_test.go](./golang/WebSocketServer_test.go "code")

```go
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
				Uid:     uuid.New(),
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
```

3.

以上代码，我们不做连接鉴权，所有连接都允许，名称使用了一个随机数，过滤器什么都不做，使用server.RegisterRequestHandlerFunc注册了一个request处理函数，收到消息直接就转发给所有连接，这是我们的聊天室需求，然后使用net/http启动server

4. 下载javascript的客户端代码到你的项目，客户端代码主要在 WebSocketClient.js 文件
5. 在你的html页面中引入WebSocketClient.js文件

```html

<script type="text/javascript" src="./WebSocketClient.js"></script>
```

```js
function openSocketConn() {
    $WebSocketHttp.openConnection("ws://127.0.0.1:8080/websocket/http", function () {
        $WebSocketHttp.registerRequestHandlerFunction("Chat", "Room", function (response) {
            let cv = document.getElementById("messageOutput").innerText;
            cv = cv + "\n\n" + response.body
            document.getElementById("messageOutput").innerText = cv
        })
    })
}

function doCloseConn() {
    $WebSocketHttp.closeConnection(1000, '主动关闭')
}

function doSendMessageText() {
    const message = document.getElementById("messageInput").value;
    $WebSocketHttp.sendTextMessage("Chat", "Room", message, function (response, ws) {
        console.log("doSendMessageText", response)
    })
}

function doClearMessage() {
    document.getElementById("messageOutput").innerText = ""
}
```

6. 引入WebSocketClient.js后则可以调用函数中的操作建立连接，$WebSocketHttp是WebSocketHtpp绑定到windows的对象
7. 向指定服务器发起连接，同时在成功的回调函数中注册了一个request处理，处理handler名为Chat，方法为Room的函数，收到消息后直接打印到页面上
8. 发送消息到服务器的名称为Chat，方法为Room的处理器
9. 代码参考 [WebSocketClient_test.html](./javascript/WebSocketClient_test.html "code")

![chatroom.png](.images/chatroom.png "chatroom")

#### 参与贡献

1. Fork 本仓库
2. 新建 Feat_xxx 分支
3. 提交代码
4. 新建 Pull Request