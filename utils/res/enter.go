package res

import (
	"github.com/gin-gonic/gin"
)

type Response struct {
	Code int    `json:"code"`
	Data any    `json:"data"`
	Msg  string `json:"msg"`
}

func Ok(data any, msg string, c *gin.Context) {
	c.JSON(200, Response{
		Code: 0,
		Data: data,
		Msg:  msg,
	})
}

func OkWithMsg(msg string, c *gin.Context) {
	Ok(gin.H{}, msg, c)
}

func OkWithData(data any, c *gin.Context) {
	Ok(data, "成功", c)
}

func OkWithList(list any, count int64, c *gin.Context) {
	Ok(map[string]any{
		"list":  list,
		"count": count,
	}, "成功", c)
}
func Fail(code int, msg string, c *gin.Context) {
	c.JSON(200, Response{
		Code: code,
		Data: gin.H{},
		Msg:  msg,
	})
}

func FailWithMsg(msg string, c *gin.Context) {
	Fail(7, msg, c)
}

func FailWithError(err error, c *gin.Context) {
	msg := err.Error()
	Fail(7, msg, c)
}

// 鉴权约定码：401 未登录/token 失效，428 需先初始化（设置密码）
const (
	CodeUnauthorized = 401
	CodeNeedInit     = 428
)

func FailUnauthorized(c *gin.Context) {
	Fail(CodeUnauthorized, "未登录或登录已失效", c)
}

func FailNeedInit(c *gin.Context) {
	Fail(CodeNeedInit, "系统尚未初始化", c)
}
