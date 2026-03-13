package v1

import (
	"github.com/gogf/gf/v2/frame/g"
)

type ChatReq struct {
	g.Meta   `path:"/chat" method:"post" summary:"安全咨询对话"`
	Id       string `json:"id" v:"required|length:1,64" dc:"会话ID"`
	Question string `json:"question" v:"required|length:1,4096" dc:"用户提问内容"`
}

type ChatRes struct {
	Answer string `json:"answer"`
}

type ChatStreamReq struct {
	g.Meta   `path:"/chat_stream" method:"post" summary:"流式安全咨询对话"`
	Id       string `json:"id" v:"required|length:1,64" dc:"会话ID"`
	Question string `json:"question" v:"required|length:1,4096" dc:"用户提问内容"`
}

type ChatStreamRes struct {
}

type FileUploadReq struct {
	g.Meta `path:"/upload" method:"post" mime:"multipart/form-data" summary:"上传安全Playbook文档"`
}

type FileUploadRes struct {
	FileName string `json:"fileName" dc:"保存的文件名"`
	FilePath string `json:"filePath" dc:"文件保存路径"`
	FileSize int64  `json:"fileSize" dc:"文件大小(字节)"`
}

type AIOpsReq struct {
	g.Meta `path:"/ai_ops" method:"post" summary:"安全事件自动响应"`
}

type AIOpsRes struct {
	Result   string   `json:"result"`
	Detail   []string `json:"detail"`
	TicketNo string   `json:"ticket_no,omitempty" dc:"自动生成的工单编号"`
}
