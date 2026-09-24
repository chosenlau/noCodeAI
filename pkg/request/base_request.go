package request

type DeleteRequest struct {
	Id int64 `json:"id"`
}

type PageRequest struct {
	PageNum   int    `json:"pageNum"`
	PageSize  int    `json:"pageSize"`
	SortField string `json:"sortField"`
	SortOrder string `json:"sortOrder"`
}
