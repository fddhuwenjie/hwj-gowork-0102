package httpapi

import (
	"net/http"
	"strings"
)

type route struct {
	Method  string
	Pattern []string
	Handler http.HandlerFunc
}

type Router struct {
	routes []route
}

func NewRouter() *Router {
	return &Router{}
}

func (r *Router) Handle(method, pattern string, handler http.HandlerFunc) {
	r.routes = append(r.routes, route{Method: method, Pattern: splitPath(pattern), Handler: handler})
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	path := splitPath(req.URL.Path)
	for _, rt := range r.routes {
		if rt.Method != req.Method {
			continue
		}
		if params, ok := match(rt.Pattern, path); ok {
			req = req.WithContext(withParams(req.Context(), params))
			rt.Handler.ServeHTTP(w, req)
			return
		}
	}
	WriteError(w, http.StatusNotFound, "not_found", "接口不存在", nil)
}

func splitPath(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

func match(pattern, path []string) (map[string]string, bool) {
	if len(pattern) != len(path) {
		return nil, false
	}
	params := map[string]string{}
	for i := range pattern {
		if strings.HasPrefix(pattern[i], "{") && strings.HasSuffix(pattern[i], "}") {
			params[strings.Trim(pattern[i], "{}")] = path[i]
			continue
		}
		if pattern[i] != path[i] {
			return nil, false
		}
	}
	return params, true
}
