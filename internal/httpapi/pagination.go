package httpapi

import (
	"net/http"

	"weld-ndt/internal/domain"
	"weld-ndt/internal/platform"
)

func ParsePage(r *http.Request) domain.Page {
	return platform.ParsePage(r)
}

func FilterMap(r *http.Request) map[string]any {
	return platform.FilterMap(r)
}
