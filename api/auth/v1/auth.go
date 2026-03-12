package v1

import "github.com/gogf/gf/v2/frame/g"

// LoginReq 登录请求
type LoginReq struct {
	g.Meta   `path:"/auth/login" method:"post" tags:"认证" summary:"用户登录获取JWT Token"`
	Username string `json:"username" v:"required|length:1,64" dc:"用户名"`
	Password string `json:"password" v:"required|length:1,128" dc:"密码"`
}

// LoginRes 登录响应
type LoginRes struct {
	Token     string `json:"token" dc:"JWT访问令牌"`
	ExpiresIn int    `json:"expires_in" dc:"过期时间（秒）"`
}
