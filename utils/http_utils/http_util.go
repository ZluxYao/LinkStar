package httpUtils

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// GetHTTPRequest 处理HTTP返回结果,返回序列化json
func GetHTTPResponse(resp *http.Response, result interface{}) error {

	body, err := GetHTTPResponseBody(resp)
	if err != nil {
		return err
	}

	if len(body) != 0 {
		err = json.Unmarshal(body, &result)
	}

	return err
}

// GetHTTPResponseOrg 处理HTTP结果返回byte
func GetHTTPResponseBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	lr := io.LimitReader(resp.Body, 1024000)
	body, err := io.ReadAll(lr)

	if err != nil {
		return nil, err
	}

	//300及以上的状态码都是异常
	if resp.StatusCode >= 300 {
		err = fmt.Errorf("返回内容：%s ,返回状态码:%d", string(body), resp.StatusCode)
	}

	return body, err
}
