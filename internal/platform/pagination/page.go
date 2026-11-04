// Package pagination defines framework-free pagination contracts.
package pagination

import "encoding/json"

type PageQuery struct {
	PageNo   int `form:"pageNo"`
	PageSize int `form:"pageSize"`
}

func (q PageQuery) Normalize() PageQuery {
	if q.PageNo < 1 {
		q.PageNo = 1
	}
	if q.PageSize != -1 && q.PageSize < 1 {
		q.PageSize = 10
	}
	if q.PageSize > 200 {
		q.PageSize = 200
	}
	// Bound the offset to a portable signed 32-bit SQL LIMIT value.
	if q.PageSize > 0 && q.PageNo > 2147483647/q.PageSize {
		q.PageNo = 2147483647 / q.PageSize
	}
	return q
}

func (q PageQuery) Offset() int {
	q = q.Normalize()
	if q.PageSize == -1 {
		return 0
	}
	return (q.PageNo - 1) * q.PageSize
}

type PageResult[T any] struct {
	List  []T   `json:"list"`
	Total int64 `json:"total"`
}

func (p PageResult[T]) MarshalJSON() ([]byte, error) {
	if p.List == nil {
		p.List = []T{}
	}
	type result PageResult[T]
	return json.Marshal(result(p))
}
