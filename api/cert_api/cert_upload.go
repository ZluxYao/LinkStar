package cert_api

import (
	"fmt"
	"io"
	"mime/multipart"
	"strconv"
	"strings"

	"linkstar/modules/cert"
	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

// PEM 是文本，正常链路撑死几十 KB，给 1 MiB 已经很宽松了
const maxPEMSize = 1 << 20

// CertUploadRequest JSON 形式：直接粘贴两段 PEM 文本
type CertUploadRequest struct {
	ID      uint   `json:"id" binding:"required"`
	CertPEM string `json:"certPem" binding:"required"`
	KeyPEM  string `json:"keyPem" binding:"required"`
}

// CertUploadView 上传证书 PEM。同时支持两种提交方式：
//
//	application/json      → {id, certPem, keyPem}   （网页上直接粘贴）
//	multipart/form-data   → id + cert 文件 + key 文件（选文件上传）
//
// 这里不走 BindJsonMiddleware，因为要根据 Content-Type 二选一。
func (CertApi) CertUploadView(c *gin.Context) {
	var id uint
	var certPEM, keyPEM []byte

	if strings.HasPrefix(c.ContentType(), "multipart/form-data") {
		var err error
		if id, certPEM, keyPEM, err = readMultipartPEM(c); err != nil {
			res.FailWithMsg(err.Error(), c)
			return
		}
	} else {
		var cr CertUploadRequest
		if err := c.ShouldBindJSON(&cr); err != nil {
			res.FailWithMsg("参数错误："+err.Error(), c)
			return
		}
		id = cr.ID
		certPEM = []byte(cr.CertPEM)
		keyPEM = []byte(cr.KeyPEM)
	}

	updated, err := cert.UploadPEM(id, certPEM, keyPEM)
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithData(toCertView(updated), c)
}

func readMultipartPEM(c *gin.Context) (uint, []byte, []byte, error) {
	raw := c.PostForm("id")
	n, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || n == 0 {
		return 0, nil, nil, fmt.Errorf("缺少有效的 id 字段")
	}

	certPEM, err := readFormPEM(c, "cert")
	if err != nil {
		return 0, nil, nil, err
	}
	keyPEM, err := readFormPEM(c, "key")
	if err != nil {
		return 0, nil, nil, err
	}
	return uint(n), certPEM, keyPEM, nil
}

func readFormPEM(c *gin.Context, field string) ([]byte, error) {
	fh, err := c.FormFile(field)
	if err != nil {
		return nil, fmt.Errorf("缺少 %s 字段", field)
	}
	if fh.Size > maxPEMSize {
		return nil, fmt.Errorf("%s 文件过大（上限 1MB），确认选对文件了吗", field)
	}

	f, err := fh.Open()
	if err != nil {
		return nil, fmt.Errorf("读取 %s 失败: %w", field, err)
	}
	defer func(f multipart.File) { _ = f.Close() }(f)

	// 即便 Size 已经校验过也再兜一层，避免伪造的 Content-Length
	data, err := io.ReadAll(io.LimitReader(f, maxPEMSize+1))
	if err != nil {
		return nil, fmt.Errorf("读取 %s 失败: %w", field, err)
	}
	if len(data) > maxPEMSize {
		return nil, fmt.Errorf("%s 文件过大（上限 1MB）", field)
	}
	return data, nil
}
