package http

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"oneweekonething.com/owot-pro/internal/platform/pagination"
	"oneweekonething.com/owot-pro/internal/platform/timeutil"
	"oneweekonething.com/owot-pro/internal/system/app"
	"oneweekonething.com/owot-pro/internal/system/domain"
)

func positiveID(c *gin.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Query("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, domain.ErrInvalidArgument
	}
	return id, nil
}

func idList(c *gin.Context) ([]int64, error) {
	var ids []int64
	for _, value := range c.QueryArray("ids") {
		for _, raw := range strings.Split(value, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
			if err != nil || id <= 0 {
				return nil, domain.ErrInvalidArgument
			}
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 || len(ids) > 1000 {
		return nil, domain.ErrInvalidArgument
	}
	return ids, nil
}

func pageQuery(c *gin.Context) (pagination.PageQuery, error) {
	var query pagination.PageQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		return query, domain.ErrInvalidArgument
	}
	return query.Normalize(), nil
}

func optionalInt(c *gin.Context, name string) (*int, error) {
	raw := c.Query(name)
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return nil, domain.ErrInvalidArgument
	}
	return &value, nil
}

func optionalInt64(c *gin.Context, name string) (*int64, error) {
	raw := c.Query(name)
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, domain.ErrInvalidArgument
	}
	return &value, nil
}

func dateRange(c *gin.Context) (*time.Time, *time.Time, error) {
	var result [2]*time.Time
	for i := range result {
		raw := c.Query("createTime[" + strconv.Itoa(i) + "]")
		if raw == "" {
			continue
		}
		value, err := timeutil.Parse(raw)
		if err != nil {
			return nil, nil, domain.ErrInvalidArgument
		}
		result[i] = &value
	}
	if result[0] != nil && result[1] != nil && result[0].After(*result[1]) {
		return nil, nil, domain.ErrInvalidArgument
	}
	return result[0], result[1], nil
}

func typeFilter(c *gin.Context) (app.DictTypeFilter, error) {
	var filter app.DictTypeFilter
	var err error
	filter.PageQuery, err = pageQuery(c)
	if err != nil {
		return filter, err
	}
	filter.Name, filter.Type = c.Query("name"), c.Query("type")
	filter.Status, err = optionalInt(c, "status")
	if err != nil {
		return filter, err
	}
	filter.From, filter.To, err = dateRange(c)
	return filter, err
}

func dataFilter(c *gin.Context) (app.DictDataFilter, error) {
	var filter app.DictDataFilter
	var err error
	filter.PageQuery, err = pageQuery(c)
	if err != nil {
		return filter, err
	}
	filter.DictType, filter.Label = c.Query("dictType"), c.Query("label")
	filter.Status, err = optionalInt(c, "status")
	return filter, err
}
