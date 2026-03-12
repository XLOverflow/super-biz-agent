package auth

import (
	v1 "SecOpsAgent/api/auth/v1"
	"SecOpsAgent/utility/middleware"
	"context"
	"errors"
)

// 演示用户（生产环境应从数据库校验）
var demoUsers = map[string]struct {
	Password string
	Role     string
}{
	"admin":   {Password: "secops2024", Role: "admin"},
	"analyst": {Password: "analyst123", Role: "analyst"},
}

type ControllerV1 struct{}

func NewV1() *ControllerV1 {
	return &ControllerV1{}
}

func (c *ControllerV1) Login(ctx context.Context, req *v1.LoginReq) (res *v1.LoginRes, err error) {
	user, ok := demoUsers[req.Username]
	if !ok || user.Password != req.Password {
		return nil, errors.New("用户名或密码错误")
	}

	token, err := middleware.GenerateToken(req.Username, user.Role)
	if err != nil {
		return nil, errors.New("Token 签发失败")
	}

	return &v1.LoginRes{
		Token:     token,
		ExpiresIn: 86400, // 24小时
	}, nil
}
