package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/golang-jwt/jwt/v5"
)

// JWTClaims 自定义 JWT 声明
type JWTClaims struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"` // admin, analyst, viewer
	jwt.RegisteredClaims
}

var jwtSecret []byte

// InitJWT 初始化 JWT 密钥（从配置加载）
func InitJWT(secret string) {
	jwtSecret = []byte(secret)
}

// GenerateToken 签发 JWT Token
func GenerateToken(userID, role string) (string, error) {
	claims := JWTClaims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "secops-agent",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// JWTAuthMiddleware JWT 认证中间件
// 从 Authorization 头提取 Bearer Token，验证签名和有效期，将 user_id 和 role 注入请求上下文。
// 未携带或无效 Token 返回 401 Unauthorized。
func JWTAuthMiddleware(r *ghttp.Request) {
	// 免认证路径
	path := r.URL.Path
	if path == "/healthz" || path == "/readyz" || path == "/api/auth/login" {
		r.Middleware.Next()
		return
	}

	authHeader := r.GetHeader("Authorization")
	if authHeader == "" {
		r.Response.WriteStatus(http.StatusUnauthorized)
		r.Response.WriteJsonExit(Response{
			Code:    40100,
			Message: "缺少认证信息，请在 Authorization 头中携带 Bearer Token",
		})
		return
	}

	// 提取 Bearer Token
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		r.Response.WriteStatus(http.StatusUnauthorized)
		r.Response.WriteJsonExit(Response{
			Code:    40101,
			Message: "Authorization 格式错误，应为 Bearer <token>",
		})
		return
	}

	tokenStr := parts[1]
	claims := &JWTClaims{}

	token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return jwtSecret, nil
	})

	if err != nil || !token.Valid {
		r.Response.WriteStatus(http.StatusUnauthorized)
		r.Response.WriteJsonExit(Response{
			Code:    40102,
			Message: "Token 无效或已过期，请重新登录",
		})
		return
	}

	// 将用户信息注入上下文
	r.SetCtxVar("user_id", claims.UserID)
	r.SetCtxVar("user_role", claims.Role)

	g.Log().Debugf(r.Context(), "JWT auth passed: user_id=%s role=%s", claims.UserID, claims.Role)

	r.Middleware.Next()
}
