package errorutil

import (
	"errors"
	"fmt"
)

// BusinessError 自定义错误结构体
type BusinessError struct {
	Code    int
	Message string
}

func (e BusinessError) Error() string {
	return fmt.Sprintf("errorCode: %d, message: %s", e.Code, e.Message)
}

func NewBusinessError(code ErrorNo, message string) BusinessError {
	return BusinessError{
		Code:    int(code),
		Message: message,
	}
}

func NewBusinessErrorByNo(code ErrorNo) BusinessError {
	message, _ := ErrMessageMap[code]
	return BusinessError{
		Code:    int(code),
		Message: message,
	}
}

func (e BusinessError) WithMessage(message string) BusinessError {
	e.Message = message
	return e
}

// ConvertError 将错误转换自定义系统错误
func ConvertError(err error) BusinessError {
	newErr := BusinessError{}
	if errors.As(err, &newErr) {
		return newErr
	}

	newErr = SystemError
	newErr.Message = err.Error()
	return newErr
}

type ErrorNo int

// 错误码枚举
const (
	SuccessErrCode        ErrorNo = 0
	ParamErrCode          ErrorNo = 40000
	NotLoginErrCode       ErrorNo = 40100
	NotAuthErrCode        ErrorNo = 40101
	ForbiddenErrorCode    ErrorNo = 40400
	TooManyRequestErrCode ErrorNo = 40300
	SystemErrorCode       ErrorNo = 50000
	OperationErrorCode    ErrorNo = 51001
)

var ErrMessageMap = map[ErrorNo]string{
	SuccessErrCode:        "ok",
	ParamErrCode:          "请求参数错误",
	NotLoginErrCode:       "未登录",
	NotAuthErrCode:        "无权限",
	ForbiddenErrorCode:    "禁止访问",
	TooManyRequestErrCode: "请求过于频繁",
	SystemErrorCode:       "系统内部异常",
	OperationErrorCode:    "操作失败",
}

// 错误码全局变量
var (
	Success             = NewBusinessErrorByNo(SuccessErrCode)
	ParamsError         = NewBusinessErrorByNo(ParamErrCode)
	NotLoginError       = NewBusinessErrorByNo(NotLoginErrCode)
	NotAuthError        = NewBusinessErrorByNo(NotAuthErrCode)
	ForbiddenError      = NewBusinessErrorByNo(ForbiddenErrorCode)
	TooManyRequestError = NewBusinessErrorByNo(TooManyRequestErrCode)
	SystemError         = NewBusinessErrorByNo(SystemErrorCode)
	OperationError      = NewBusinessErrorByNo(OperationErrorCode)
)
