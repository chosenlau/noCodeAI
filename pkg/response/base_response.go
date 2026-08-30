package response

import (
	"errors"

	"github.com/chosenlau/noCodeAI/pkg/errorutil"
)

type BaseResponse[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

func NewSuccessResponse[T any](data T) *BaseResponse[T] {
	return &BaseResponse[T]{
		Code:    int(errorutil.SuccessErrCode),
		Message: errorutil.Success.Message,
		Data:    data,
	}
}

func NewErrorResponse[T any](err error) *BaseResponse[T] {
	newError := errorutil.BusinessError{}
	if errors.As(err, &newError) {
		return &BaseResponse[T]{
			Code:    newError.Code,
			Message: newError.Message,
		}
	} else {
		newError = errorutil.ConvertError(err)
		return &BaseResponse[T]{
			Code:    newError.Code,
			Message: newError.Message,
		}
	}
}

func NewResponse[T any](code int, message string, data T) *BaseResponse[T] {
	return &BaseResponse[T]{
		Code:    code,
		Message: message,
		Data:    data,
	}
}

type PageResponse[T any] struct {
	Records            []T  `json:"records"`
	PageNum            int  `json:"pageNum"`
	PageSize           int  `json:"pageSize"`
	TotalPage          int  `json:"totalPage"`
	TotalRow           int  `json:"totalRow"`
	OptimizeCountQuery bool `json:"optimizeCountQuery"`
}
