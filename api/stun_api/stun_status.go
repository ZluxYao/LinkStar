package stun_api

import (
	"encoding/json"
	"linkstar/modules/stun"

	"github.com/gin-gonic/gin"
)

// GetStunStatusView 返回所有服务的当前运行时快照
func (StunApi) GetStunStatusView(c *gin.Context) {
	c.JSON(200, stun.Runtime.Scheduler.Snapshot())
}

// StunStatusEventsView SSE 端点：订阅 Scheduler，实时推送 StateEvent
func (StunApi) StunStatusEventsView(c *gin.Context) {
	ch, unsubscribe := stun.Runtime.Scheduler.Subscribe()
	defer unsubscribe()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // 禁止 nginx 缓冲

	// 连接后立即推全量快照，让客户端有初始状态
	for _, event := range stun.Runtime.Scheduler.Snapshot() {
		writeSSEEvent(c, event)
	}
	c.Writer.Flush()

	clientGone := c.Request.Context().Done()
	for {
		select {
		case <-clientGone:
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			writeSSEEvent(c, event)
			c.Writer.Flush()
		}
	}
}

func writeSSEEvent(c *gin.Context, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	c.Writer.WriteString("data: ")
	c.Writer.Write(data)
	c.Writer.WriteString("\n\n")
}
