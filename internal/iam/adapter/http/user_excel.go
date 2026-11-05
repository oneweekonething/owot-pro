package http

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"

	"oneweekonething.com/owot-pro/internal/iam/app"
	"oneweekonething.com/owot-pro/internal/iam/domain"
	"oneweekonething.com/owot-pro/internal/platform/httpx"
)

func (h *Handler) label(ctx context.Context, dict string, value int) (string, error) {
	if h.dictionaries == nil {
		return strconv.Itoa(value), nil
	}
	return h.dictionaries.Label(ctx, dict, strconv.Itoa(value))
}
func (h *Handler) dictInt(ctx context.Context, dict, value string) (int, error) {
	if n, err := strconv.Atoi(value); err == nil {
		return n, nil
	}
	if h.dictionaries == nil {
		return 0, domain.ErrInvalidArgument
	}
	raw, err := h.dictionaries.Value(ctx, dict, value)
	if err != nil {
		return 0, domain.ErrInvalidArgument
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, domain.ErrInvalidArgument
	}
	return n, nil
}
func (h *Handler) registerUserExcel(r *httpx.Router) {
	headers := []string{"用户账号", "用户昵称", "部门编号", "用户邮箱", "手机号码", "用户性别", "帐号状态", "初始密码"}
	h.route(r, http.MethodGet, "user/get-import-template", "system:user:import", func(c *gin.Context) {
		h.export(c, "user-import-template", headers, [][]any{{"testuser", "测试用户", 0, "", "", "男", "开启", "admin123"}})
	})
	for _, alias := range []string{"export", "export-excel"} {
		h.route(r, http.MethodGet, "user/"+alias, "system:user:export", func(c *gin.Context) {
			q, err := query(c)
			if err != nil {
				h.fail(c, err)
				return
			}
			q.PageSize = -1
			page, err := h.service.PageUsers(c.Request.Context(), q)
			if err != nil {
				h.fail(c, err)
				return
			}
			rows := [][]any{}
			for _, u := range page.List {
				sex, err := h.label(c.Request.Context(), "system_user_sex", u.Sex)
				if err != nil {
					h.fail(c, err)
					return
				}
				status, err := h.label(c.Request.Context(), "common_status", u.Status)
				if err != nil {
					h.fail(c, err)
					return
				}
				rows = append(rows, []any{u.Username, u.Nickname, u.DeptID, u.Email, u.Mobile, sex, status})
			}
			h.export(c, "users", headers[:7], rows)
		})
	}
	h.route(r, http.MethodPost, "user/import", "system:user:import", func(c *gin.Context) {
		update := false
		if raw := c.Query("updateSupport"); raw != "" {
			v, err := strconv.ParseBool(raw)
			if err != nil {
				h.fail(c, domain.ErrInvalidArgument)
				return
			}
			update = v
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8<<20)
		if err := c.Request.ParseMultipartForm(8 << 20); err != nil {
			h.fail(c, domain.ErrInvalidArgument)
			return
		}
		defer func() { _ = c.Request.MultipartForm.RemoveAll() }()
		file, _, err := c.Request.FormFile("file")
		if err != nil {
			h.fail(c, domain.ErrInvalidArgument)
			return
		}
		defer func() { _ = file.Close() }()
		book, err := excelize.OpenReader(io.LimitReader(file, 8<<20), excelize.Options{UnzipSizeLimit: 32 << 20, UnzipXMLSizeLimit: 16 << 20})
		if err != nil {
			h.fail(c, domain.ErrInvalidArgument)
			return
		}
		defer func() { _ = book.Close() }()
		sheets := book.GetSheetList()
		if len(sheets) == 0 {
			h.fail(c, domain.ErrInvalidArgument)
			return
		}
		rows, err := book.Rows(sheets[0])
		if err != nil {
			h.fail(c, domain.ErrInvalidArgument)
			return
		}
		defer func() { _ = rows.Close() }()
		if !rows.Next() {
			h.fail(c, domain.ErrInvalidArgument)
			return
		}
		columns, err := rows.Columns()
		if err != nil || len(columns) < 7 {
			h.fail(c, domain.ErrInvalidArgument)
			return
		}
		for i, name := range headers[:7] {
			if columns[i] != name {
				h.fail(c, domain.ErrInvalidArgument)
				return
			}
		}
		imports := []app.ImportUser{}
		for rows.Next() {
			if len(imports) >= 1000 {
				h.fail(c, domain.ErrInvalidArgument)
				return
			}
			values, err := rows.Columns()
			if err != nil {
				h.fail(c, domain.ErrInvalidArgument)
				return
			}
			if len(values) == 0 {
				continue
			}
			for len(values) < 8 {
				values = append(values, "")
			}
			dept, err := strconv.ParseInt(strings.TrimSpace(values[2]), 10, 64)
			if err != nil || dept < 0 {
				h.fail(c, domain.ErrInvalidArgument)
				return
			}
			sex, err := h.dictInt(c.Request.Context(), "system_user_sex", values[5])
			if err != nil {
				h.fail(c, err)
				return
			}
			status, err := h.dictInt(c.Request.Context(), "common_status", values[6])
			if err != nil {
				h.fail(c, err)
				return
			}
			password := values[7]
			if password == "" {
				password = "admin123"
			}
			imports = append(imports, app.ImportUser{User: domain.AdminUser{Username: values[0], Nickname: values[1], DeptID: dept, Email: values[3], Mobile: values[4], Sex: sex, Status: status, PostIDs: []int64{}}, Password: password})
		}
		if err := rows.Error(); err != nil {
			h.fail(c, domain.ErrInvalidArgument)
			return
		}
		result, err := h.service.ImportUsers(c.Request.Context(), imports, update)
		h.complete(c, gin.H{"createUsernames": result.CreateUsernames, "updateUsernames": result.UpdateUsernames, "failureUsernames": result.FailureUsernames}, err)
	})
}
