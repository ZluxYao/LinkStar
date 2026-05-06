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
		URL     string `json:"url"`
		URLBase string `json:"urlbase"`
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

	resolution := strings.ToLower(c.DefaultQuery("resolution", "uhd"))
	imageURL := data.Images[0].URL
	if data.Images[0].URLBase != "" {
		imageURL = data.Images[0].URLBase + "_UHD.jpg"
	} else {
		imageURL = strings.Replace(imageURL, "1920x1080", "UHD", 1)
	}
	if resolution == "1080" || resolution == "1920x1080" {
		imageURL = strings.Replace(imageURL, "UHD", "1920x1080", 1)
	}
	if strings.HasPrefix(imageURL, "/") {
		imageURL = "https://bing.com" + imageURL
	}

	c.Header("Cache-Control", "public, max-age=3600")
	c.JSON(http.StatusOK, gin.H{"url": imageURL})
}
