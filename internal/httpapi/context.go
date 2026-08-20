package httpapi

import "context"

type paramsKey struct{}

func withParams(ctx context.Context, params map[string]string) context.Context {
	return context.WithValue(ctx, paramsKey{}, params)
}

func Param(r interface{ Context() context.Context }, name string) string {
	if v, ok := r.Context().Value(paramsKey{}).(map[string]string); ok {
		return v[name]
	}
	return ""
}
