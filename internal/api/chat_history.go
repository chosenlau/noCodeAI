package api

import (
	"time"

	"github.com/chosenlau/noCodeAI/internal/dal/model"
	"github.com/chosenlau/noCodeAI/pkg/response"
)

type NoCodeChatHistoryQueryRequest struct {
	Id             int64     `json:"id"`
	AppId          int64     `json:"appId"`
	Message        string    `json:"message"`
	MessageType    string    `json:"messageType"`
	UserId         int64     `json:"userId"`
	LastCreateTime time.Time `json:"lastCreateTime"`
	LastId         int64     `json:"lastId"`
}

type NoCodeChatHistoryQueryResponse response.BaseResponse[response.PageResponse[*model.ChatHistory]]
