const ws = {
    websocket: null,
    connUrl: "",
    connException: false,
    requestHandlerMap: {},
    responseCallbackMap: {},
    UUID: function () {
        return 'xxxxxxxx-xxxx-xxxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
            const r = Math.random() * 16 | 0, v = c === 'x' ? r : (r & 0x3 | 0x8);
            return v.toString(16);
        });
    },
    MessageBodyEncode: function (sign, body) {
        if (sign === "") {
            return body
        } else {
            let result = ''
            switch (sign) {  // 对body加密
                case "none":
                    result = body
                    break
                case "base64":
                    // result = Base64.encode(body)
                    break
                case "url":
                    result = encodeURIComponent(body)
                    break
                default:
                    console.error("MessageBodyEncode 未知的Encode类型", sign)
                    break
            }
            return result
        }
    },
    MessageBodyDecode: function (sign, body) {
        if (sign === "") {
            return body
        } else {
            let result = ''
            switch (sign) {  // 对body解密
                case "none":
                    result = body
                    break
                case "base64":
                    // result = Base64.decode(body)
                    break
                case "url":
                    result = decodeURIComponent(body)
                    break
                default:
                    console.error("MessageBodyDecode 未知的Decode类型", sign)
                    break
            }
            return result
        }
    },
    builderRequest: function (uid) {
        return {
            uid: uid,
            handler: '',
            method: '',
            header: {},
            body: '',
            sign: 'none',
        }
    },
    builderResponse: function (uid) {
        return {
            uid: uid,
            header: {},
            code: 0,
            msg: '',
            body: '',
            sign: 'none',
        }
    },
    openConnection: function (url, callback) {
        this.connUrl = url
        this.websocket = new WebSocket(url)
        this.websocket.onerror = function (event) {
            this.connException = true
            console.log(event)
        }.bind(this)
        this.websocket.onclose = function (event) {
            this.connException = true
            console.log(event)
        }.bind(this)
        this.websocket.onopen = function () {
            this.connException = false
            if (callback != null) {
                callback(this.websocket)
            }
            console.log('open_success', url)
        }.bind(this)
        this.websocket.onmessage = function (event) {
            if (event.data.size !== undefined && typeof event.data === "object") {
                this.onHandlerResponse(event.data)  // 响应消息走字节通道
            } else {
                this.onHandlerRequest(event.data)    // 主动请求消息走字符串通道
            }
        }.bind(this)
    },
    closeConnection: function (code, msg) {
        this.websocket.close(code, msg)
        this.websocket = null
    },
    sendTextMessage: function (handler, method, body, callback) {
        let request = this.builderRequest(this.UUID())
        request.handler = handler
        request.method = method
        request.header = {}
        request.body = body // MessageBodyEncode(sign, body)
        request.sign = "none"
        this.sendRequestMessage(request, 1000 * 60, callback)
    },
    sendRequestMessage: function (request, timeout, callback) {
        const timeoutHandler = function (code, msg) {
            let callback = this.responseCallbackMap[request.uid]
            if (callback != null) {
                callback({code: code, msg: msg})
                delete this.responseCallbackMap[request.uid]
            }
        }.bind(this)

        request.body = this.MessageBodyEncode(request.sign, request.body)

        this.responseCallbackMap[request.uid] = function (data) {
            if (callback != null) callback(data, this)
        }.bind(this)
        if (!this.connException) {
            this.websocket.send(JSON.stringify(request))
            setTimeout(function () {
                timeoutHandler(-3, "failed_to_send")
            }, timeout)
        } else {
            timeoutHandler(-3, "failed_to_send")
        }
    },
    onHandlerRequest: function (data) {
        let request = JSON.parse(data)
        let func = this.requestHandlerMap[`${request.handler}@${request.method}`]
        if (func == null) {
            console.error("onHandlerRequest 没有对应的 handler 处理", data)
        } else {
            request.body = this.MessageBodyDecode(request.sign, request.body)
            let response = this.builderResponse()
            response.uid = request.uid // request 与 response
            func(request, response) // 函数执行处理
            this.websocket.send(new Blob([JSON.stringify(response)]))   // 给服务端回写相应(必须是字节类型)
        }
    },
    onHandlerResponse: function (data) {
        let reader = new FileReader()
        reader.onload = function (e) {
            let response = JSON.parse(String(e.target.result))    // 字节 转 字符
            let callback = this.responseCallbackMap[response.uid]
            if (callback != null) {
                response.body = this.MessageBodyDecode(response.sign, response.body)
                callback(response)
                delete this.responseCallbackMap[response.uid]
            } else {
                console.log("onHandlerResponse 没有对应的 callback 处理", data)
            }
        }.bind(this)
        reader.readAsText(data)
    },
    registerRequestHandlerFunction: function (handlerName, handlerMethod, handlerFunction) {
        this.requestHandlerMap[handlerName + "@" + handlerMethod] = handlerFunction
    }
};

setInterval(function () {
    if (ws.websocket != null) {
        ws.sendTextMessage("Health", "Index", new Date().getTime().toString(10), function (response, apiFunc) {
            if (response.code !== 0 && response.msg === "failed_to_send") {
                console.log("WebSocket try retry conn", ws.connUrl)
                ws.openConnection(ws.connUrl, null)   // 重连
            } else {
                console.log("WebSocket conn health check", response.code, response.msg)
            }
        })
    }
}, 4500)

window.$WebSocketHttp = ws