package platform

import (
	"net/http"
	"strconv"

	"weld-ndt/internal/domain"
)

func ParsePage(r *http.Request) domain.Page {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	return domain.Page{Page: page, Size: size}.Normalize()
}

func FilterMap(r *http.Request) map[string]any {
	m := map[string]any{}
	for k, vals := range r.URL.Query() {
		if len(vals) == 0 {
			continue
		}
		switch k {
		case "page", "size", "sort":
			continue
		default:
			m[k] = vals[0]
		}
	}
	return m
}
