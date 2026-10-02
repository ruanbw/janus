package geo

import (
	"errors"
	"net/url"
)

// sanitizeURLErr 把 *url.Error 里的完整 URL(可能内嵌 API key)剥掉,
// 只保留错误本质(DNS/超时/连接拒绝等),避免 key 进日志或响应。
func sanitizeURLErr(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return errors.New(ue.Op + ": " + ue.Err.Error())
	}
	return err
}
