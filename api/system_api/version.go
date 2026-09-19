package system_api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const Version = "0.6.2"

type VersionResponse struct {
	Version string `json:"version"`
}

func (SystemApi) GetVersion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": VersionResponse{
			Version: Version,
		},
	})
}
