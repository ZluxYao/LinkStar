package home_api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type bingWallpaperResponse struct {
	Images []struct {
		URL string `json:"url"`
	} `json:"images"`
}

func (HomeApi) BingWallpaperView(c *gin.Context) {
	client := http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("https://www.bing.com/HPImageArchive.aspx?format=js&idx=0&n=1&mkt=en-US")
	if err != nil {
		c.Status(http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	var data bingWallpaperResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil || len(data.Images) == 0 || data.Images[0].URL == "" {
		c.Status(http.StatusBadGateway)
		return
	}

	imageURL := data.Images[0].URL
	if strings.HasPrefix(imageURL, "/") {
		imageURL = "https://www.bing.com" + imageURL
	}
	imageURL = strings.Replace(imageURL, "1920x1080", "UHD", 1)

	c.Header("Cache-Control", "public, max-age=3600")
	c.JSON(http.StatusOK, gin.H{"url": imageURL})
}
