package middleware

import (
	"net/http"

	"github.com/gogf/gf/v2/net/ghttp"
)

// Response 统一响应结构
type Response struct {
	Code    int         `json:"code"    dc:"业务状态码: 0=成功, 40xxx=客户端错误, 50xxx=服务端错误"`
	Message string      `json:"message" dc:"消息提示"`
	Data    interface{} `json:"data"    dc:"执行结果"`
}

// CORSMiddleware 处理CORS跨域请求
func CORSMiddleware(r *ghttp.Request) {
	r.Response.CORSDefault()
	r.Middleware.Next()
}

// ResponseMiddleware 统一响应格式封装中间件
// 将 handler 的返回值和错误统一包装为 {code, message, data} 格式，并设置正确的 HTTP 状态码。
func ResponseMiddleware(r *ghttp.Request) {
	r.Middleware.Next()

	// SSE 流式响应不做包装（已由 SSE handler 自行处理 Content-Type）
	if r.Response.Header().Get("Content-Type") == "text/event-stream" {
		return
	}

	var (
		code int
		msg  string
		res  = r.GetHandlerResponse()
		err  = r.GetError()
	)

	if err != nil {
		msg = err.Error()
		code = 50000
		r.Response.WriteStatus(http.StatusInternalServerError)
	} else {
		msg = "OK"
		code = 0
	}

	r.Response.WriteJson(Response{
		Code:    code,
		Message: msg,
		Data:    res,
	})
}
